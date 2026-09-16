// Package retry executa operações com novas tentativas e espera exponencial
// configurável, reutilizado pelos adaptadores do harness.
package retry

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"
)

const (
	// defaultMaxAttempts é o total de tentativas quando a política não define
	// um valor positivo.
	defaultMaxAttempts = 1
	// defaultBaseDelay é a espera base quando a política não define uma.
	defaultBaseDelay = 100 * time.Millisecond
	// jitterFactor é a variação máxima aplicada com Jitter ligado (±20%).
	jitterFactor = 0.2
)

// Policy configura o número de tentativas e o backoff entre elas.
type Policy struct {
	// MaxAttempts é o total de execuções permitidas; <= 0 vale 1.
	MaxAttempts int
	// BaseDelay é a espera após a primeira falha; <= 0 vale 100ms.
	BaseDelay time.Duration
	// MaxDelay limita a espera; menor que BaseDelay vale BaseDelay.
	MaxDelay time.Duration
	// Jitter aplica variação aleatória de ±20% sobre a espera calculada.
	Jitter bool
}

// Sleeper espera a duração informada, respeitando o cancelamento do contexto.
type Sleeper func(ctx context.Context, d time.Duration) error

// Do executa op até obter sucesso. Entre tentativas chama sleep com o delay da
// próxima tentativa; falha na espera interrompe o laço imediatamente. O erro da
// última tentativa é devolvido embrulhado para preservar errors.Is.
func Do(ctx context.Context, p Policy, sleep Sleeper, op func(ctx context.Context) error) error {
	return DoIf(ctx, p, sleep, op, nil)
}

// DoIf é como Do, mas interrompe o laço quando retryable(err) é falso, para não
// repetir erros permanentes (ex.: 4xx que não 429). retryable nil repete sempre.
func DoIf(ctx context.Context, p Policy, sleep Sleeper, op func(ctx context.Context) error, retryable func(error) bool) error {
	p = p.normalized()

	var opErr error
	for attempt := 1; attempt <= p.MaxAttempts; attempt++ {
		opErr = op(ctx)
		if opErr == nil {
			return nil
		}
		if attempt == p.MaxAttempts {
			break
		}
		if retryable != nil && !retryable(opErr) {
			break
		}
		if waitErr := sleep(ctx, p.delayFor(attempt+1)); waitErr != nil {
			return fmt.Errorf("retry: espera cancelada: %w", waitErr)
		}
	}

	return fmt.Errorf("retry: %d tentativas: %w", p.MaxAttempts, opErr)
}

// SleepCtx espera a duração informada respeitando o cancelamento do contexto.
func SleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// normalized devolve a política com os valores padrão aplicados.
func (p Policy) normalized() Policy {
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = defaultMaxAttempts
	}
	if p.BaseDelay <= 0 {
		p.BaseDelay = defaultBaseDelay
	}
	if p.MaxDelay < p.BaseDelay {
		p.MaxDelay = p.BaseDelay
	}
	return p
}

// delayFor calcula a espera antes da tentativa n (n >= 2):
// BaseDelay * 2^(n-2), limitada a MaxDelay e, com Jitter, variada em ±20%.
func (p Policy) delayFor(n int) time.Duration {
	delay := p.BaseDelay
	for i := 0; i < n-2 && delay < p.MaxDelay; i++ {
		if delay > p.MaxDelay/2 {
			delay = p.MaxDelay
			break
		}
		delay *= 2
	}
	if delay > p.MaxDelay {
		delay = p.MaxDelay
	}
	if p.Jitter {
		delay = time.Duration(float64(delay) * (1 + (rand.Float64()*2-1)*jitterFactor))
	}
	return delay
}
