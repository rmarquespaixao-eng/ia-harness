# Research — 002 Multimodal (imagem/documento)

**Feature**: `specs/nucleo/002-multimodal-imagem-documento` | **Data**: 2026-09-16
Objetivo: decidir o formato de fio de cada provedor, o modelo de capacidade e os limites (resolve L-MM-1..4 do `spec.md`). Fontes são as documentações públicas das APIs; nenhum teste da feature usa rede real (fixtures/`httptest`).

## R1 — Formato de imagem por provedor

**OpenAI-compatible (Chat Completions)**: `messages[].content` passa de string para **array de partes**:

```json
[
  {"type": "text", "text": "o que é esta imagem?"},
  {"type": "image_url", "image_url": {"url": "data:image/png;base64,<...>", "detail": "auto"}}
]
```

`url` aceita tanto `https://…` quanto **data URI** (`data:<mime>;base64,<...>`). É o formato adotado por vLLM, OpenRouter, Groq, DeepSeek (onde houver visão) e LM Studio.
**Variação relevante — Ollama**: usa um campo **não padronizado** `messages[].images: ["<base64>"]` em vez de `image_url`. Fica como *não suportado no v1* pelo adaptador OpenAI-compatible (documentado; host usa o campo via `Params`/adaptador próprio ou um proxy). Decisão: o v1 cobre o formato OpenAI oficial; Ollama-imagem fica como evolução.
**Detalhe/texto**: `detail: "low"|"high"|"auto"` é opcional; default do provedor quando ausente.

**Anthropic (Messages)**: bloco dedicado, base64 ou URL:

```json
{"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": "<...>"}}
{"type": "image", "source": {"type": "url", "url": "https://…"}}
```

MIME aceitos: `image/jpeg`, `image/png`, `image/gif`, `image/webp`.

**Decisão (L-MM-3)**: Part de mídia carrega `mime`, `name` (opcional), `size_bytes` e **uma** fonte: `bytes` (base64/`[]byte`) **ou** `reference` (`url`). O adaptador traduz: bytes → data URI (OpenAI) / `source.base64` (Anthropic); url → `image_url.url` (OpenAI) / `source.url` (Anthropic).

## R2 — Formato de documento por provedor

**Anthropic**: documento nativo em bloco:

```json
{"type": "document", "source": {"type": "base64", "media_type": "application/pdf", "data": "<...>"}}
{"type": "document", "source": {"type": "text", "media_type": "text/plain", "data": "<...>"}}
```

**OpenAI Chat Completions**: bloco de arquivo existe para parte dos modelos/versões:

```json
{"type": "file", "file": {"file_data": "data:application/pdf;base64,<...>", "filename": "fatura.pdf"}}
```

Como a disponibilidade **varia por modelo/versão** (e por provedor compatível), o adaptador trata documento binário como **capacidade** (`documents`); se o perfil não declarar, o harness recusa (FR-MM-003) — nunca envia um campo que o endpoint pode ignorar silenciosamente.
**Decisão (L-MM-2)**: o v1 envia o bloco `file` quando `documents=true`; para provedores compatíveis sem suporte, o host converte o PDF para texto e anexa como `text/plain`/`text/csv` (caminho textual sempre suportado).

## R3 — Limites e tetos (resolve L-MM-1)

- Teto default por mídia: **5 MiB** (`max_media_bytes`), configurável por perfil e globalmente; acima disso, erro nomeado antes de I/O.
- Máximo de mídias por turno: **10** (configurável); excedente rejeitado.
- Limitação do provedor é do host/provedor; o harness aplica o teto **antes** de enviar, para não gastar chamada.
- `text/plain`/`text/csv` inline respeita o limite de texto da janela (`ContextPolicy`), com `truncated=true` quando cortado.

## R4 — MIME allowlist (resolve D-MM-4)

- Imagem: `image/png`, `image/jpeg`, `image/webp`, `image/gif`.
- Documento: `application/pdf`, `text/plain`, `text/csv`.
- Fora da allowlist (ex.: `image/tiff`, `application/vnd...sheet`): rejeitado com código estável; o host converte.

## R5 — Capacidade e fallback (resolve D-MM-2 / FR-MM-011)

`Capabilities` ganha `vision` (bool), `documents` (bool) e `max_media_bytes` (int64). O gate é aplicado no loop antes de montar o `ChatRequest`, no mesmo ponto em que hoje se checa `ToolCalling` (`harness/loop.go:263`). No fallback (`harness/provider_route.go`), um perfil só é elegível se declarar a capacidade exigida pelas partes do turno; senão é pulado — e, se nenhum elegível, erro explícito preservando a causa.

## R6 — Redação e janela

- Redação: bytes inline e URL de mídia entram na allowlist de redação (`harness/redact.go`); chaves `bytes`, `data`, `url`, `*_base64` → `[REDACTED]`. Metadados (`mime`, `name`, `size_bytes`) permanecem.
- Janela: a estimativa de tokens contabiliza mídia por heurística conservadora derivada de `size_bytes` (ex.: `ceil(size_bytes / bytes_per_token)` limitado por um teto de tokens-imagem), registrando a premissa no evento. Mídia antiga pode ser descartada pela `ContextPolicy` com registro (L-MM-4: descarte junto do truncamento `oldest`).

## R7 — Alternativas descartadas

- **Mídia em `ResultContent`** (tool) no v1: aumenta superfície e não é o caso de uso; o host textualiza.
- **Download de URL pelo harness**: responsabilidade do host (segurança/SSRF/credencial); o harness repassa a URL.
- **Campo `images` do Ollama**: fora do v1 (formato não padronizado).
- **Suporte a xlsx**: fora do v1 (conversão no host).

## Fontes

- OpenAI — Chat Completions, conteúdo multimodal (`content` array, `image_url`/`file`) e data URI: documentação pública da API, 2026.
- Anthropic — Messages API, blocos `image`/`document` (`source.base64`/`source.url`/`source.text`) e limites por request: documentação pública, 2026.
- OpenRouter / vLLM — `image_url` no formato OpenAI: documentação pública, 2026.
- Ollama — campo `images` na API `/api/chat`: documentação pública, 2026 (registrado como variação não suportada).
