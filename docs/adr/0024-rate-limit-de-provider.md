# ADR 0024 — Rate limiting de provider

**Status**: Accepted
**Data**: 2026-09-17
**Feature**: `specs/nucleo/019-rate-limit-provider`

## Contexto

O provedor cobra/limita por conta (alias): uma rajada de turnos gera 429 e o retry da feature 007 só **reage** ao erro, depois de já ter gasto a chamada e o orçamento de tempo. Falta espaçar as chamadas **antes** do I/O, com limite por alias — contas distintas têm tetos distintos, e um limite global estrangula provedores saudáveis.

## Decisão

Token bucket **in-process por alias de provider**, aplicado antes do I/O. `Config.RateLimits` (map alias → `RateLimit{RequestsPerMinute, Burst, MaxWait}`) + `Config.DefaultRateLimit`; zero/ausente desliga. Porta nova `Waiter interface{ Wait(ctx, d) error }` (default `clock.SystemWait`) em vez de estender `Clock`, para não quebrar implementadores do host; a espera é **cancelável** pelo contexto. `MaxWait` excedido ⇒ `*ProviderError{Code:"ratelimit/espera-excedida"}` com `Retryable:true`, deixando a decisão de reagendar para cima. A regra pura fica em `internal/engine/ratelimit/bucket.go` (`Reserve(now) time.Duration`, refill contínuo, `last` monotônico) e o wiring em `internal/engine/provider_route.go` (`acquireRateLimit`), que mantém um bucket lazy por alias e emite `RateLimitEvent` quando o handler implementa a interface opcional.

## Alternativas consideradas

- **Reusar `Clock` com `Sleep`**: quebraria todo implementador host de `Clock` (MAJOR). Porta `Waiter` separada e opcional. Rejeitada.
- **Só delegar o 429 ao retry da feature 007**: não evita a rajada nem o desperdício da chamada. Rejeitada.
- **Limiter global único**: o limite é por conta/alias; um global pune quem está saudável. Rejeitada.
- **`golang.org/x/time/rate`**: dependência nova para uma regra de ~40 linhas que precisa do `Clock` injetado. Rejeitada (constitution §2: dependências mínimas).

## Consequências

- Espaçamento determinístico e testável: `Clock` + `Waiter` fakes dão suites sem `time.Sleep` real; testes com `-race`.
- O bucket é **in-process**: em múltiplas instâncias o limite efetivo é por processo — coordenação entre hosts é responsabilidade do host, não do núcleo.
- A espera **consome o `Budget.MaxWall`** do turno; fica limitada por `MaxWait` e documentada.
- Release MINOR (campos/portas aditivos); `Harness` expõe os aliases.
