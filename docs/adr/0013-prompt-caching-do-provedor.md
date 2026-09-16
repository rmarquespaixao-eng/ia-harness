# ADR 0013 — Prompt caching do provedor (breakpoints + tokens cacheados no custo)

**Status**: Accepted
**Data**: 2026-09-16
**Feature**: `specs/nucleo/006-prompt-caching`

## Contexto

Modelos com prompt caching cobram menos por tokens já cacheados (Anthropic `cache_read` ~0.1× input e `cache_creation` ~1.25×; OpenAI/Responses reportam `cached_tokens`; Zen/Go publicam preço de "Cached Read/Write"). O harness não pedia cache (o Anthropic só cacheia com `cache_control` explícito) nem lia os tokens cacheados no `usage`, então o `Usage` não os representava e o custo era superestimado.

## Decisão

1. **Sinal por capacidade**: `Capabilities.PromptCaching` (perfil) → `ChatRequest.PromptCaching` no turno (aditivo).
2. **Anthropic**: com cache ligado, `system` vira bloco `[{type:text, text, cache_control:{type:ephemeral}}]` e o **último** tool recebe `cache_control`; sem cache, `system` continua string e nenhum breakpoint é enviado.
3. **Leitura de tokens cacheados** nos três adapters: `cache_read_input_tokens`/`cache_creation_input_tokens` (Anthropic), `prompt_tokens_details.cached_tokens` (Chat), `input_tokens_details.cached_tokens` (Responses). `InputTokens` passa a ser o **total** (não cacheado + leitura + escrita); `CachedInputTokens` é o subconjunto lido; `CacheWriteTokens` é a escrita.
4. **Custo com preço de cache**: `Pricing.CachedInputPriceMicrosPerMillion`/`CacheWritePriceMicrosPerMillion`; zero cai no preço de input (conservador, nunca subestima). `costMicrosWithCache` calcula `(input−cached)·input + cached·cacheRead + write·cacheWrite + output·output`.
5. **Contratos**: `Usage` (sessão/snapshot), `UsageEvent` e `AuditEvent` ganham os campos; schemas atualizados e `go generate` regenerado.

## Alternativas consideradas

- **Sinal por `Params`**: mistura configuração de domínio com corpo do provedor e não alcança `cache_control` dentro de content blocks. Rejeitada.
- **Cachear também as mensagens**: mais breakpoints (limite 4 na Anthropic) e ganho incerto; adiado.
- **Não ler tokens cacheados**: mantém custo superestimado. Rejeitada.
- **Cache de resposta no núcleo**: fora do escopo (host).

## Consequências

- Custo reportado/estimado fica fiel ao desconto de cache quando o provedor reporta os tokens.
- Sem `PromptCaching`, nada muda (nenhum breakpoint, `estimated` como antes).
- Snapshot/uso e os eventos carregam os campos novos (aditivo); schema e gerado em sincronia.
- Testes: `telemetry/cache_cost_test.go`, `cache_test.go` nos três adapters e `harness/prompt_caching_test.go`.
