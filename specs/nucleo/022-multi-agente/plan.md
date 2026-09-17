# Plan: Multi-agente — delegação entre agentes (022)

**Feature**: `specs/nucleo/022-multi-agente/` · **Status**: Aprovado

Constitution: **não viola** nenhuma seção. Aditivo na API pública; nenhum JSON Schema de fio/persistência obrigatório muda (a ligação pai→filho vai num campo **opcional** aditivo do snapshot, se necessário; v1 usa `ParentSessionID` no `RunRequest`).

## Arquitetura

1. **Config** (`internal/core/config.go`): `AgentSpec{Description, Model, SystemPrompt string; Tools []string; MaxIterations int; Policy *AgentPolicy}`; `Config.Agents map[string]AgentSpec`; `Config.MaxAgentDepth int`.
2. **Porta de execução de sub-agente** (`internal/core/ports.go`): `AgentRunner interface { RunSubAgent(ctx context.Context, req SubAgentRequest) (SubAgentResult, error) }` — implementada pelo `Harness` (evita ciclo motor→adaptador). `SubAgentRequest{ParentSessionID, UserID, AgentID, Input []Part, Budget Budget}`; `SubAgentResult{SessionID string, Output []Part, StopReason StopReason}`.
3. **Regra pura/catálogo** (`internal/engine/agents`): `Spec`, `FilterTools(tools []Tool, patterns []string) []Tool` (glob por `servidor.nome`/`nome`), `DelegateToolSpec()` (JSON Schema de `{agent, task}`), `DepthFromContext`/`WithDepth` (guard de profundidade no contexto).
4. **ToolSource sintética** (`internal/engine/agents/source.go`): `Source` que implementa `ToolSource` (List devolve `agents.delegate` quando há agentes; Call valida o agente, monta o `SubAgentRequest`, chama `AgentRunner` e converte o output em `ToolResult`). Erros de agente desconhecido/profundidade viram `ToolResult.IsError=true` (nunca erro de transporte).
5. **Motor** (`internal/engine`):
   - `New`/boot: se `len(Agents) > 0`, injeta o `Source` no catálogo (respeitando `ToolMiddleware`).
   - `runTurn`: aumenta a profundidade no contexto; resolve `AgentSpec` do agente ativo para modelo/prompt/maxIterations/política.
   - `RunSubAgent`: cria sessão nova do agente alvo (`ParentSessionID`), roda `Run` com `Input=task`, devolve o texto final. Falha → erro.
6. **Observabilidade** (`internal/core/events.go`): `SubAgentEvent{ParentSessionID, ChildSessionID, Agent, Depth, Status}` + `SubAgentHandler` opcional.

## Contratos (aditivos)

- `AgentSpec`, `Config.Agents`, `Config.MaxAgentDepth`, `AgentRunner`, `SubAgentRequest`, `SubAgentResult`, `SubAgentEvent`, `SubAgentHandler`; aliases em `harness/alias.go`.
- `RunRequest.ParentSessionID` (aditivo, opcional).

## Alternativas consideradas

- **Agente como tool por agente** (uma tool por especialista): enche o catálogo e o schema; uma tool com enum é mais simples e versionável. Escolhida a segunda.
- **Handoff explícito (troca de agente no mesmo loop)**: muda a semântica de sessão e o host perde o controle; adiada para v2.
- **Runner como adaptador público**: criaria ciclo motor↔adaptador; a porta mantém a direção das dependências (constitution §3). Escolhida a porta.
- **Sem limite de profundidade**: risco de recursão/DoS. `MaxAgentDepth` default 1.

## Riscos

- Explosão de custo com delegações: cada sub-turno tem budget próprio; documentado e limitado por profundidade.
- Confirmação dentro do sub-turno: precisa de sessão própria; o host resolve pelo `ChildSessionID` (observável).
- Filtro de tools por glob: comportamento conservador (padrão sem curinga casa exato).
