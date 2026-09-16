# Tasks: OpenCode Zen/Go (004)

**Input**: `specs/nucleo/004-provider-opencode-zen-go/` (spec · research · plan)
**Pré-requisitos**: plan aprovado. Cada task fecha com teste AAA e `make verify` verde; nenhum teste toca rede (FR-ZG-008).

## Fase 1 — Config-only (famílias já cobertas)

- [x] T301 Documentar em `README.md` e `docs/features/nucleo-harness.md` o wiring Zen/Go para `/chat/completions` (adapter openai) e `/messages` (adapter anthropic) — FR-ZG-001
- [x] T302 `examples/financeiro/di.go`: adicionar perfis de exemplo Zen/Go (chat e messages) sem segredo — FR-ZG-007

## Fase 2 — Adapter Responses

- [x] T303 Criar `adapters/provider/openai_responses/chat.go` (Config/Deps/New/Chat/authorize/logCall) espelhando `provider/openai` — FR-ZG-002
- [x] T304 Criar `adapters/provider/openai_responses/stream.go`: parser SSE (`output_text.delta`, `function_call_arguments.delta`, `completed.usage`), tolerante a eventos desconhecidos — FR-ZG-003/FR-ZG-006
- [x] T305 Criar `adapters/provider/openai_responses/wire.go`: request (`input`, `tools`, `store=false`, `stream=true`) e mapeamento de histórico/item ↔ canônico — FR-ZG-002
- [x] T306 Implementar `SessionHeader`/`User-Agent` (headers fixos + sessão por request) — FR-ZG-004
- [x] T307 Testes de contrato `httptest`: SSE completo com tool call, usage ausente→estimado, stream cortado, evento desconhecido, 429/timeout — FR-ZG-006/FR-ZG-008
- [x] T308 Teste de extensibilidade: novo adapter plugado por DI sem tocar no núcleo — FR-ZG-002/SC-ZG-002

## Fase 3 — Integração com 002/003

- [ ] T309 Aplicar o gate `Capabilities.Vision/Documents` no adapter Responses quando a feature 002 estiver presente (sem quebrar texto) — FR-ZG-009

## Fase 4 — Documentação e fechamento

- [x] T310 Escrever `docs/adr/0011-adapter-openai-responses-e-opencode-zen-go.md` (Status/Contexto/Decisão/Consequências) — §8
- [x] T311 Atualizar `docs/features/nucleo-harness.md` (tabela de arquivos + seção "Provedores") e o `CHANGELOG` — FR-ZG-007
- [x] T312 Escrever runbook `docs/runbooks/AAAA-MM-DD-smoke-zen-go.md` (smoke real do operador com `gpt-5.6-luna`, chave fora do repo) — FR-ZG-007
- [x] T313 `make verify` completo (fmt/vet/staticcheck/generate sem diff/test/build/govulncheck) + suíte intacta — SC-ZG-003/SC-ZG-004

## Dependências

- T303→T304→T305→T306→T307; T303→T308; T309 depende da feature 002; T310–T313 fecham.
- T301/T302 [P]; T304/T305 [P] parcialmente.

## Rastreabilidade — tasks × requisitos

| Requisito | Tasks |
|---|---|
| FR-ZG-001 | T301, T302 |
| FR-ZG-002 | T303, T305, T308 |
| FR-ZG-003 | T304 |
| FR-ZG-004 | T306 |
| FR-ZG-005 | T303 (CredentialProvider) |
| FR-ZG-006 | T304, T307 |
| FR-ZG-007 | T301, T302, T311, T312 |
| FR-ZG-008 | T307, T313 |
| FR-ZG-009 | T309 |
| SC-ZG-001 | T312 (smoke operador) |
| SC-ZG-002 | T308 |
| SC-ZG-003/004 | T313 |

## Resultado

**Implementado (2026-09-16), `make verify` completo verde**, suíte existente intacta.

- **Config-only documentado** (T301/T302): `README.md` (nova seção "Provedores: OpenCode Zen e Go"), `docs/features/nucleo-harness.md` (tabela + nota) e `examples/financeiro/di.go` com os três perfis (`opencode-go-chat`, `opencode-go-messages`, `opencode-go-responses`), sem segredo.
- **Adapter novo** `adapters/provider/openai_responses/` (`chat.go`, `wire.go`, `stream.go`) + testes `httptest` (stream com texto/tool/usage, usage ausente→estimado, stream cortado, evento desconhecido ignorado, 429, headers de sessão/UA, cancelamento) e teste interno de `buildBody`.
- **`ChatRequest.SessionID`** (aditivo) + **`Config.SessionHeader`** nos três adapters (`openai`, `anthropic`, `openai_responses`): `x-opencode-session` preenchido com o `SessionID` do turno (FR-ZG-004), sem expor `net/http`.
- **ADR 0011** `Accepted`; runbook `docs/runbooks/2026-09-16-smoke-zen-go.md` preparado (operador).
- **T309 pendente** por depender da feature 002 (gate de mídia no adapter Responses).
- **T312 (smoke real) pendente de execução do operador.** `T090` (tag `v0.1.0`) segue pendente.

