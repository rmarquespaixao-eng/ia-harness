# Tasks: Multi-agente — delegação entre agentes (022)

**Input**: `specs/nucleo/022-multi-agente/` (spec · plan · use-cases)
**Pré-requisitos**: plan aprovado. Cada task fecha com teste AAA e `make verify` verde.

## Fase 1 — Contratos aditivos

- [x] T2201 `internal/core/config.go`: `AgentSpec`, `Config.Agents`, `Config.MaxAgentDepth` — FR-MA-001/004
- [x] T2202 `internal/core/ports.go`: `AgentRunner`, `SubAgentRequest`, `SubAgentResult` — FR-MA-003
- [x] T2203 `internal/core/config.go`: `RunRequest.ParentSessionID` — FR-MA-008
- [x] T2204 `internal/core/events.go`: `SubAgentEvent` + `SubAgentHandler` — FR-MA-007
- [x] T2205 `harness/alias.go`: aliases públicos — API pública

## Fase 2 — Regra e fonte sintética

- [x] T2206 `internal/engine/agents/agents.go`: `FilterTools` (glob) + `DelegateToolSpec` + guard de profundidade — FR-MA-002/004/006
- [x] T2207 `internal/engine/agents/source.go`: `Source` (`ToolSource`) que chama `AgentRunner` — FR-MA-002/003/005
- [x] T2208 `internal/engine/agents/agents_test.go` + `source_test.go`: glob, schema, erros — CU-MA-2/3

## Fase 3 — Motor

- [x] T2209 `internal/engine/config_validate.go`: defaults (`MaxAgentDepth`) e validação dos specs — FR-MA-001/004
- [x] T2210 `internal/engine/engine.go`: injetar `agents.Source` no boot quando houver agentes — FR-MA-002/009
- [x] T2211 `internal/engine/engine.go`: `RunSubAgent` (sessão própria, modelo/prompt/política do agente) — FR-MA-003/006/007
- [x] T2212 `internal/engine/loop.go`: profundidade no contexto + `SubAgentEvent` — FR-MA-004/007
- [x] T2213 `internal/engine/engine.go`: resolver `AgentSpec` no turno (modelo/prompt/maxIterations) — FR-MA-001

## Fase 4 — Testes e fechamento

- [x] T2214 `harness` teste: orquestrador delega e recebe o texto do sub-agente — CU-MA-1
- [x] T2215 `harness` teste: profundidade máxima, agente desconhecido, filtro de tools — CU-MA-2/3
- [x] T2216 `make verify` verde + ADR 0027 + CHANGELOG + docs de feature — Constitution §7

## Dependências

- T2201→T2202→T2203→T2204→T2205; T2206→T2207→T2208; T2201→T2209→T2210→T2211→T2212/T2213; tudo→T2214–T2216.
- Depende de 003 (motor), 014 (middleware) e 018 (janela) — já entregues.

## Rastreabilidade — tasks × requisitos

| Requisito | Tasks |
|---|---|
| FR-MA-001 | T2201, T2213 |
| FR-MA-002 | T2206, T2207, T2210 |
| FR-MA-003 | T2202, T2207, T2211 |
| FR-MA-004 | T2201, T2206, T2212 |
| FR-MA-005 | T2207, T2208 |
| FR-MA-006 | T2206, T2211 |
| FR-MA-007 | T2204, T2211, T2212 |
| FR-MA-008 | T2203, T2211 |
| FR-MA-009 | T2210 |
| CU-MA-1/2/3 | T2214, T2215 |
