# Use Cases: Multi-agente — delegação entre agentes (022)

Formato fully-dressed (Cockburn) + Gherkin. O operador implementa; o agente audita.

## CU-MA-1 — Orquestrador delega a um especialista

- **Identificador**: CU-MA-1 · **Escopo**: motor do harness · **Nível**: subfunção
- **Ator primário**: host (biblioteca embarcada)
- **Partes interessadas e interesses**: operador (especializar agentes sem orquestrar por fora); usuário (resposta do especialista)
- **Pré-condições**: `Config.Agents` com o agente alvo; agente atual permitido a usar `agents.delegate`
- **Gatilho**: o modelo emite a tool call `agents.delegate {agent, task}`
- **Garantia de sucesso**: o sub-agente roda em sessão própria e devolve o texto final ao pai
- **Garantia mínima**: falha do sub-turno vira resultado de erro da tool, sem derrubar o pai
- **Mapeamento técnico**: `agents.Source.Call` → `Harness.RunSubAgent` → `Run`
- **Fluxo principal de sucesso**:
  1. O catálogo do turno inclui `agents.delegate`.
  2. O modelo chama a tool com o agente e a tarefa.
  3. O motor valida o agente, a profundidade e filtra as tools do alvo.
  4. Um `Run` do agente alvo executa com sessão própria (`ParentSessionID`).
  5. O texto final do sub-turno vira o resultado da tool; o pai continua.
- **Fluxos alternativos e de exceção**:
  - `3a` Agente desconhecido: resultado de erro nomeado, sem sub-turno.
  - `3b` Profundidade > `MaxAgentDepth`: resultado de erro, sem recursão.
  - `4a` Sub-turno pausa por confirmação: devolve o `ChildSessionID` ao host (status awaiting_confirmation).
  - `4b` Sub-turno falha: resultado de erro da tool.
- **Regras de negócio**: RN-1 política do alvo aplicada (default deny); RN-2 sessão própria e isolada; RN-3 delegação auditada.
- **Critérios de aceite (Gherkin)**:
```gherkin
Cenário: delegação simples
  Dado um agente "resumo" e o provedor fake do pai devolvendo a tool call
  Quando o turno roda
  Então existe uma sessão do agente "resumo" e o resultado da tool contém o texto do sub-turno
```
- **Rastreabilidade**: FR-MA-001..003, 006, 007.

## CU-MA-2 — Profundidade máxima

- **Identificador**: CU-MA-2 · **Escopo**: motor · **Nível**: subfunção
- **Garantia de sucesso**: sem recursão infinita
- **Fluxo principal**: o sub-agente, se tentar delegar além de `MaxAgentDepth`, recebe erro na tool.
- **Rastreabilidade**: FR-MA-004.

## CU-MA-3 — Agente desconhecido e filtro de tools

- **Identificador**: CU-MA-3 · **Escopo**: motor/adaptador · **Nível**: subfunção
- **Garantia mínima**: nunca iniciar sub-turno inválido
- **Fluxo principal**: agente inexistente ⇒ erro nomeado; `AgentSpec.Tools` filtra o catálogo por glob; vazio herda.
- **Rastreabilidade**: FR-MA-005/006.

## Requisitos especiais (NFR)

- Determinismo: provedores/tools fake; nenhum teste toca rede.
- Isolamento: sessões de sub-agente separadas; `ParentSessionID` apenas para rastreio.
- Sem breaking change: `Config.Agents` vazio ⇒ comportamento anterior byte a byte.
