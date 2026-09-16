# Implementation Plan: Entrada multimodal (imagem/documento) condicionada ao modelo (002)

**Branch**: `nucleo/002-multimodal-imagem-documento` | **Date**: 2026-09-16 | **Spec**: [spec.md](./spec.md)

**Input**: `specs/nucleo/002-multimodal-imagem-documento/` (spec + research + data-model + use-cases)

## Summary

Estender o modelo canônico e os dois adaptadores de provedor para aceitar **imagens** (png/jpeg/webp/gif) e **documentos** (PDF nativo; texto/CSV inline) como partes de mensagem do usuário, com **gate por capacidade do modelo** (`vision`/`documents`/`max_media_bytes`), **allowlist de MIME**, **teto de tamanho** e **redação invariável** de bytes/URL. O caminho de texto existente permanece intacto. Contrato de snapshot versionado + `go generate` idempotente.

**Abordagem técnica (research R1–R6):** parte de mídia com fonte dupla (`bytes` xor `reference`); adaptador OpenAI-compatible emite `content` array (`image_url`/`file`) e o Anthropic emite blocos `image`/`document`; gate de capacidade no loop antes de montar o `ChatRequest`; fallback só para perfil com a capacidade exigida. Decisão registrada em **ADR 0009**.

## Technical Context

**Language/Version**: Go 1.27 (mesmo módulo `rmarquespaixao/ia-harness`) · **Dependencies**: nenhuma nova · **Storage**: n/a (contrato = JSON Schema gerado) · **Testing**: `testing` + `testify`, fakes/`httptest` (sem rede) · **Target**: biblioteca embutida · **Project Type**: library · **Constraints**: sem `net/http` na API pública; contrato gerado sem diff no gate; redação invariável · **Scale**: 2 adaptadores, N perfis de modelo.

## Constitution Check

| § | Requisito | Situação |
|---|---|---|
| §2 | Contrato = JSON Schema gerado, `go generate` sem diff | **Aplica** — schema de sessão alterado e regenerado |
| §3 | Núcleo público; adaptador importa o núcleo; sem `net/http` público | OK — só tipos/adaptações |
| §4 | Redação por allowlist; nenhum binário/segredo em log | **Aplica** — FR-MM-008 |
| §5 | Evento por chamada com metadados | OK — mídia vira metadado |
| §6 | API pública aditiva e estável; eventos versionáveis | **Aplica** — campos/enum aditivos (MINOR) |
| §7 | Testes sem rede; `make verify` | **Aplica** — FR-MM-012 |
| §8 | SDD com portão; decisão → ADR | **Aplica** — ADR 0009 |

*Pós-design*: sem violação; nenhuma exceção/ADR de waiver. Mudança é **aditiva** (MINOR no contrato da biblioteca).

## Project Structure

### Documentation (feature)

```text
specs/nucleo/002-multimodal-imagem-documento/
├── spec.md · research.md · data-model.md · use-cases.md · plan.md · tasks.md
└── contracts/            # referência ao contrato do repo (contracts/session/*)
docs/adr/0009-partes-multimodais-e-capacidade-por-modelo.md   # decisão
```

### Source Code (repository root)

```text
harness/
├── session.go            # Part: kinds image/document + struct Media
├── config.go             # Capabilities: vision, documents, max_media_bytes
├── config_validate.go    # default de max_media_bytes global
├── media.go              # (novo) validação allowlist/teto/fonte única + helpers
├── loop.go               # gate de capacidade + injeção da mídia no ChatRequest
├── provider_route.go     # fallback elegível só com a capacidade exigida
├── redact.go             # chaves de mídia (bytes/data/url/reference) na allowlist
└── window.go             # estimativa de tokens com mídia + descarte registrado
adapters/provider/openai/
├── chat.go               # wireMessage.Content any + partsToContent (image_url/file)
└── stream.go             # inalterado (resposta é texto/tool)
adapters/provider/anthropic/
└── chat.go               # wireBlock.Source + blocos image/document
contracts/session/session_snapshot.json   # enum kind + $defs/media
contracts/gen/session.go                  # regenerado por go generate
```

## Design

### 1. Modelo (`harness/session.go`, `config.go`)
- `PartKind` += `PartImage`, `PartDocument`; `Part.Media *Media`.
- `type Media struct { MIME, Name string; SizeBytes int64; Bytes []byte; Reference string }` (JSON: `mime`,`name`,`size_bytes`,`bytes`,`reference`).
- `Capabilities` += `Vision`, `Documents`, `MaxMediaBytes`.

### 2. Validação (`harness/media.go`, novo)
- `validateMedia(cfg, caps, part) error`: MIME na allowlist; `size_bytes` ≤ teto (perfil senão global); exatamente uma fonte; decodificação de `bytes` quando inline; `*ConfigError`/novo `*MediaError` nomeado (código estável `media_*`).
- Gate por tipo: imagem exige `Vision`; PDF exige `Documents`; textual não exige.

### 3. Loop (`harness/loop.go`)
- Antes de montar `ChatRequest`, validar mídias e capacidades; erro explícito sem I/O.
- Passar as Parts de mídia no `ChatRequest.Messages` (já vai inteiro).

### 4. Roteamento (`harness/provider_route.go`)
- `profileSupportsMedia(profile, requiredCaps)`; fallback pula perfil inelegível; sem elegível → erro preservando a causa.

### 5. Adaptadores
- **OpenAI**: `partsToContent(m Parts) any` — só texto → `string` (compatível); com mídia → `[]any{ {"type":"text",...}, {"type":"image_url","image_url":{"url": ...}}, {"type":"file","file":{...}} }`; bytes → data URI.
- **Anthropic**: `wireBlock` ganha `Source *wireSource{Type,MediaType,Data,URL}`; `userBlocks` emite `image`/`document`; texto/CSV inline como bloco `text`.

### 6. Redação (`harness/redact.go`)
- Allowlist += `bytes`, `data`, `reference`, `url` (valor `[REDACTED]`); preserva `mime`, `name`, `size_bytes`.

### 7. Janela (`harness/window.go`)
- `estimateWindowTokens` soma mídia por heurística conservadora sobre `size_bytes`; mídia antiga descartada junto do truncamento `oldest` com registro.

### 8. Contrato
- Atualizar `contracts/session/session_snapshot.json` (enum + `$defs/media`); `go generate ./...` regenera `contracts/gen/session.go`; ampliar `contracts/conformance_test.go` com Part de mídia.

## Complexity Tracking

| Decisão | Alternativa | Por quê |
|---|---|---|
| Fonte dupla (`bytes` xor `reference`) | só bytes / só URL | host escolhe; PDF/imagem por URL nem sempre é aceito igual entre provedores |
| Gate por capacidade | inferir suporte pelo provider | fiel ao FR-015; evita campo ignorado em silêncio |
| Mídia em `Part` do usuário | mídia em `ResultContent` | caso de uso é entrada; reduz superfície |

## Gate

`make verify` verde (fmt/vet/staticcheck/generate sem diff/test/build/govulncheck) + suíte 001 intacta (SC-MM-005). **Sem código antes da aprovação deste plan/tasks.**
