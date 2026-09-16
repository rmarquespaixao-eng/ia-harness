# Implementation Plan: Organização — fachada `harness` + motor `internal/` (003)

**Branch**: `nucleo/003-organizacao-pacotes-harness` | **Date**: 2026-09-16 | **Spec**: [spec.md](./spec.md)

**Input**: `specs/nucleo/003-organizacao-pacotes-harness/` (spec + research)

## Summary

Refactor **puro** (sem mudança de comportamento/API) que dá parede física ao repositório: **contrato** em `internal/core`, **motor** em `internal/engine` com subpastas por preocupação, **fachada** fina no pacote raiz `harness` (aliases + `New/Run/ResolveConfirmation/Session/Close`). Resolve o ciclo de import por um pacote folha (`internal/core`) e aliases exportados (research R1). Decisão em **ADR 0010**; emenda MINOR da constitution §3.

## Technical Context

**Language**: Go 1.27 · **Dependencies**: nenhuma nova · **Storage**: n/a · **Testing**: suíte existente movida, sem reescrita · **Constraints**: API pública idêntica; direção de import acíclica; `make verify` verde · **Scale**: ~19 arquivos do núcleo.

## Constitution Check

| § | Requisito | Situação |
|---|---|---|
| §2 | Sem dependência nova; contrato gerado sem diff | OK |
| §3 | Arquitetura/estrutura | **Emenda MINOR** (§3 passa a espelhar a árvore nova) |
| §4/§5 | Segurança/observabilidade | Inalteradas (refactor mecânico) |
| §6 | API pública estável | **Aplica** — preservada por aliases (FR-ORG-001) |
| §7 | Testes sem rede; `make verify` | **Aplica** |
| §8 | SDD + ADR | **Aplica** — ADR 0010 |

*Pós-design*: sem violação; única alteração de governança é o Sync Impact Report da §3 (MINOR).

## Project Structure

### Árvore-alvo (source)

```text
harness/                     # FACHADA PÚBLICA (pacote harness)
├── harness.go               # asserts de API + New/Run/ResolveConfirmation/Session/Close (delega a internal/engine)
├── alias.go                 # type Session = core.Session; Config = core.Config; Provider = core.Provider; ...
└── doc.go                   # doc do pacote e mapa da árvore
internal/core/               # CONTRATO (folha): session.go, ports.go, config.go, events.go, errors.go
internal/engine/             # MOTOR (orquestração)
├── engine.go                # struct Engine/Harness (estado: cfg + portas) — construtor interno
├── loop.go                  # runTurn + retomada
├── config_validate.go
├── policy/                  # policy.go + policy_gate.go
├── routing/                 # provider_route.go
├── context/                 # window.go + memory_context.go
├── telemetry/               # audit_emit.go + cost.go + redact.go
├── session/                 # session_snapshot.go
├── budget/                  # budget.go
└── media/                   # (criado na feature 002)
adapters/*                   # ajuste só de import para a fachada (contrato)
contracts/                   # inalterado (schema + gen + conformidade)
```

### Documentation (feature)

```text
specs/nucleo/003-organizacao-pacotes-harness/
├── spec.md · research.md · plan.md · tasks.md
docs/adr/0010-fachada-harness-e-motor-interno.md
```

## Migração (mapa arquivo→destino)

| Hoje (`harness/`) | Destino |
|---|---|
| `session.go`, `ports.go`, `config.go`, `events.go`, `errors.go` | `internal/core/` (mesmos nomes) |
| `harness.go` | `harness/harness.go` (fachada) + `internal/engine/engine.go` (impl.) |
| `loop.go`, `config_validate.go` | `internal/engine/` |
| `policy.go`, `policy_gate.go` | `internal/engine/policy/` |
| `provider_route.go` | `internal/engine/routing/` |
| `window.go`, `memory_context.go` | `internal/engine/context/` |
| `audit_emit.go`, `cost.go`, `redact.go` | `internal/engine/telemetry/` |
| `session_snapshot.go` | `internal/engine/session/` |
| `budget.go` | `internal/engine/budget/` |
| `*_test.go` de regra pura | pacote correspondente do motor |
| `usecase_cu_har*`, `loop_test.go`, `nonet_test.go` | `harness/` (API) ou engine (orquestração) |

## Design

### Fachada (`harness/`)
- `alias.go`: `type (Session = core.Session; Message = core.Message; Part = core.Part; Config = core.Config; Provider = core.Provider; ... )` para **todos** os símbolos públicos hoje exportados pela raiz (mapa completo no ADR 0010/tasks T204).
- `harness.go`: `func New(cfg Config) (*Harness, error)`, `(*Harness).Run/ResolveConfirmation/Session/Close` delegando ao engine. `Harness` é struct opaca com campo `eng *engine.Engine` (não exportado).

### Motor (`internal/engine/...`)
- `engine.Engine` detém `core.Config`, portas e estado; `New` aplica defaults/validação; `Run` orquestra `loop`.
- Subpacotes puros recebem `core.Config`/tipos explícitos e devolvem decisões/estruturas (ex.: `policy.Evaluate(cfg, agentID, tool)`, `context.BuildMessages(...)`, `telemetry.Redactor`). Não importam `harness` nem `engine` (evita ciclo).

### Contrato (`internal/core/`)
- Só tipos/portas/config/eventos/erros; **sem** lógica de I/O. Pode importar `contracts/gen` (persistência) — como hoje.

### Verificação de fronteira
- Teste `internal/core/imports_test.go` (ou `go list -deps` no gate) garantindo que `internal/core` não importa `internal/engine`/`harness`/`adapters`, e `harness` não importa `adapters`.

## Complexity Tracking

| Decisão | Alternativa | Por quê |
|---|---|---|
| Contrato em `internal/core` + aliases | contrato público `harness/contract` | mantém um único pacote público estável |
| Subpacotes por preocupação | `internal/engine` plano | atende "layout organizado" com pastas visíveis |
| Refactor isolado (003) | junto da 002 | auditoria limpa (D-ORG-4) |

## Gate

`make verify` verde + consumidores (`examples/financeiro`, `cmd/harnessctl`) compilando sem trocar identificadores + fronteira de import checada + docs/grafo atualizados. **Sem código antes da aprovação.**
