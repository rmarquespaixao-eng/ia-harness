package harness_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/harness"
	"github.com/rmarquespaixao-eng/ia-harness/internal/testutil"
)

// cu2SessionStore é o SessionStore em memória mínimo dos cenários CU-HAR-2.
type cu2SessionStore struct {
	sessions map[string]*harness.Session
}

// Load devolve a sessão salva ou erro nomeado quando ausente.
func (s *cu2SessionStore) Load(_ context.Context, id string) (*harness.Session, error) {
	session, ok := s.sessions[id]
	if !ok {
		return nil, fmt.Errorf("cu2: sessão %q não encontrada", id)
	}
	return session, nil
}

// Save guarda o snapshot da sessão.
func (s *cu2SessionStore) Save(_ context.Context, session *harness.Session) error {
	if s.sessions == nil {
		s.sessions = map[string]*harness.Session{}
	}
	s.sessions[session.ID] = session
	return nil
}

// cu2BaseConfig monta a configuração do harness para os cenários do CU-HAR-2.
func cu2BaseConfig(providers map[string]harness.Provider, models map[string]harness.ModelProfile, defaultModel string, tools []harness.ToolSource) harness.Config {
	return harness.Config{
		Providers:    providers,
		Models:       models,
		Tools:        tools,
		DefaultModel: defaultModel,
		Policy:       harness.PolicyConfig{Default: harness.PolicyAllow},
		Credentials:  &testutil.FakeCredentialProvider{Values: map[string]string{}},
		Sessions:     &cu2SessionStore{},
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// cu2ToolResultTexts extrai os textos dos resultados de tool presentes nas mensagens.
func cu2ToolResultTexts(messages []harness.Message) []string {
	var out []string
	for _, message := range messages {
		for _, part := range message.Parts {
			if part.Kind != harness.PartToolResult || part.Result == nil {
				continue
			}
			for _, content := range part.Result.Content {
				out = append(out, content.Text)
			}
		}
	}
	return out
}

// CU-HAR-2, cenário "Troca de modelo só por configuração": o mesmo adaptador e
// o mesmo script servem dois perfis; só o alias default da Config muda e a
// próxima sessão passa a usar o novo modelo.
func TestCU_HAR2TrocaDeModeloPorConfiguracao(t *testing.T) {
	// Arrange
	script := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "pronto"}, {Text: "pronto"}}}
	providers := map[string]harness.Provider{"stub": script}
	models := map[string]harness.ModelProfile{
		"alpha": {Provider: "stub", Model: "modelo-a"},
		"beta":  {Provider: "stub", Model: "modelo-b"},
	}
	hAlpha, err := harness.New(cu2BaseConfig(providers, models, "alpha", nil))
	require.NoError(t, err)
	hBeta, err := harness.New(cu2BaseConfig(providers, models, "beta", nil))
	require.NoError(t, err)
	req := harness.RunRequest{UserID: "u-1", Input: []harness.Part{{Kind: harness.PartText, Text: "olá"}}}

	// Act
	resultAlpha, err := hAlpha.Run(context.Background(), req, nil)
	require.NoError(t, err)
	resultBeta, err := hBeta.Run(context.Background(), req, nil)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, "alpha", resultAlpha.Model)
	assert.Equal(t, "beta", resultBeta.Model)
	require.Len(t, script.Requests, 2)
	assert.Equal(t, "modelo-a", script.Requests[0].Model, "a troca de modelo vem só da configuração")
	assert.Equal(t, "modelo-b", script.Requests[1].Model)
}

// CU-HAR-2, cenário "Fallback sem repetir tool": o primário pede uma tool e
// falha na chamada seguinte; o fallback conclui reaproveitando o resultado já
// registrado, sem segunda execução da tool (R12/FR-014).
func TestCU_HAR2FallbackSemRepetirTool(t *testing.T) {
	// Arrange
	primary := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{
		{ToolCalls: []harness.ToolCall{{ID: "call-1", Name: "consultar_saldo"}}},
		{Err: errors.New("429 do provedor primário")},
	}}
	backup := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "saldo: 10 (fallback)"}}}
	tools := &testutil.MemToolSource{
		Tools: []harness.Tool{{Name: "consultar_saldo", InputSchema: json.RawMessage(`{"type":"object"}`)}},
		Results: map[string]harness.ToolResult{
			"consultar_saldo": {Content: []harness.ResultContent{{Kind: harness.ResultText, Text: "saldo: 10"}}},
		},
	}
	cfg := cu2BaseConfig(
		map[string]harness.Provider{"p-primary": primary, "p-backup": backup},
		map[string]harness.ModelProfile{
			"primary": {Provider: "p-primary", Model: "modelo-p", Capabilities: harness.Capabilities{ToolCalling: true, Streaming: true}, Fallbacks: []string{"backup"}},
			"backup":  {Provider: "p-backup", Model: "modelo-b", Capabilities: harness.Capabilities{ToolCalling: true, Streaming: true}},
		},
		"primary",
		[]harness.ToolSource{tools},
	)
	h, err := harness.New(cfg)
	require.NoError(t, err)

	// Act
	result, err := h.Run(context.Background(), harness.RunRequest{
		UserID: "u-1",
		Input:  []harness.Part{{Kind: harness.PartText, Text: "qual o saldo?"}},
	}, nil)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "backup", result.Model, "modelo efetivo é o alias do fallback")
	assert.Equal(t, harness.StopCompleted, result.StopReason)
	assert.Equal(t, 2, primary.Calls, "primário: 1 chamada com tool + 1 que falhou")
	assert.Equal(t, 1, backup.Calls)
	require.Len(t, tools.Calls, 1, "tool não é reexecutada no fallback (R12)")
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "consultar_saldo", result.ToolCalls[0].Tool)
	require.Len(t, backup.Requests, 1)
	assert.Contains(t, cu2ToolResultTexts(backup.Requests[0].Messages), "saldo: 10",
		"fallback reaproveita o resultado da tool já executada")
}
