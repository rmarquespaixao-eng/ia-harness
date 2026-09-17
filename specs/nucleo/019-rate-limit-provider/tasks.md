# Tasks: Rate limiting de provider (019)

**Input**: `specs/nucleo/019-rate-limit-provider/` (spec · plan · use-cases)
**Pré-requisitos**: plan aprovado. Cada task fecha com teste AAA e `make verify` verde.

## Fase 1 — Contratos aditivos

- [x] T1901 `internal/core/ports.go`: porta `Waiter` — FR-RL-003/006
- [x] T1902 `internal/core/config.go`: `RateLimit`, `Config.RateLimits`, `Config.DefaultRateLimit`, `Config.Waiter` — FR-RL-001/002
- [x] T1903 `internal/core/events.go`: `RateLimitEvent` + `RateLimitHandler` — FR-RL-007
- [x] T1904 `harness/alias.go`: aliases de `RateLimit`/`Waiter`/`RateLimitEvent`/`RateLimitHandler` — API pública

## Fase 2 — Regra pura e plataforma

- [x] T1905 `internal/platform/clock`: `SystemWait` (`time.Sleep` respeitando `ctx`) — FR-RL-003
- [x] T1906 `internal/engine/ratelimit/bucket.go`: `Bucket.Reserve` (token bucket determinístico) + `ratelimit_test.go` — FR-RL-001/005/006

## Fase 3 — Wiring

- [x] T1907 `internal/engine/provider_route.go`: bucket por alias lazy, espera, teto e erro retryável — FR-RL-003/004/005
- [x] T1908 `internal/engine/loop.go`/`provider_route.go`: emitir `RateLimitEvent` via type assertion — FR-RL-007
- [x] T1909 `internal/engine/config_validate.go`: default do `Waiter` — FR-RL-006

## Fase 4 — Testes e fechamento

- [x] T1910 `provider_route` teste: rajada espaça, teto falha, cancelamento não chama o provider — CU-RL-1/2/3
- [x] T1911 `make verify` verde + ADR 0024 + CHANGELOG + docs de feature — Constitution §7

## Dependências

- T1901→T1902→T1903→T1904; T1905/T1906→T1907→T1908/T1909; tudo→T1910–T1911.
- Depende de 007 (retry) e 003 (motor) — já entregues.

## Rastreabilidade — tasks × requisitos

| Requisito | Tasks |
|---|---|
| FR-RL-001 | T1902, T1906 |
| FR-RL-002 | T1902, T1907 |
| FR-RL-003 | T1901, T1905, T1907 |
| FR-RL-004 | T1907 |
| FR-RL-005 | T1906, T1907 |
| FR-RL-006 | T1901, T1905, T1906, T1909 |
| FR-RL-007 | T1903, T1904, T1908 |
| FR-RL-008 | T1905, T1907 |
| CU-RL-1/2/3 | T1910 |
