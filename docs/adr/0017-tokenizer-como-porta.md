# ADR 0017 — Tokenizer como porta (sem dependência no núcleo)

**Status**: Accepted
**Data**: 2026-09-16
**Feature**: `specs/nucleo/010-tokenizer-port`

## Contexto

A janela/orçamento usavam bytes/token; a contagem real depende do tokenizer do modelo. Embutir tiktoken (ou vários) no núcleo adicionaria dependência pesada e acoplamento a provedores.

## Decisão

Expor a porta `Tokenizer{Count(text) int}`. Quando `Config.Tokenizer` é injetado, `estimateWindowTokens` conta o payload real (texto, args de tool call, conteúdo de resultado); sem ele, mantém a heurística `Pricing.BytesPerToken`. O host pluga a implementação (ex.: tiktoken por modelo). A estimativa de **custo** permanece por volume (a porta é para a janela).

## Alternativas consideradas

- **Dependência de tiktoken no núcleo**: acopla a modelos OpenAI e engorda o binário. Rejeitada.
- **Heurística "melhorzinha" (split por chars)**: não é a contagem real. Rejeitada.

## Consequências

- Janela precisa quando o host fornece o tokenizer; comportamento anterior preservado sem a porta.
- Novo `Config.Tokenizer` (aditivo). Testes: `internal/engine/tokenizer_test.go`.
