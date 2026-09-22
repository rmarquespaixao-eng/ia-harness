package mcpclient

import (
	"context"
	"errors"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rmarquespaixao-eng/ia-harness/internal/platform/retry"
)

// reconnectPolicy é a política do T030: a operação original mais uma tentativa
// após reconectar, com base de 50ms e teto de 200ms.
var reconnectPolicy = retry.Policy{
	MaxAttempts: 2,
	BaseDelay:   50 * time.Millisecond,
	MaxDelay:    200 * time.Millisecond,
}

// withReconnect executa op com a sessão ativa; se a falha for de sessão/
// transporte, invalida a sessão, reconecta uma única vez e repete op. Erro de
// protocolo (ex.: tool desconhecida) e contexto cancelado não reconectam
// (FR-011).
func (c *Client) withReconnect(ctx context.Context, op func(context.Context, *mcp.ClientSession) error) error {
	var permanent error
	err := retry.Do(ctx, reconnectPolicy, sleep, func(ctx context.Context) error {
		if permanent != nil {
			return permanent
		}
		cs, err := c.ensureSession(ctx)
		if err != nil {
			if !shouldReconnect(err) {
				permanent = err
			}
			return err
		}
		if err := op(ctx, cs); err != nil {
			if !shouldReconnect(err) {
				permanent = err
				return err
			}
			c.invalidateSession()
			return err
		}
		return nil
	})
	if permanent != nil {
		return permanent
	}
	return err
}

// invalidateSession fecha e descarta a sessão e o cache do catálogo.
func (c *Client) invalidateSession() {
	c.mu.Lock()
	cs := c.session
	c.session = nil
	c.catalog = nil
	c.mu.Unlock()

	if cs != nil {
		_ = cs.Close()
	}
}

// shouldReconnect classifica a falha: erro de protocolo JSON-RPC,
// cancelamento de contexto e falhas de início stdio são definitivos;
// o restante é sessão/transporte.
func shouldReconnect(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if isPermanentStdioError(err) {
		return false
	}
	if errors.Is(err, mcp.ErrConnectionClosed) {
		return true
	}
	var rpcErr *jsonrpc.Error
	return !errors.As(err, &rpcErr)
}

// sleep aguarda o backoff respeitando o cancelamento do contexto.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
