# ADR 0014 — Retry classificado das chamadas de provider

**Status**: Accepted
**Data**: 2026-09-16
**Feature**: `specs/nucleo/007-retry-provider`

## Contexto

Os adaptadores faziam uma única tentativa; erros transitórios (429, timeout, 5xx, transporte) encerravam o turno. O `internal/platform/retry` já existia (usado pelo `mcpclient`), mas não classificava erros nem era aplicado aos providers. A comunidade trata retry como básico.

## Decisão

Extrair `internal/platform/providerhttp` com `Retryable(err)` (códigos transitórios de `*harness.ProviderError`, **nunca** cancelamento) e `Do(ctx, policy, attempt)` (envolve `retry.DoIf` + `retry.SleepCtx`). Os três adaptadores passam a montar a requisição num callback `attempt` e delegar ao `Do`; o **stream é consumido depois do retry**, então falha no meio do stream não repete. `Config` ganha `MaxAttempts` (default 3), `RetryBaseDelay`/`RetryMaxDelay` (200ms/2s), e o `retry.Policy` já aplica jitter ±20% e saturação.

## Alternativas consideradas

- **Retry dentro de cada adapter**: duplicaria classificação e laço. Rejeitada.
- **Retry no engine (em volta do `Provider.Chat`)**: repetiria o turno inteiro, inclusive tools já executadas — perigoso. Rejeitada.
- **Repetir também stream parcial**: duplicaria texto/cobrança/eventos. Rejeitada.
- **Default sem retry (1 tentativa)**: não entrega o "básico" out-of-the-box. Rejeitada (default 3).

## Consequências

- Resiliência a 429/5xx/timeout sem intervenção do host; permanece configurável.
- Cancelamento e erros permanentes falham na hora (sem espera).
- Testes: `openai/retry_test.go` (recupera em 429; 400 não repete; `MaxAttempts=1`).
- Custo: até `MaxAttempts` requisições em falha transitória (cobrado só o sucesso).
