# ADR 0009 — Partes multimodais (imagem/documento) com gate por capacidade do modelo

**Status**: Accepted
**Data**: 2026-09-16
**Feature**: `specs/nucleo/002-multimodal-imagem-documento`

## Contexto

O núcleo só entendia texto (`Part` com `text|tool_call|tool_result`; adaptadores serializando conteúdo como string). Os provedores-alvo já aceitam conteúdo multimodal (OpenAI-compatible `content` array com `image_url`/`file`; Anthropic blocos `image`/`document`), e o caso de uso do financeiro (anexar imagem/PDF de fatura) exige isso. Era necessário decidir **onde** a mídia vive no modelo, **como** é validada e **como** é mapeada por adaptador, sem violar a constitution §4 (redação) nem o FR-015 (capacidade explícita).

## Decisão

1. Mídia entra como **partes de mensagem do usuário**: `PartKind` ganha `image` e `document`, com `Part.Media *Media{mime,name,size_bytes,bytes,reference}`. Resultado de tool continua `text|json` no v1.
2. **Fonte dupla, exclusiva**: exatamente uma entre `bytes` (inline, base64) e `reference` (URL/ref do host). O harness não baixa URLs.
3. **Gate por capacidade do modelo**: `Capabilities` ganha `vision`, `documents`, `max_media_bytes`. Sem a capacidade exigida → erro nomeado **antes** de chamar o provedor (espelha o FR-015). `text/plain`/`text/csv` não exige `documents` (caminho textual sempre disponível).
4. **Allowlist de MIME** e **teto de tamanho** (default 5 MiB/mídia, 10 mídias/turno); fora disso → código estável. xlsx/outros binários ficam fora do v1 (host converte).
5. **Redação invariável**: `bytes` e `reference`/`url` nunca saem em log/auditoria; só `mime`/`name`/`size_bytes`. Contrato de sessão versionado (`contracts/session/*.json` → `go generate`).

## Alternativas consideradas

- **Só bytes inline**: simples, mas força trafegar binário grande no histórico e não cobre URL/ref (documento já no S3 do host). Rejeitada.
- **Só referência**: expõe o adaptador a variações (PDF por URL nem sempre aceito) e o host precisaria padronizar. Rejeitada.
- **Mídia em `ResultContent` (tool)**: fora do caso de uso (entrada do usuário); aumenta superfície. Rejeitada no v1.
- **Inferir suporte do modelo pelo provider (sem flag)**: arriscado — campo pode ser ignorado em silêncio. Rejeitada (mantém FR-015).

## Consequências

- Mudança **aditiva** da API pública (campos/enum novos) → MINOR; suíte 001 permanece válida.
- Adaptador OpenAI-compatible passa a emitir `content` como array quando há mídia (string quando só texto — compatível); Anthropic ganha blocos `image`/`document` e `wireBlock.Source`.
- Snapshot de sessão cresce (mídia persistida): teto e janela controlam o tamanho; mídia antiga pode ser descartada no truncamento, com registro.
- Limitações registradas: Ollama (campo `images`) e multimodal em resultado de tool ficam fora do v1.
