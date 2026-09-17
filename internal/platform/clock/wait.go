package clock

import (
	"context"
	"time"
)

// SystemWait é a porta Waiter padrão: suspende até o prazo respeitando o
// cancelamento do contexto (feature 019). É a única ocorrência de time.Sleep na
// árvore, ao lado de System.Now.
type SystemWait struct{}

// Wait aguarda d ou até o contexto ser cancelado, o que vier primeiro.
func (SystemWait) Wait(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
