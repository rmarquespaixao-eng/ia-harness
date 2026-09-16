# ADR 0016 — Structured output por porta de schema no turno

**Status**: Accepted
**Data**: 2026-09-16
**Feature**: `specs/nucleo/009-structured-output`

## Contexto

Sem saída estruturada validada, extrair dados (fatura → JSON) dependia de `Params` cru e de validação manual do host. A comunidade trata saída tipada como central.

## Decisão

`RunRequest.OutputSchema` (JSON Schema) → `ChatRequest.OutputSchema`. Os adaptadores OpenAI-compatible e Responses mapeiam para `response_format`/`text.format` (`json_schema`, `strict`). O engine compila o schema uma vez por turno e valida o texto final quando não há tool calls; divergência vira `*OutputError` (`output/schema-invalido`), schema inválido falha rápido (`ConfigError`). Anthropic sem json_schema nativo no v1 (documentado).

## Alternativas consideradas

- **Só `Params`**: sem validação nem contrato. Rejeitada.
- **Auto-reparo com re-prompt**: mais uma volta no modelo; adiado (o host pode reexecutar com o erro).
- **Tool-forcing na Anthropic**: funciona, mas muda a semântica do turno; adiado.

## Consequências

- Saída tipada e verificada; erro nomeado e causa preservada.
- Aditivo: sem `OutputSchema`, nada muda. Anthropic não impõe o schema no provedor (só o host valida).
- Testes: `harness/output_schema_test.go` (válido/inválido) e schema no body dos adapters.
