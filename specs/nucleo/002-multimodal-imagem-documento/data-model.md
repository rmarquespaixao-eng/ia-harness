# Data Model — 002 Multimodal (imagem/documento)

**Feature**: `specs/nucleo/002-multimodal-imagem-documento` | **Data**: 2026-09-16
Alterações sobre o modelo canônico de `specs/nucleo/001-harness-ia-reutilizavel/data-model.md`. O contrato de fio/persistência é gerado de JSON Schema (`contracts/<assunto>/*.json` → `contracts/gen`); as structs canônicas públicas ficam em `harness/session.go`.

## Part (estendida)

| Campo | Tipo | Regra |
|---|---|---|
| `kind` | `PartKind` | enum ampliado: `text`, `tool_call`, `tool_result`, **`image`**, **`document`** |
| `text` | string | só para `text` |
| `call` | `*ToolCall` | só para `tool_call` |
| `result` | `*ToolResult` | só para `tool_result` |
| `media` | `*Media` | **novo**; só para `image`/`document` |

Invariante: exatamente um campo populado conforme o `kind` (FR-MM-002).

## Media (novo)

| Campo | Tipo | Regra |
|---|---|---|
| `mime` | string | allowlist (R4): png/jpeg/webp/gif, application/pdf, text/plain, text/csv |
| `name` | string | opcional; nome do arquivo (não sensível) |
| `size_bytes` | int64 | tamanho efetivo da mídia |
| `bytes` | `[]byte` (JSON base64) | **xor** `reference` |
| `reference` | string | URL/ref fornecida pelo host; **xor** `bytes` |

Invariante: exatamente uma fonte (`bytes` xor `reference`); `size_bytes` > 0 e ≤ `max_media_bytes`.
Redação: `bytes` e `reference` nunca saem em log/auditoria (FR-MM-008).

## Capabilities (estendida)

| Campo | Tipo | Default | Papel |
|---|---|---|---|
| `tool_calling` | bool | false | (existente) |
| `streaming` | bool | false | (existente) |
| `max_context_tokens` | int | 0 | (existente) |
| `max_output_tokens` | int | 0 | (existente) |
| `vision` | bool | false | **novo** — aceita imagens |
| `documents` | bool | false | **novo** — aceita documento binário (PDF) |
| `max_media_bytes` | int64 | 0 (usa o global, default 5 MiB) | **novo** — teto por mídia |

`text/plain`/`text/csv` não exigem `documents` (caminho textual).

## Contrato JSON Schema (persistência/snapshot)

Arquivo: `contracts/session/session_snapshot.json`.
- `part.kind` enum += `image`, `document`.
- `part.properties.media` → `$ref: #/$defs/media`.
- `$defs/media`: `mime` (string, `pattern` da allowlist), `name` (string), `size_bytes` (integer, minimum 0), `bytes` (string, `contentEncoding: base64`), `reference` (string, `format: uri`), com `oneOf` exigindo exatamente uma fonte (`bytes` xor `reference`).
- Manter `additionalProperties: false`; `go generate` regenera `contracts/gen/session.go`; teste de conformidade (`contracts/conformance_test.go`) valida o marshal de uma Part de mídia.

> *Observação de tamanho:* o schema não limita `bytes` por `maxLength` (base64 grande estoura mensagem de validação); o teto é imposto em runtime (FR-MM-004) e coberto por teste de unidade.

## Impacto nos adaptadores

- **openai**: `wireMessage.Content` passa de `string` para `any`/array de partes; helper converte `PartText → {type:text}` e `PartImage/PartDocument → {type:image_url|file}`; mensagens mistas (texto + imagem) viram array; mensagens só-texto continuam string (compatibilidade).
- **anthropic**: `wireBlock` ganha `Source` (objeto) e os tipos `image`/`document`; `userBlocks`/`assistantBlocks` passam a emitir os blocos; `system`/tool permanecem.

## Fora do modelo (v1)

- Mídia em `ResultContent` (tool) — segue `text|json`.
- Áudio/vídeo.
