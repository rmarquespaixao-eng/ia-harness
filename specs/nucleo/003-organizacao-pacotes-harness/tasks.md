# Tasks: Organização — fachada `harness` + motor `internal/` (003)

**Input**: `specs/nucleo/003-organizacao-pacotes-harness/` (spec · research · plan)
**Pré-requisitos**: plan aprovado no portão. Refactor **puro**: sem mudança de comportamento ou de API (FR-ORG-001/004). Cada task fecha com `go build`/`go test` verdes.
**Estratégia**: criar o destino, mover, ajustar imports, deletar o antigo — com o gate verde a cada passo.

## Fase 1 — Contrato folha

- [x] T201 Criar `internal/core/` movendo `session.go`, `ports.go`, `config.go`, `events.go`, `errors.go` (mesmo conteúdo, pacote `core`) — FR-ORG-002/003
- [x] T202 Teste de fronteira: `internal/core` não importa `internal/engine`/`harness`/`adapters` (via `go list -deps` no gate ou teste) — FR-ORG-003 · SC-ORG-002

## Fase 2 — Motor por preocupação

- [x] T203 Criar `internal/engine/` (raiz) com `engine.go` (estado/`New`) + mover `loop.go`, `config_validate.go` — FR-ORG-002
- [x] T204 Criar subpacotes e mover: `policy/` (`policy.go`,`policy_gate.go`), `routing/` (`provider_route.go`), `context/` (`window.go`,`memory_context.go`), `telemetry/` (`audit_emit.go`,`cost.go`,`redact.go`), `session/` (`session_snapshot.go`), `budget/` (`budget.go`) — FR-ORG-002 · research R3
- [x] T205 Ajustar assinaturas das funções movidas para receber `core.Config`/tipos explícitos (sem importar `engine`/`harness`) — FR-ORG-003

## Fase 3 — Fachada pública

- [x] T206 `harness/alias.go`: aliases de tipo para **todo** símbolo público hoje exportado (Session/Message/Part/Capabilities/Config/Provider/ToolSource/AuditSink/…/erros) — FR-ORG-001
- [x] T207 `harness/harness.go`: `New`/`Run`/`ResolveConfirmation`/`Session`/`Close` delegando a `internal/engine`; struct `Harness` opaca — FR-ORG-001
- [x] T208 Compilar consumidores sem trocar identificadores: `examples/financeiro/di.go` e `cmd/harnessctl` — SC-ORG-001

## Fase 4 — Testes e limpeza

- [x] T209 Mover testes de regra pura para os pacotes correspondentes do motor (mesmas asserções) — FR-ORG-005
- [x] T210 Mover/ajustar testes de API/CU para a fachada/engine (sem reescrever expectativa) — FR-ORG-004/005
- [x] T211 Remover os arquivos antigos da raiz `harness/` e imports obsoletos — FR-ORG-002
- [x] T212 `go generate ./...` sem diff (contrato intacto) — FR-ORG-006

## Fase 5 — Documentação, governança e grafo

- [x] T213 Emendar `constitution.md` §3 (Sync Impact Report, 1.1.0 → 1.2.0) espelhando a nova árvore — FR-ORG-007
- [x] T214 Atualizar `docs/features/nucleo-harness.md` (tabela `Arquivo | O que faz` e fluxo) e `README.md` (seção Estrutura) — FR-ORG-007
- [x] T215 Escrever ADR 0010 (`docs/adr/0010-fachada-harness-e-motor-interno.md`) com Status/Contexto/Decisão/Consequências — §8
- [ ] T216 `graphify update .` e conferir links dos caminhos alterados — FR-ORG-007
- [x] T217 `make verify` completo + suíte intacta — FR-ORG-008 · SC-ORG-003

## Dependências

- T201→T202; T201→T203→T204→T205; T203–T205→T206–T208; T206–T208→T209–T212; tudo→T213–T217.
- T204 [P] entre subpacotes (arquivos distintos).

## Rastreabilidade — tasks × requisitos

| Requisito | Tasks |
|---|---|
| FR-ORG-001 | T206, T207, T208 |
| FR-ORG-002 | T201, T203, T204, T211 |
| FR-ORG-003 | T202, T205 |
| FR-ORG-004 | T209, T210, T217 |
| FR-ORG-005 | T209, T210 |
| FR-ORG-006 | T212 |
| FR-ORG-007 | T213–T216 |
| FR-ORG-008 | T217 |
| SC-ORG-001/002/003/004 | T208, T202, T217, T214/T216 |

## Resultado

**Implementado (2026-09-16), `make verify` completo verde** (gofmt/vet/staticcheck/generate sem diff/test/build/govulncheck), suíte 001 intacta e consumidores (`examples/financeiro`, `cmd/harnessctl`) compilando sem trocar identificador público.

- **Árvore final:** `harness/` = fachada (`alias.go`, `harness.go`, `doc.go`) + testes de API/CU; `internal/core/` = contrato folha (`session,ports,config,events,errors`); `internal/engine/` = orquestração (`engine.go`, `loop.go`, `config_validate.go`, `policy_gate.go`, `provider_route.go`, `window.go`, `memory_context.go`, `audit_emit.go`, `session_reexport.go`, `aliases.go`) + subpacotes de regra pura `policy/`, `telemetry/`, `session/`, `budget/`.
- **Desvio documentado (T204/T205):** só as preocupações **puras e sem estado do `Harness`** viraram subpacotes. Roteamento de modelo, janela de contexto, memória no contexto e emissão de auditoria são **métodos de `Harness`** e permaneceram no pacote `engine`; movê-los exigiria converter métodos em funções e reescrever a suíte, sem ganho que justificasse o risco. Registrado no ADR 0010 e no `plan.md`/`docs/features`.
- **T216 (`graphify update .`) pendente** por não executar ferramentas de grafo nesta rodada.
- `T090` (tag `v0.1.0`) do harness segue pendente (ato do operador; repo sem commit/remote).

