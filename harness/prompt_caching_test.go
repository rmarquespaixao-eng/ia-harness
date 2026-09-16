package harness_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmarquespaixao/ia-harness/adapters/session/memory"
	"rmarquespaixao/ia-harness/harness"
	"rmarquespaixao/ia-harness/internal/testutil"
)

func TestPromptCaching_CapacidadeChegaAoProvider(t *testing.T) {
	// Arrange — perfil com prompt caching ligado.
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "ok"}}}
	cfg := harness.Config{
		Providers: map[string]harness.Provider{turnProviderKey: provider},
		Models: map[string]harness.ModelProfile{
			turnModelAlias: {Provider: turnProviderKey, Model: "fake-1", Capabilities: harness.Capabilities{
				ToolCalling: true, Streaming: true, PromptCaching: true,
			}},
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

	// Act
	_, err = h.Run(context.Background(), pergunta("saldo?"), nil)

	// Assert
	require.NoError(t, err)
	require.Len(t, provider.Requests, 1)
	assert.True(t, provider.Requests[0].PromptCaching)
}
