# Plan: Rate limiting de provider (019)

**Feature**: `specs/nucleo/019-rate-limit-provider/` · **Status**: Aprovado

Constitution: **não viola** nenhuma seção. Aditivo na API pública (campos/portas novos), sem quebra de contrato de fio/persistência (nenhum JSON Schema muda).

## Arquitetura

1. **Porta de espera** (`internal/core/ports.go`): `Waiter interface { Wait(ctx context.Context, d time.Duration) error }`. Default `clock.SystemWait{}` em `internal/platform/clock` (único lugar com `time.Sleep`). Mantém o princípio "tempo entra por porta".
2. **Config** (`internal/core/config.go`): `RateLimit{RequestsPerMinute int; Burst int; MaxWait time.Duration}` e `Config.RateLimits map[string]RateLimit` (chave = alias do provider) + `Config.DefaultRateLimit RateLimit`. `Waiter` opcional.
3. **Regra pura** (`internal/engine/ratelimit`): `Bucket` com `Reserve(now time.Time) (wait time.Duration)`, sem estado global nem goroutines. Refill contínuo por delta de tempo; `Burst` como capacidade; `last` atualizado monotonicamente (relógio para trás ⇒ delta 0).
4. **Wiring** (`internal/engine/provider_route.go`): `chat` mantém um `*ratelimit.Bucket` por alias lazy (mutex no `Harness`); em `attempt`, antes do I/O: `wait := bucket.Reserve(now)`; se `wait > MaxWait` ⇒ `*ProviderError{Code:"ratelimit/espera-excedida", Retryable:true}`; senão `Waiter.Wait(ctx, wait)`; emite `RateLimitEvent` se o handler implementar.
5. **Observabilidade** (`internal/core/events.go`): `RateLimitEvent{Provider, WaitMS}` + `RateLimitHandler` opcional (type assertion, não quebra `Handler`).

## Contratos (aditivos)

- `RateLimit`, `Config.RateLimits`, `Config.DefaultRateLimit`, `Config.Waiter`.
- `Waiter`, `RateLimitEvent`, `RateLimitHandler`; aliases em `harness/alias.go`.

## Alternativas consideradas

- **Reusar `Clock` com `Sleep`**: quebraria implementadores host de `Clock` (semver MAJOR). Porta separada opcional. Rejeitada a quebra.
- **Só delegar 429 ao retry**: não evita a rajada nem o desperdício. Rejeitada.
- **Limiter global único**: o limite é por conta/provider (chave distinta); um global estrangula providers saudáveis. Rejeitada.
- **`golang.org/x/time/rate`**: dependência nova para regra que cabe em ~40 linhas e precisa do `Clock` injetado. Rejeitada (constitution §2: dependências mínimas).

## Riscos

- Espera dentro do turno consome `Budget.MaxWall`; documentado e limitado por `MaxWait`.
- Testes não podem dormir de verdade: `Waiter` fake + `Clock` fake dão determinismo total.
