# ADR 0019 — Deltas de streaming por interface opcional

**Status**: Accepted
**Data**: 2026-09-16
**Feature**: `specs/nucleo/012-streaming-deltas`

## Contexto

A UI precisava de argumentos de tool parciais e raciocínio (thinking) em tempo real; os adapters só entregavam texto incremental (`onText`) e descartavam o resto. Alterar `Provider.Chat` quebraria todos os adaptadores e consumidores.

## Decisão

Manter `Provider.Chat(ctx, req, onText)` e adicionar uma **interface opcional** `StreamingProvider{ChatStream(ctx, req, sink StreamSink)}` com `StreamSink{Text, Reasoning, ToolCallArgs}`. O engine checa o tipo em runtime: se o provider implementa, usa `ChatStream`; senão, `Chat` com `TextSink`. Os adapters implementam `ChatStream` e fazem `Chat` delegar (compatível). O `Handler` ganha `ReasoningDelta` e `ToolCallDelta` (com no-op em `NopHandler`).

## Alternativas consideradas

- **Trocar a assinatura de `Provider.Chat`**: quebra a API e todos os testes. Rejeitada.
- **Campos no `ChatRequest` para callbacks**: mistura dados com comportamento. Rejeitada.
- **Não expor reasoning/args**: não atende a UI. Rejeitada.

## Consequências

- Aditivo: providers antigos continuam funcionando; `Handler` ganha 2 métodos (hosts que embutem `NopHandler` não quebram).
- Deltas de args podem chegar antes do `CallID` (primeiros fragmentos) — documentado.
- Testes: `harness/streaming_test.go` e `openai/stream_sink_test.go`.
