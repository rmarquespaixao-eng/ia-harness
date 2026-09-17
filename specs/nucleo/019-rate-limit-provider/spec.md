# Feature Specification: Rate limiting de provider

**Feature Branch**: `nucleo/019-rate-limit-provider` · **Status**: Draft (aguardando portão)

**Input**: pedido do operador — Fase D (P2) do harness: "rate limiting de provider" (escolhido junto de cache semântico, execução durável e multi-agente).

## Problema

O harness chama o provedor sem qualquer controle de vazão. Um turno com várias iterações de tool, vários turnos concorrentes do host ou uma cadeia de fallback podem disparar rajadas que o provedor responde com `429`/`Retry-After`. Hoje o retry (feature 007) repete a chamada sem espaçamento, gastando tentativas e podendo agravar o limite; não há noção de "quantas requisições por minuto este provider aceita" no núcleo.

## Requisitos (EARS)

- **FR-RL-001**: The harness SHALL limit provider calls per provider alias using a token-bucket (requests per minute + burst), configurable in `Config`.
- **FR-RL-002**: WHEN a provider alias has no specific limit, the harness SHALL apply the global default limit (if configured); zero/absent limit ⇒ sem throttling (comportamento anterior).
- **FR-RL-003**: WHEN the bucket has no token, the harness MUST wait for the next refill using the injected `Waiter` (respeitando cancelamento do `context`), antes de chamar o provedor.
- **FR-RL-004**: WHEN the required wait exceeds the configured maximum, the harness MUST fail the attempt with a retryable provider error (`ratelimit/espera-excedida`) instead of blocking indefinidamente.
- **FR-RL-005**: The limiter MUST be compartilhado entre turnos e entre tentativas/fallbacks do mesmo provider (o limite é do provider, não do turno).
- **FR-RL-006**: The refill MUST be driven only by the injected `Clock`; `time.Now()` permanece proibido fora de `internal/platform/clock`.
- **FR-RL-007**: WHEN a call is throttled, the harness MUST emit an optional `RateLimitEvent` (provider, wait ms) quando o `Handler` implementar `RateLimitHandler`.
- **FR-RL-008**: Cancelamento durante a espera MUST devolver `context.Canceled/DeadlineExceeded` sem chamar o provedor.

## Critérios (Gherkin)

```gherkin
Cenário: rajada acima do limite é espaçada
  Dado um provider com RequestsPerMinute=60 e Burst=1
  Quando dois turnos chamam o provider no mesmo instante
  Então a segunda chamada espera ~1s antes de ser enviada

Cenário: espera maior que o teto falha rápido
  Dado um provider com RequestsPerMinute=1 e MaxWait=100ms
  Quando a segunda chamada exigiria 1s de espera
  Então o harness devolve erro retryável ratelimit/espera-excedida

Cenário: cancelamento durante a espera
  Dado um provider saturado com MaxWait alto
  Quando o contexto é cancelado durante a espera
  Então a chamada devolve context.Canceled e o provedor não é chamado
```

## Casos de borda

- `Burst` ≤ 0 cai para 1; `RequestsPerMinute` ≤ 0 desliga o limite.
- Relógio para trás (Now < last): o limiter nunca gera espera negativa.
- Fallback para outro provider: cada alias tem seu próprio bucket.
- Limite global + específico: o específico vence.

## Não-objetivos (v1)

- Limite por token (tokens/min) — apenas requisições/min.
- Coordenação entre processos (o bucket é in-process; multi-instância é do host).
- Retry-After do provedor (a feature 007 já classifica `429` como transitório; integrar o header é evolução futura).

## Dependências e espelho

- Depende de 007 (`retry.DoIf`/classificação de transitório) e 003 (fachada/motor).
- Host: `financeiro-api-v2` pode configurar limites por provider no `di.go` do assistente.
