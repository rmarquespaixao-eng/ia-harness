package harness_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/adapters/session/memory"
	"github.com/rmarquespaixao-eng/ia-harness/harness"
	"github.com/rmarquespaixao-eng/ia-harness/internal/testutil"
)

// cuHar4Summarizer devolve um resumo fixo e conta as chamadas.
type cuHar4Summarizer struct {
	summary string
	calls   int
}

func (s *cuHar4Summarizer) Summarize(context.Context, []harness.Message, int) (string, error) {
	s.calls++
	return s.summary, nil
}

// compactionRecorder embute NopHandler e registra os eventos de compactação.
type compactionRecorder struct {
	harness.NopHandler
	events []harness.CompactionEvent
}

func (r *compactionRecorder) Compaction(_ context.Context, ev harness.CompactionEvent) {
	r.events = append(r.events, ev)
}

// TestCompactacao_PorModeloResumeEExpoeInfo cobre CU-CTX-1/3: com a janela vinda
// da capacidade do modelo, o turno longo compacta o histórico antigo num resumo
// e expõe o efeito em TurnResult.Compaction e no evento opcional.
func TestCompactacao_PorModeloResumeEExpoeInfo(t *testing.T) {
	// Arrange — janela de 300 (reserva 50) e histórico grande no 2º turno.
	ctx := context.Background()
	summarizer := &cuHar4Summarizer{summary: "o começo da conversa"}
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{
		{Text: strings.Repeat("r", 2000)},
		{Text: "ok"},
	}}
	handler := &compactionRecorder{}
	h := cuHar4NewHarness(t, memory.New(), provider, func(cfg *harness.Config) {
		cfg.Models = map[string]harness.ModelProfile{
			"default": {Provider: "stub", Model: "stub-1", Capabilities: harness.Capabilities{
				MaxContextTokens: 300, MaxOutputTokens: 50,
			}},
		}
		cfg.Context = harness.ContextPolicy{Strategy: harness.StrategySummarize, Summarizer: summarizer}
	})

	primeiro, err := h.Run(ctx, harness.RunRequest{
		UserID: "user-1",
		Input:  cuHar4Input(strings.Repeat("q", 400)),
	}, handler)
	require.NoError(t, err)

	// Act
	segundo, err := h.Run(ctx, harness.RunRequest{
		SessionID: primeiro.SessionID,
		UserID:    "user-1",
		Input:     cuHar4Input("e agora?"),
	}, handler)

	// Assert — o 2º request vai compactado, com resumo e info/evento.
	require.NoError(t, err)
	require.Len(t, provider.Requests, 2)
	require.NotNil(t, segundo.Compaction, "compactação exposta em TurnResult")
	assert.True(t, segundo.Compaction.Summarized)
	assert.Greater(t, segundo.Compaction.TokensBefore, segundo.Compaction.TokensAfter)
	assert.Equal(t, 1, summarizer.calls, "resumo no máximo 1×/turno")

	require.Len(t, handler.events, 1)
	assert.True(t, handler.events[0].Summarized)
	assert.Equal(t, segundo.Compaction.TokensBefore, handler.events[0].TokensBefore)

	serializado := serializarMensagens(provider.Requests[1].Messages)
	assert.Contains(t, serializado, "resumo de histórico: o começo da conversa")
}

// serializarMensagens concatena os textos das mensagens para asserção.
func serializarMensagens(messages []harness.Message) string {
	var builder strings.Builder
	for _, message := range messages {
		for _, part := range message.Parts {
			builder.WriteString(part.Text)
			builder.WriteByte('\n')
		}
	}
	return builder.String()
}
