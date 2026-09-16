# Feature Specification: Structured output (saída validada por JSON Schema)

**Feature Branch**: `nucleo/009-structured-output`

**Status**: Aprovado e implementado

**Input**: lacuna P0 — o harness só passava `response_format` por `Params` cru, sem validação nem garantia.

## 1. Problema

Casos como extrair dados de uma fatura exigem saída estruturada e validada. A comunidade trata isso como diferencial central (Pydantic AI). O harness não tinha campo de schema de saída nem validação do resultado.

## 2. Requisitos (EARS)

- **FR-OUT-001**: WHEN `RunRequest.OutputSchema` está presente, o harness SHALL propagá-lo em `ChatRequest.OutputSchema`.
- **FR-OUT-002**: O adaptador OpenAI-compatible MUST enviar `response_format: {type: json_schema, json_schema:{name, schema, strict}}`; o adaptador Responses MUST enviar `text: {format: {json_schema...}}`. (Anthropic sem json_schema nativo no v1.)
- **FR-OUT-003**: Quando o turno conclui sem tool calls e há `OutputSchema`, o harness MUST validar o texto final contra o schema; divergência MUST virar `*OutputError` (código `output/schema-invalido`) preservando a causa.
- **FR-OUT-004**: Schema de saída inválido MUST falhar rápido (`ConfigError` `run/output-schema-invalido`) antes de chamar o provedor.
- **FR-OUT-005**: Sem `OutputSchema`, o comportamento é idêntico ao anterior.

## 3. Critérios de aceite (Gherkin)

```gherkin
Cenário: saída válida
  Dado um RunRequest com OutputSchema
  E o modelo responde JSON dentro do schema
  Quando o turno conclui
  Então o schema foi enviado ao provedor
  E o turno conclui sem erro

Cenário: saída inválida
  Dado um RunRequest com OutputSchema
  E o modelo responde fora do schema
  Quando o turno conclui
  Então retorna *OutputError (output/schema-invalido)
```

## 4. Não-objetivos

- Auto-reparo (re-prompt com o erro) — evolução.
- Anthropic `json_schema` nativo (não existe no v1).
- Saída estruturada em tool results.
