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

// newPromptEnv monta um harness com o system prompt global e a política de
// agente informados (feature 005).
func newPromptEnv(t *testing.T, globalPrompt string, agents map[string]harness.AgentPolicy) (*harness.Harness, *testutil.ScriptedProvider) {
	t.Helper()
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "ok"}}}
	policy := harness.PolicyConfig{Default: harness.PolicyAllow, Agents: agents}
	cfg := harness.Config{
		Providers: map[string]harness.Provider{turnProviderKey: provider},
		Models: map[string]harness.ModelProfile{
			turnModelAlias: {Provider: turnProviderKey, Model: "fake-1", Capabilities: harness.Capabilities{ToolCalling: true, Streaming: true}},
		},
		Credentials:  &testutil.FakeCredentialProvider{Values: map[string]string{}},
		Sessions:     memory.New(),
		Logger:       slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
		DefaultModel: turnModelAlias,
		Policy:       policy,
		SystemPrompt: globalPrompt,
	}
	h, err := harness.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	return h, provider
}

func TestSystemPrompt_GlobalChegaAoProvider(t *testing.T) {
	// Dado um system prompt global configurado.
	h, prov := newPromptEnv(t, "Você é o assistente financeiro.", nil)

	// Quando o turno roda.
	_, err := h.Run(context.Background(), pergunta("saldo?"), nil)

	// Então o provider recebe o prompt em ChatRequest.System.
	require.NoError(t, err)
	require.Len(t, prov.Requests, 1)
	assert.Equal(t, "Você é o assistente financeiro.", prov.Requests[0].System)
}

func TestSystemPrompt_OverridePorAgente(t *testing.T) {
	// Dado um prompt global e um override do agente.
	agents := map[string]harness.AgentPolicy{
		turnAgentID: {SystemPrompt: "Você é o agente de conciliação."},
	}
	h, prov := newPromptEnv(t, "prompt global", agents)

	// Quando o turno do agente roda.
	_, err := h.Run(context.Background(), pergunta("saldo?"), nil)

	// Então o override vence.
	require.NoError(t, err)
	require.Len(t, prov.Requests, 1)
	assert.Equal(t, "Você é o agente de conciliação.", prov.Requests[0].System)
}

func TestSystemPrompt_AgenteSemOverrideHerdaGlobal(t *testing.T) {
	agents := map[string]harness.AgentPolicy{
		"outro-agente": {SystemPrompt: "outro"},
	}
	h, prov := newPromptEnv(t, "global", agents)

	_, err := h.Run(context.Background(), pergunta("saldo?"), nil)

	require.NoError(t, err)
	require.Len(t, prov.Requests, 1)
	assert.Equal(t, "global", prov.Requests[0].System)
}

func TestSystemPrompt_VazioQuandoNaoConfigurado(t *testing.T) {
	h, prov := newPromptEnv(t, "", nil)

	_, err := h.Run(context.Background(), pergunta("saldo?"), nil)

	require.NoError(t, err)
	require.Len(t, prov.Requests, 1)
	assert.Empty(t, prov.Requests[0].System)
}
