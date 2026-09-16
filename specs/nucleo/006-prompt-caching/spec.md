# Feature Specification: Prompt caching do provedor

**Feature Branch**: `nucleo/006-prompt-caching`

**Created**: 2026-09-16

**Status**: Aprovado e implementado (portão liberado pelo operador)

**Input**: "como tá o cache dos providers para acionar?" — o harness não enviava breakpoints de cache (Anthropic `cache_control`) nem lia os tokens cacheados no usage, então o custo ficava superestimado nos modelos com preço de "cached read/write" (Zen/Go, Anthropic).

## 1. Problema

Provedores cobram menos por tokens de prompt já cacheados (ex.: Anthropic `cache_read` ~0.1× input; OpenAI `cached_tokens`; Zen/Go têm coluna de preço de cache). O harness: (a) nunca pedia cache — o Anthropic só cacheia com `cache_control` explícito; (b) ignorava os tokens cacheados no `usage` (`cache_read_input_tokens`, `cache_creation_input_tokens`, `prompt_tokens_details.cached_tokens`, `input_tokens_details.cached_tokens`) — o `Usage` não tinha campo e o custo não aplicava desconto. Resultado: custo superestimado e nenhum controle para "acionar" cache.

## 2. Requisitos (EARS)

- **FR-PC-001**: WHEN `Capabilities.PromptCaching` do perfil é verdadeiro, o harness SHALL sinalizar cache no `ChatRequest.PromptCaching` do turno.
- **FR-PC-002**: O adaptador Anthropic MUST enviar breakpoints `cache_control: {type:"ephemeral"}` no `system` (como bloco) e no **último** tool quando o cache está ligado; sem cache, `system` permanece string e nenhum `cache_control` é enviado.
- **FR-PC-003**: Os três adapters MUST ler os tokens cacheados do `usage` e devolvê-los em `Usage.CachedInputTokens`/`CacheWriteTokens` (InputTokens é o total, incluindo os cacheados).
- **FR-PC-004**: O custo MUST aplicar o preço de cache (`Pricing.CachedInputPriceMicrosPerMillion`/`CacheWritePriceMicrosPerMillion`); preço de cache zerado cai no preço de input (nunca subestima).
- **FR-PC-005**: `UsageEvent` e `AuditEvent` MUST expor os tokens de cache; o snapshot de sessão/uso MUST persistí-los (contrato atualizado + `go generate`).
- **FR-PC-006**: Sem cache reportado/ligado, o comportamento é idêntico ao anterior (estimativa `estimated=true` sem campos de cache).

### Decisões (D-PC)

- **D-PC-1**: cache sob `Capabilities.PromptCaching` (por perfil), sinalizado em `ChatRequest.PromptCaching`; sem `Params` mágicos.
- **D-PC-2**: breakpoints no `system` + último tool (cobre o prefixo estável: instruções + catálogo), sem tocar nas mensagens no v1.
- **D-PC-3**: `InputTokens` = total; `CachedInputTokens` é subconjunto lido; `CacheWriteTokens` é a escrita.

## 3. Critérios de aceite (Gherkin)

```gherkin
Cenário: Anthropic com cache ligado
  Dado um perfil com Capabilities.PromptCaching
  Quando o turno chama a Messages API
  Então o system vai como bloco com cache_control ephemeral
  E o último tool tem cache_control

Cenário: tokens cacheados entram no uso
  Dado um usage com cache_read_input_tokens e cache_creation_input_tokens
  Quando o adaptador monta a resposta
  Então Usage.CachedInputTokens/CacheWriteTokens são preenchidos
  E InputTokens é o total

Cenário: custo com preço de cache
  Dado Pricing com preço de cache read menor que input
  Quando o custo é calculado
  Então os tokens lidos usam o preço de cache
  E preço de cache zerado cai no preço de input
```

## 4. Não-objetivos

- Cache de resposta de modelo (host).
- Cache de catálogo MCP (já existe).
- Breakpoints nas mensagens/últimos turnos (evolução).
- Janela/TTL do cache (definido pelo provedor).
