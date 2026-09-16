package harness_test

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmarquespaixao/ia-harness/adapters/session/memory"
	"rmarquespaixao/ia-harness/harness"
	"rmarquespaixao/ia-harness/internal/testutil"
)

type recordingTracer struct {
	mu    sync.Mutex
	names []string
}

func (t *recordingTracer) record(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.names = append(t.names, name)
}

func (t *recordingTracer) StartTurn(ctx context.Context, _ harness.TurnAttrs) (context.Context, harness.Span) {
	t.record("turn")
	return ctx, nopSpan{}
}
func (t *recordingTracer) StartModel(ctx context.Context, _ harness.ModelAttrs) (context.Context, harness.Span) {
	t.record("model")
	return ctx, nopSpan{}
}
func (t *recordingTracer) StartTool(ctx context.Context, _ harness.ToolAttrs) (context.Context, harness.Span) {
	t.record("tool")
	return ctx, nopSpan{}
}

type nopSpan struct{}

func (nopSpan) End(error) {}

func TestTracer_EmiteSpansDeTurnoModeloTool(t *testing.T) {
	// Arrange — um turno com uma tool executada.
	tracer := &recordingTracer{}
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{
		{ToolCalls: []harness.ToolCall{{ID: "c1", Name: "consultar_saldo", Args: []byte(`{"q":"x"}`)}}},
		{Text: "pronto"},
	}}
	cfg := harness.Config{
		Providers: map[string]harness.Provider{turnProviderKey: provider},
		Models: map[string]harness.ModelProfile{
			turnModelAlias: {Provider: turnProviderKey, Model: "fake-1", Capabilities: harness.Capabilities{ToolCalling: true, Streaming: true}},
		},
		Tools:        []harness.ToolSource{&testutil.MemToolSource{Tools: []harness.Tool{{Name: "consultar_saldo", InputSchema: turnToolSchema}}}},
		Credentials:  &testutil.FakeCredentialProvider{Values: map[string]string{}},
		Sessions:     memory.New(),
		Logger:       slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
		DefaultModel: turnModelAlias,
		Policy:       harness.PolicyConfig{Default: harness.PolicyAllow},
		Tracer:       tracer,
	}
	h, err := harness.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })

	// Act
	_, err = h.Run(context.Background(), pergunta("saldo?"), &turnHandler{})

	// Assert
	require.NoError(t, err)
	tracer.mu.Lock()
	defer tracer.mu.Unlock()
	assert.Contains(t, tracer.names, "turn")
	assert.Contains(t, tracer.names, "model")
	assert.Contains(t, tracer.names, "tool")
}
