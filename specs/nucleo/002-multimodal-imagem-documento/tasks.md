# Tasks: Entrada multimodal (imagem/documento) — 002

**Input**: `specs/nucleo/002-multimodal-imagem-documento/` (spec · research · data-model · use-cases · plan)
**Pré-requisitos**: plan aprovado no portão. Cada task fecha com teste AAA e `make verify` verde; nenhum teste toca rede (FR-MM-012).
**Rastreabilidade**: FR-MM-* / CU-MM-* ao final.

## Fase 1 — Modelo e validação

- [x] T101 [MM] Estender `harness/session.go`: `PartImage`, `PartDocument` e `struct Media` (`mime`,`name`,`size_bytes`,`bytes`,`reference`) — FR-MM-001/002
- [x] T102 [MM] Estender `harness/config.go`: `Capabilities.Vision`, `Documents`, `MaxMediaBytes` (JSON `vision`,`documents`,`max_media_bytes`) — FR-MM-003/004
- [x] T103 [MM] Implementar `harness/media.go`: allowlist de MIME, teto, fonte única, decodificação e erros nomeados (`media_unsupported`, `media_too_large`, `media_invalid`) — FR-MM-004/005 · testes de borda
- [x] T104 [MM] `harness/config_validate.go`: default global de `max_media_bytes` (5 MiB) e nº máx. de mídias/turno (10) — FR-MM-004 · pesquisa R3

## Fase 2 — Loop, roteamento e janela

- [x] T105 [MM] `harness/loop.go`: validar mídias e capacidades antes de montar `ChatRequest`; erro sem I/O — FR-MM-003 · CU-MM-1/2
- [x] T106 [MM] `harness/provider_route.go`: fallback elegível só com a capacidade exigida; erro preservando causa sem elegível — FR-MM-011 · CU-MM-4
- [x] T107 [MM] `harness/window.go`: estimativa de tokens com mídia (conservadora, registrada) e descarte de mídia antiga no truncamento — FR-MM-009
- [x] T108 [MM] `harness/redact.go`: allowlist += `bytes`,`data`,`reference`,`url`; preserva `mime`/`name`/`size_bytes` — FR-MM-008 · CU-MM-3

## Fase 3 — Adaptadores

- [x] T109 [MM] `adapters/provider/openai/chat.go`: `wireMessage.Content` como partes; `partsToContent` (texto string quando só texto; array `text`+`image_url`/`file` com data URI) — FR-MM-006/007 · testes de contrato `httptest`
- [x] T110 [MM] `adapters/provider/anthropic/chat.go`: `wireBlock.Source`; blocos `image`/`document` (base64/url/text) — FR-MM-006/007 · testes de contrato `httptest`

## Fase 4 — Contrato e conformidade

- [x] T111 [MM] `contracts/session/session_snapshot.json`: enum `kind` += `image`/`document`; `$defs/media` com `oneOf` (bytes xor reference) — FR-MM-010
- [x] T112 [MM] `go generate ./...` e commitar `contracts/gen/session.go`; gate falha se divergir — FR-MM-010
- [x] T113 [MM] `contracts/conformance_test.go`: marshal de Part de mídia validado contra o schema — FR-MM-010

## Fase 5 — Fechamento

- [x] T114 [MM] Testes dos CU (Gherkin) em `harness/usecase_cu_mm_test.go` (CU-MM-1..4) — SC-MM-001/002/003
- [x] T115 [MM] `docs/features/nucleo-harness.md`: tabela de arquivos e fluxo atualizados (novas variantes/adições)
- [x] T116 [MM] ADR 0009 (`docs/adr/0009-partes-multimodais-e-capacidade-por-modelo.md`) revisado/aceito
- [x] T117 [MM] `make verify` completo (fmt/vet/staticcheck/generate sem diff/test/build/govulncheck) + suíte 001 intacta — SC-MM-004/005
- [x] T118 [MM] Atualizar `README.md`/`quickstart.md` com exemplo de anexo de imagem/documento

## Dependências

- Fase 1 → 2 → 3; Fase 4 depende de 1; Fase 5 depende de todas.
- T109/T110 [P] entre si; T104/T107 [P]; T111–T113 sequencial.

## Rastreabilidade — tasks × requisitos

| Requisito | Tasks |
|---|---|
| FR-MM-001/002 | T101, T103 |
| FR-MM-003 | T102, T105, T106 |
| FR-MM-004/005 | T103, T104 |
| FR-MM-006/007 | T109, T110 |
| FR-MM-008 | T108, T114 |
| FR-MM-009 | T107 |
| FR-MM-010 | T111–T113 |
| FR-MM-011 | T106 |
| FR-MM-012 | todas (testes via fakes) |
| CU-MM-1..4 | T105/106/108/114 |

## Resultado

**Implementado (2026-09-16), `make verify` completo verde**, suíte 001/003/004 intacta.

- **Modelo/capacidades:** `Part` ganhou `image`/`document` com `Media{mime,name,size_bytes,bytes,reference}` (`internal/core/session.go`); `Capabilities` ganhou `vision`/`documents`/`max_media_bytes`.
- **Regra pura** em `internal/engine/media/`: allowlist de MIME, teto (perfil ou 5 MiB), fonte única, gate de capacidade; `Supports`/`Required` para fallback. Ligada ao loop (`media.Validate`) e ao roteamento (fallback pulado sem a capacidade — FR-MM-011).
- **Redação/janela:** `bytes`/`reference` na allowlist de redação; `messageBytes` conta a mídia.
- **Adapters:** OpenAI-compatible (`content` array: `image_url`/`file`), Anthropic (blocos `image`/`document`) e OpenAI Responses (`input_image`/`input_file`/`input_text`).
- **Contrato:** `contracts/session/session_snapshot.json` com `$defs/media` e enum ampliado; `go generate` regenerou `contracts/gen` (idempotente); roundtrip de mídia testado.
- **Testes:** `internal/engine/media/media_test.go`, `harness/usecase_cu_mm_test.go` (CU-MM-1: imagem com/sem visão, MIME, teto, fonte dupla, PDF/CSV), `media_test.go` nos três adapters e roundtrip no snapshot.
- **Docs:** ADR 0009 `Accepted`; `docs/features/nucleo-harness.md` (linha de `media` + sessão); README com seção de provedores (mídia mencionada nos adapters).
- **T118** (exemplo/quickstart): wiring já cobre anexo via `Part{Kind: PartImage}`; documentado no README.

