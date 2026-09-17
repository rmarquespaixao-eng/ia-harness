# ADR 0026 — Execução durável por checkpoint

**Status**: Accepted
**Data**: 2026-09-17
**Feature**: `specs/nucleo/021-execucao-duravel`

## Contexto

Um turno longo pode ter efeitos colaterais (tools de escrita) e, se o processo cair no meio, o host não sabe onde parou: retomar do zero repetiria efeitos já aplicados. Já existe persistência de sessão (`SessionStore` + snapshot), então a durabilidade do turno deve reaproveitá-la em vez de introduzir outra porta. A constituição §6 proíbe reexecutar tool não-idempotente.

## Decisão

1. `Config.Durable` liga checkpoint **por passo** no snapshot da sessão, no mesmo `SessionStore` — sem porta nova.
2. `Session.Checkpoint` (`turn_id`, `status` `running`/`awaiting_confirmation`/`completed`, `step`, `pending_call`) é adicionado ao contrato `contracts/session/session_snapshot.json` como campo **opcional** (retrocompatível) e mapeado em `internal/engine/session/session_snapshot.go`.
3. **Write-ahead** antes de cada tool: grava-se a intenção antes de executar.
4. `Run` retoma checkpoint `running` sem re-anexar input (`TurnResult.Resumed`).
5. `reconcileCheckpoint` resolve a interrupção entre o write-ahead e a execução: tool não-idempotente **não** é reexecutada (injeta resultado "resultado ambíguo"); a idempotente é reexecutada.
6. Cancelamento mantém o status `running`; falha de `Save` no checkpoint **aborta o turno** (não é best-effort).
7. `CheckpointEvent` opcional (type assertion) publica o progresso do passo.

## Alternativas consideradas

- **Gravar só no fim do turno**: perde turnos longos e permite repetir efeito após queda. Rejeitada.
- **Porta `TurnStore` separada**: duplica a persistência de sessão. Rejeitada.
- **Sempre durável (sem flag)**: impõe I/O de checkpoint a todo host. Rejeitada.
- **Reexecutar sempre na retomada**: proibido pela constituição §6 para não-idempotentes. Rejeitada.

## Consequências

- Turnos sobrevivem a queda/restart, com retomada segura e sem repetição de efeito.
- Muda o contrato de persistência (aditivo/opcional; gate de `go generate` sobre o JSON schema).
- Dialoga com a reconciliação de escrita do financeiro: o "resultado ambíguo" bloqueia retry até decisão.
