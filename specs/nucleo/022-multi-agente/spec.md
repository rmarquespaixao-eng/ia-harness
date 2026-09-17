# Feature Specification: Multi-agente (delegação entre agentes)

**Feature Branch**: `nucleo/022-multi-agente` · **Status**: Draft (aguardando portão)

**Input**: pedido do operador — Fase D (P2): "multi-agente".

## Problema

O harness conhece `AgentID` (política/prompt por agente), mas todo turno é executado por um único agente do host. Não há como um agente **delegar** uma subtarefa a outro especialista (ex.: um agente "orquestrador" que chama um agente "analista" ou "conciliador"), nem como restringir as tools de cada especialista. O resultado é que o host precisa orquestrar por fora, perdendo o loop com tool-calling e a auditoria unificada.

## Requisitos (EARS)

- **FR-MA-001**: The harness SHALL suportar agentes nomeados declarados em `Config.Agents`, cada um com `Description`, `Model` (alias), `SystemPrompt`, `Tools` (filtro), `MaxIterations` e `Policy` (sobreposição).
- **FR-MA-002**: WHEN um agente está ativo no turno e há agentes delegáveis, o harness MUST expor uma tool sintética `agents.delegate` (args `{agent, task}`) ao modelo.
- **FR-MA-003**: WHEN o modelo chama `agents.delegate`, o harness MUST executar um **sub-turno** com o agente alvo (modelo/prompt/política/tools próprios), com o mesmo `UserID` e sessão própria persistida, e devolver o texto final como resultado da tool.
- **FR-MA-004**: A profundidade de delegação MUST ser limitada por `Config.MaxAgentDepth` (default 1); exceder devolve resultado de erro ao modelo, sem recursão infinita.
- **FR-MA-005**: Agent desconhecido no argumento MUST devolver resultado de erro nomeado; nenhum sub-turno roda.
- **FR-MA-006**: Tools de um agente (`AgentSpec.Tools`) MUST filtrar o catálogo do sub-turno por nome/namespace (glob simples); vazio = herda o catálogo do host.
- **FR-MA-007**: A política do agente alvo MUST ser aplicada ao sub-turno (default deny preservado); a delegação em si é auditável (evento `SubAgentEvent`).
- **FR-MA-008**: O sub-turno MUST ser cancelável pelo contexto do pai e MUST NOT herdar confirmações pendentes simplesmente (a confirmação do sub-turno é resolvida pelo host via sessão do sub-agente).
- **FR-MA-009**: `Config.Agents` vazio MUST preservar o comportamento anterior (nenhuma tool sintética).

## Critérios (Gherkin)

```gherkin
Cenário: orquestrador delega
  Dado um agente "resumo" configurado e o agente atual com permissão de delegar
  Quando o modelo chama agents.delegate {agent:"resumo", task:"resuma X"}
  Então um sub-turno do agente "resumo" roda em sessão própria
  E o texto final volta como resultado da tool

Cenário: profundidade limitada
  Dado MaxAgentDepth=1 e um agente que também poderia delegar
  Quando o sub-agente tenta delegar
  Então recebe erro "profundidade máxima de delegação" e o turno segue

Cenário: agente desconhecido
  Quando o modelo chama agents.delegate {agent:"inexistente"}
  Então a tool devolve erro nomeado sem iniciar sub-turno
```

## Casos de borda

- Delegação durante `Determine` de confirmação: sub-turno pausa em awaiting_confirmation com sessão própria (o host decide retomar).
- Falha do sub-turno: vira resultado de erro da tool, nunca derruba o pai.
- `AgentSpec.Model` vazio: usa `DefaultModel`/modelo do pai.
- Isolamento: o sub-agente nunca lê sessão do pai; a ligação é por `ParentSessionID` (observável/auditoria).

## Não-objetivos (v1)

- Grafo de agentes/handoff explícito ("devolver controle a outro agente") — v1 é delegação aninhada.
- Orçamento global compartilhado entre pai e sub-turnos (cada um tem o seu `Budget`).
- UI de escolha de agente (host).
- Concorrência de múltiplos sub-agentes no mesmo turno (o loop já executa tool calls; delegações em paralelo vêm com a feature 015 se permitidas).

## Dependências e espelho

- Depende de 003 (fachada/motor), 014 (middleware) e 018 (janela por modelo, no sub-turno).
- Host: `financeiro-api-v2` define agentes (ex.: orquestrador + conciliador) e expõe sessões de sub-agente.
