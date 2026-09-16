package harness_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/adapters/session/memory"
	"github.com/rmarquespaixao-eng/ia-harness/harness"
	"github.com/rmarquespaixao-eng/ia-harness/internal/testutil"
)

// streamProvider implementa StreamingProvider emitindo reasoning e fragmentos
// de argumentos além do texto (feature 012).
type streamProvider struct{}

func (streamProvider) Chat(ctx context.Context, req harness.ChatRequest, onText func(string)) (harness.ChatResponse, error) {
	return streamProvider{}.ChatStream(ctx, req, harness.TextSink(onText))
}

func (streamProvider) ChatStream(_ context.Context, _ harness.ChatRequest, sink harness.StreamSink) (harness.ChatResponse, error) {
	sink.Reasoning("pensando...")
	sink.ToolCallArgs("c1", "consultar_saldo", `{"q":`)
	sink.ToolCallArgs("c1", "consultar_saldo", `"saldo"}`)
	sink.Text("pronto")
	return harness.ChatResponse{
		Message: harness.Message{Role: harness.RoleAssistant, Parts: []harness.Part{{Kind: harness.PartText, Text: "pronto"}}},
	}, nil
}

func (streamProvider) Close() error { return nil }

type streamHandler struct {
	harness.NopHandler
	reasoning []string
	args      []string
	texts     []string
}

func (h *streamHandler) ReasoningDelta(_ context.Context, ev harness.ReasoningDelta) {
	h.reasoning = append(h.reasoning, ev.Text)
}
func (h *streamHandler) ToolCallDelta(_ context.Context, ev harness.ToolCallArgsDelta) {
	h.args = append(h.args, ev.ArgsFragment)
}
func (h *streamHandler) TextDelta(_ context.Context, ev harness.TextDelta) {
	h.texts = append(h.texts, ev.Text)
}

func TestStreaming_ReasoningEArgsChegamAoHandler(t *testing.T) {
	// Arrange
	cfg := harness.Config{
		Providers: map[string]harness.Provider{turnProviderKey: streamProvider{}},
		Models: map[string]harness.ModelProfile{
			turnModelAlias: {Provider: turnProviderKey, Model: "fake-1", Capabilities: harness.Capabilities{ToolCalling: true, Streaming: true}},
		},
		Credentials:  &testutil.FakeCredentialProvider{Values: map[string]string{}},
		Sessions:     memory.New(),
		Logger:       slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
		DefaultModel: turnModelAlias,
		Policy:       harness.PolicyConfig{Default: harness.PolicyAllow},
	}
	h, err := harness.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	handler := &streamHandler{}

	// Act
	_, err = h.Run(context.Background(), pergunta("saldo?"), handler)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"pensando..."}, handler.reasoning)
	assert.Equal(t, []string{`{"q":`, `"saldo"}`}, handler.args)
	assert.Equal(t, []string{"pronto"}, handler.texts)
}
