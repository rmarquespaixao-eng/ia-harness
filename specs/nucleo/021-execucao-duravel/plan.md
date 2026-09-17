# Plan: Execução durável (021)

**Feature**: `specs/nucleo/021-execucao-duravel/` · **Status**: Aprovado

Constitution: **não viola** nenhuma seção. Aditivo na API pública; **muda o contrato de persistência** (`session_snapshot.json` ganha `checkpoint`), regenerado por `cmd/contractgen` com gate de `go generate`.

## Arquitetura

1. **Contrato** (`contracts/session/session_snapshot.json`): `$defs/checkpoint` com `turn_id`, `status` (enum), `step`, `updated_at`, `pending_call` (opcional) e `Session.checkpoint` opcional. `go generate` regenera `contracts/gen/session.go`.
2. **Core** (`internal/core/session.go`): `TurnCheckpoint`, `CheckpointStatus`, `PendingCall`; `Session.Checkpoint *TurnCheckpoint`; `TurnResult.Resumed bool`.
3. **Config** (`internal/core/config.go`): `Config.Durable bool`.
4. **Motor** (`internal/engine`):
   - `checkpoint.go`: helpers `beginTurn`/`checkpointStep`/`beginToolCall`/`endToolCall`/`completeTurn`, sempre via `Sessions.Save` (usa `context.WithoutCancel` no cancelamento).
   - `loop.go`: chama os helpers nos pontos duráveis quando `Durable`.
   - `engine.go`: `Run` detecta checkpoint `running` e entra em modo retomada (não anexa `req.Input` se vazio; se `Input` vier com retomada, anexa como novo turno e zera o checkpoint).
   - Resultado ambíguo: em `executeTool`, se existir `pending_call` no checkpoint e não houver resultado no histórico, não reexecuta não-idempotente (FR-DU-004).
5. **Observabilidade** (`internal/core/events.go`): `CheckpointEvent{SessionID, TurnID, Status, Step}` + `CheckpointHandler` opcional.
6. **Adapter inmem** já persiste o snapshot — passa a persistir o checkpoint sem mudança (mesma struct).

## Contratos (aditivos)

- `TurnCheckpoint`, `CheckpointStatus`, `PendingCall`, `Config.Durable`, `TurnResult.Resumed`, `CheckpointEvent`, `CheckpointHandler`; aliases em `harness/alias.go`.
- Schema `session_snapshot.json` v2 (campo opcional — retrocompatível com snapshots sem checkpoint).

## Alternativas consideradas

- **Só gravar no fim (atual)**: perde turnos longos e permite repetição de efeito. Rejeitada.
- **Checkpoint em store separado (`TurnStore`)**: duplica a porta de persistência e o host teria de correlacionar; o snapshot já é a unidade retomável. Rejeitada.
- **Sempre durável**: custo de I/O a cada passo mesmo para hosts simples; ligado por flag. Rejeitada a imposição.
- **Reexecutar sempre na retomada**: viola §6 (não-idempotente). Proibido.

## Riscos

- Mudança de schema: coberta pelo gate de geração e por teste de conformidade; campo opcional mantém snapshots antigos válidos.
- Duplicação de passos: mitigada pelo write-ahead + busca de resultado por `call_id` no histórico.
- `Save` no meio do turno pode ser lento no host: documentado; flag controla.
