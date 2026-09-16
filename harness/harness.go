package harness

import (
	"context"

	core "github.com/rmarquespaixao-eng/ia-harness/internal/core"
	"github.com/rmarquespaixao-eng/ia-harness/internal/engine"
	"github.com/rmarquespaixao-eng/ia-harness/internal/engine/telemetry"
)

// Harness is the stable public facade over the internal engine.
type Harness struct{ engine *engine.Harness }

func New(cfg Config) (*Harness, error) {
	coreHarness, err := engine.New(cfg)
	if err != nil {
		return nil, err
	}
	return &Harness{engine: coreHarness}, nil
}

func (h *Harness) Run(ctx context.Context, req RunRequest, handler Handler) (TurnResult, error) {
	return h.engine.Run(ctx, req, handler)
}

func (h *Harness) ResolveConfirmation(ctx context.Context, sessionID, callID string, decision Decision, handler Handler) (TurnResult, error) {
	return h.engine.ResolveConfirmation(ctx, sessionID, callID, decision, handler)
}

func (h *Harness) Session(ctx context.Context, sessionID string) (*Session, error) {
	return h.engine.Session(ctx, sessionID)
}

func (h *Harness) Close() error { return h.engine.Close() }

func SnapshotSession(session *Session) (SessionSnapshot, error) {
	return engine.SnapshotSession(session)
}

func RestoreSession(snapshot SessionSnapshot) (*Session, error) {
	return engine.RestoreSession(snapshot)
}

func WithTraceID(ctx context.Context, id string) context.Context {
	return core.WithTraceID(ctx, id)
}

// NewRedactor monta o redator canônico a partir da configuração de redação.
func NewRedactor(cfg RedactionConfig) *Redactor {
	return telemetry.NewRedactor(cfg)
}

// TextSink adapta um callback de texto a um StreamSink (compatibilidade com
// adapters que só entregam texto).
func TextSink(onText func(string)) StreamSink { return core.TextSink(onText) }
