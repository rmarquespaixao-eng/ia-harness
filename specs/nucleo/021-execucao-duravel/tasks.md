# Tasks: Execução durável (021)

**Input**: `specs/nucleo/021-execucao-duravel/` (spec · plan · use-cases)
**Pré-requisitos**: plan aprovado. Cada task fecha com teste AAA e `make verify` verde.

## Fase 1 — Contrato e core

- [x] T2101 `contracts/session/session_snapshot.json`: `$defs/checkpoint` + `Session.checkpoint` — FR-DU-002
- [x] T2102 `go generate ./...` regenera `contracts/gen` sem diff — gate §7
- [x] T2103 `internal/core/session.go`: `TurnCheckpoint`/`CheckpointStatus`/`PendingCall` + `Session.Checkpoint` — FR-DU-002
- [x] T2104 `internal/core/config.go`: `Config.Durable` + `TurnResult.Resumed` — FR-DU-001/008
- [x] T2105 `internal/core/events.go`: `CheckpointEvent` + `CheckpointHandler` — FR-DU-008
- [x] T2106 `harness/alias.go`: aliases públicos — API pública

## Fase 2 — Motor durável

- [x] T2107 `internal/engine/checkpoint.go`: helpers de checkpoint (write-ahead, passo, conclusão) — FR-DU-001/002/009
- [x] T2108 `internal/engine/loop.go`: pontos de checkpoint quando `Durable` — FR-DU-001/002
- [x] T2109 `internal/engine/engine.go`: retomada de checkpoint `running` sem re-anexar input — FR-DU-003/006
- [x] T2110 `internal/engine/loop.go`: resultado ambíguo para não-idempotente; reexecuta idempotente — FR-DU-004/005
- [x] T2111 `internal/engine/checkpoint.go`: emitir `CheckpointEvent` — FR-DU-008

## Fase 3 — Testes e fechamento

- [x] T2112 `internal/engine/checkpoint_test.go`: write-ahead, conclusão, schema roundtrip — SC-DU
- [x] T2113 `harness` teste de turno: interrupção + retomada sem reexecutar tool não-idempotente — CU-DU-1/2
- [x] T2114 `harness` teste: `Durable=false` não grava checkpoint intermediário — CU-DU-3
- [x] T2115 `make verify` verde + ADR 0026 + CHANGELOG + docs de feature — Constitution §7

## Dependências

- T2101→T2102→T2103→T2104→T2105→T2106; T2103→T2107→T2108→T2109/T2110/T2111; tudo→T2112–T2115.

## Rastreabilidade — tasks × requisitos

| Requisito | Tasks |
|---|---|
| FR-DU-001 | T2104, T2107, T2108 |
| FR-DU-002 | T2101, T2103, T2107, T2108 |
| FR-DU-003 | T2109 |
| FR-DU-004 | T2110 |
| FR-DU-005 | T2110 |
| FR-DU-006 | T2109 |
| FR-DU-007 | T2104, T2110 |
| FR-DU-008 | T2105, T2106, T2111 |
| FR-DU-009 | T2107 |
| CU-DU-1/2/3 | T2112, T2113, T2114 |
