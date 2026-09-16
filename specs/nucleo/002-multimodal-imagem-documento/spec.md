# Feature Specification: Entrada multimodal (imagem, documento e texto) condicionada à capacidade do modelo

**Feature Branch**: `nucleo/002-multimodal-imagem-documento`

**Created**: 2026-09-16

**Status**: Draft (**aguardando aprovação no portão SDD** — nada de código antes)

**Input**: "o harness tem que suportar imagem e documentos também, de acordo com o modelo" (decisão do operador após auditoria de compatibilidade dos provedores).

**Depende de**: `specs/nucleo/001-harness-ia-reutilizavel/` (núcleo v0.1.0). Reabre o portão por ser capacidade nova de tipo de conteúdo.

## 1. Problema

O núcleo hoje só entende **texto** em mensagens: `Part` tem as variantes `text`, `tool_call` e `tool_result` (`harness/session.go:21-25`), e os adaptadores serializam o conteúdo como string (`openai/chat.go:339` `Content string`; `anthropic/chat.go:370` system concatenado). Consequência: o host não consegue mandar uma **imagem** (foto de nota, print de fatura) nem um **documento** (PDF, CSV) para o modelo, embora os provedores-alvo já aceitem conteúdo multimodal.

O caso de uso real do financeiro: o usuário anexa o PDF/imagem de uma fatura ou comprovante e pergunta "esse valor bate com o que está lançado?". Sem conteúdo multimodal, o host teria que extrair texto fora do harness (OCR/parse) e perderia a fidelidade que os próprios modelos já oferecem.

## 2. User Scenarios & Testing *(mandatory)*

### User Story 1 - Anexar imagem e perguntar sobre ela (Priority: P1)

O usuário final anexa uma imagem (PNG/JPEG/WebP/GIF) na mensagem; o harness valida o MIME/tamanho, confere que o perfil de modelo corrente declara visão e envia a imagem no formato nativo do provedor; o modelo responde sobre a imagem.

**Why this priority**: é a metade mais comum do pedido (print/foto) e o menor caminho até valor; exercita o gate por capacidade.

**Independent Test**: com provedor fake que registra o payload, anexar uma imagem suportada e verificar que o adaptador recebeu um bloco de imagem no formato nativo; repetir com um perfil sem `Vision` e verificar falha explícita **sem** chamada ao provedor.

**Acceptance Scenarios**:

1. **Given** um perfil com `capabilities.vision=true`, **When** o usuário envia uma imagem PNG dentro do teto, **Then** o provedor recebe a imagem no formato nativo e o turno conclui.
2. **Given** um perfil com `vision=false` (default), **When** o usuário envia imagem, **Then** o harness falha com erro nomeado e **não** chama o provedor nem corrompe a sessão.
3. **Given** uma imagem com MIME fora da allowlist (ex.: `image/tiff`) ou acima do teto, **When** o harness valida a entrada, **Then** rejeita com erro nomeado antes de qualquer I/O de modelo.

### User Story 2 - Anexar documento (PDF, texto/CSV) (Priority: P1)

O usuário anexa um documento; o harness aceita PDF nativo e arquivos textuais (texto/CSV), mapeando para o formato nativo do provedor quando houver suporte, ou inline textual quando o MIME for textual.

**Independent Test**: com provider fake, anexar `application/pdf` e `text/csv` e verificar o mapeamento (documento nativo vs. texto inline) por adaptador; planilha binária (xlsx) é tratada como fora do v1 (o host converte para CSV/texto).

**Acceptance Scenarios**:

1. **Given** perfil com `capabilities.documents=true`, **When** o usuário anexa um PDF dentro do teto, **Then** o provedor recebe o bloco de documento nativo.
2. **Given** um arquivo `text/csv`, **When** o harness monta o contexto, **Then** o conteúdo textual é inlined como texto (funciona mesmo sem `documents=true`, respeitando a allowlist textual).
3. **Given** um arquivo binário não suportado (ex.: xlsx), **When** o harness valida, **Then** rejeita com código estável e mensagem que orienta a conversão no host.

### User Story 3 - Redação e auditoria seguras de conteúdo binário (Priority: P2)

Bytes inline e URL/referência de mídia nunca vazam para log, auditoria ou telemetria; apenas metadados não sensíveis (MIME, nome, tamanho, presença) aparecem. A estimativa de tokens da janela contabiliza a mídia de forma conservadora e registrada.

**Independent Test**: anexar mídia com um valor-isca base64 e verificar que o evento de auditoria/log não contém o payload, e que a estimativa de contexto inclui a mídia.

**Acceptance Scenarios**:

1. **Given** uma Part de imagem com bytes inline, **When** o `AuditEvent`/log é emitido, **Then** o payload aparece como `[REDACTED]` e o `mime`/`size_bytes` permanecem.
2. **Given** uma Part de mídia por URL/referência, **When** o evento é emitido, **Then** a URL (possivelmente assinada) é redigida.
3. **Given** contexto próximo do limite com mídia anexada, **When** a janela é aplicada, **Then** a mídia conta na estimativa e a decisão fica registrada.

### User Story 4 - Contrato e providers versionados (Priority: P2)

As novas partes entram no contrato de persistência (`contracts/session/*.json`) e nos tipos gerados, com teste de conformidade e `go generate` idempotente; os dois adaptadores mapeiam os novos tipos sem quebrar o caminho de texto.

**Independent Test**: `make verify` verde; marshal de sessão com partes de mídia validado contra o schema; `go generate ./...` sem diff.

**Acceptance Scenarios**:

1. **Given** uma sessão com partes `image`/`document`, **When** o snapshot é validado, **Then** bate com o schema atualizado e o teste de conformidade passa.
2. **Given** o schema alterado, **When** `go generate ./...` roda, **Then** o gerado muda de forma determinística e o gate falha se não for commitado.

### Edge Cases

- **Mídia e fallback**: o fallback de modelo só é permitido se o fallback também declarar a capacidade de mídia exigida; caso contrário, degradar para o primário ou falhar explícito — nunca enviar mídia a um perfil que não suporta.
- **Sessão retomada com mídia**: o snapshot deve preservar a referência/bytes para reenvio no próximo turno, respeitando o teto ao restaurar.
- **Base64 inválido**: bytes que não decodificam ou MIME declarado divergente do sniff → erro nomeado antes do provider.
- **Mídia em `tool_result`**: fora do v1 — resultado de tool continua texto/JSON (o host converte binário para texto/referência).
- **Cancelamento/estouro de contexto com mídia**: mídia não é reenviada indefinidamente; a janela pode descartar mídia antiga com registro.
- **URL inacessível ao provider**: erro do provider é classificado e devolvido ao host; o harness não faz download por conta própria no v1.

## 3. Requirements *(mandatory)*

### Functional Requirements (EARS)

- **FR-MM-001**: O `Part` MUST suportar as variantes `image` e `document` além de `text`/`tool_call`/`tool_result`, com mídia carregando `mime` (allowlist), `name` (opcional) e `size_bytes`.
- **FR-MM-002**: WHEN uma Part de mídia é declarada, o harness SHALL exigir **exatamente uma** fonte: `bytes` inline (base64/`[]byte`) **ou** uma referência (`url`/`ref` fornecida pelo host).
- **FR-MM-003**: IF o perfil de modelo corrente não declarar a capacidade correspondente (`vision` para imagem; `documents` para documento binário) THEN o harness SHALL falhar com erro nomeado, sem chamar o provedor e sem alterar a sessão.
- **FR-MM-004**: WHEN a mídia excede o teto configurado (`Capabilities.max_media_bytes` ou config global), o harness SHALL rejeitar antes de qualquer I/O.
- **FR-MM-005**: WHEN o MIME da mídia não está na allowlist (imagem: `image/png|jpeg|webp|gif`; documento: `application/pdf`, `text/plain`, `text/csv`), o harness SHALL rejeitar com código estável.
- **FR-MM-006**: O adaptador OpenAI-compatible MUST mapear imagem para `image_url` (data URI de bytes ou URL) e documento suportado para o bloco de arquivo equivalente; o adaptador Anthropic MUST mapear para blocos `image`/`document` (base64 ou URL).
- **FR-MM-007**: Mídia textual (`text/plain`, `text/csv`) MUST ser inlined como texto quando o modelo não suportar documento binário, preservando nome/limite.
- **FR-MM-008**: O harness MUST redigir bytes e URLs de mídia em log, auditoria e telemetria, preservando apenas metadados não sensíveis (mime, nome, tamanho, presença).
- **FR-MM-009**: A janela de contexto MUST contabilizar mídia na estimativa de tokens de forma conservadora, registrando a decisão no evento do turno.
- **FR-MM-010**: O snapshot de sessão e o envelope de eventos MUST versionar as novas partes, com `go generate` idempotente e teste de conformidade.
- **FR-MM-011**: O harness MUST permitir o fallback apenas para perfis que declarem a capacidade de mídia exigida pelo turno.
- **FR-MM-012**: Nenhum teste da feature MUST tocar rede real (fakes/`httptest`), conforme constitution §7.

### Key Entities

- **Part (estendida)**: união tagueada `text|tool_call|tool_result|image|document`.
- **Media**: mídia de uma Part — `mime`, `name`, `size_bytes`, `source` (`bytes` inline ou `reference`/`url`).
- **Capabilities (estendida)**: ganha `vision`, `documents`, `max_media_bytes` (por perfil).

### Decisões desta spec (D-MM)

- **D-MM-1**: imagem e documento são **partes de mensagem do usuário**; resultado de tool continua texto/JSON no v1.
- **D-MM-2**: suporte é **por capacidade de modelo** (gate explícito), espelhando o FR-015 do núcleo; sem capacidade, erro nomeado.
- **D-MM-3**: fonte dupla permitida (bytes inline **ou** referência), decidida pelo host; o harness não baixa URLs no v1.
- **D-MM-4**: MIME por **allowlist**; planilha binária (xlsx) fica de fora — o host converte para CSV/texto.
- **D-MM-5**: redação de mídia é invariável (constitution §4): bytes/URL nunca saem em log/auditoria.

## 4. Success Criteria *(mandatory)*

- **SC-MM-001**: Enviar imagem suportada com `vision=true` chega ao provedor no formato nativo (verificado por teste de contrato do adaptador).
- **SC-MM-002**: Enviar mídia com perfil sem capacidade **sempre** falha explícita, sem chamada de rede (teste).
- **SC-MM-003**: **Zero** bytes/URL de mídia aparecem em log/auditoria (teste de redação com valor-isca).
- **SC-MM-004**: `make verify` verde, incluindo `go generate` sem diff no contrato atualizado.
- **SC-MM-005**: O caminho de texto existente permanece intacto (suíte do núcleo 100% verde).

## 5. Não-objetivos (v1)

- Áudio e vídeo.
- OCR/parse de conteúdo no harness (extração é do host/modelo).
- Download de mídia por URL dentro do harness.
- Mídia em resultado de tool (o host textualiza).
- Novos provedores nativos (Google/Bedrock/Azure): entram por adaptador próprio (feature futura).

## 6. Riscos

- **R-MM-1 — Vazamento de binário/PII**: mitigação FR-MM-008 + SC-MM-003.
- **R-MM-2 — Envio de mídia a modelo incapaz**: mitigação FR-MM-003/FR-MM-011.
- **R-MM-3 — Sessão/snapshot inflado**: mitigação FR-MM-004/FR-MM-009 (teto + janela).
- **R-MM-4 — Divergência de API dos provedores para arquivos**: mitigação testes de contrato por adaptador + allowlist conservadora.
- **R-MM-5 — Regressão do caminho de texto**: mitigação SC-MM-005 (suíte 001 continua verde).

## 7. Lacunas (a resolver no plan/research)

- **L-MM-1**: limite default de `max_media_bytes` e de mídias por turno.
- **L-MM-2**: bloco exato de documento no OpenAI-compatible (varia por implementação/versão) — decidir no `research.md` com fallback textual.
- **L-MM-3**: como o snapshot representa bytes vs. referência (schema) — decidir no `data-model.md`/contrato.
- **L-MM-4**: política de descarte de mídia antiga na janela.
