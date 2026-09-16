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

func TestParallelTools_ExecutaEmOrdemDeResultado(t *testing.T) {
	// Arrange — o modelo pede duas tools independentes no mesmo turno.
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{
		{ToolCalls: []harness.ToolCall{
			{ID: "c1", Name: "consultar_saldo", Args: []byte(`{"q":"saldo"}`)},
			{ID: "c2", Name: "consultar_limite", Args: []byte(`{"q":"limite"}`)},
		}},
		{Text: "pronto"},
	}}
	tools := &testutil.MemToolSource{
		Tools: []harness.Tool{
			{Name: "consultar_saldo", InputSchema: turnToolSchema},
			{Name: "consultar_limite", InputSchema: turnToolSchema},
		},
		Results: map[string]harness.ToolResult{
			"consultar_saldo":  {Content: []harness.ResultContent{{Kind: harness.ResultText, Text: "saldo"}}},
			"consultar_limite": {Content: []harness.ResultContent{{Kind: harness.ResultText, Text: "limite"}}},
		},
	}
	cfg := harness.Config{
		Providers: map[string]harness.Provider{turnProviderKey: provider},
		Models: map[string]harness.ModelProfile{
			turnModelAlias: {Provider: turnProviderKey, Model: "fake-1", Capabilities: harness.Capabilities{ToolCalling: true, Streaming: true}},
		},
		Tools:         []harness.ToolSource{tools},
		Credentials:   &testutil.FakeCredentialProvider{Values: map[string]string{}},
		Sessions:      memory.New(),
		Logger:        slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
		DefaultModel:  turnModelAlias,
		Policy:        harness.PolicyConfig{Default: harness.PolicyAllow},
		ParallelTools: true,
	}
	h, err := harness.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	handler := &turnHandler{}

	// Act
	result, err := h.Run(context.Background(), pergunta("saldo e limite"), handler)

	// Assert — as duas tools executaram e os eventos saem na ordem das chamadas.
	require.NoError(t, err)
	assert.Len(t, tools.Calls, 2)
	require.Len(t, handler.toolResults, 2)
	assert.Equal(t, "c1", handler.toolResults[0].CallID)
	assert.Equal(t, "c2", handler.toolResults[1].CallID)
	require.Len(t, result.ToolCalls, 2)
	assert.Equal(t, "c1", result.ToolCalls[0].CallID)
	assert.Equal(t, harness.StatusOK, result.ToolCalls[0].Status)
}

func TestParallelTools_ConfirmacaoCaiNoSequencial(t *testing.T) {
	// Arrange — uma tool exige confirmação: o lote inteiro NÃO é paralelizado.
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{
		{ToolCalls: []harness.ToolCall{
			{ID: "c1", Name: "consultar_saldo", Args: []byte(`{"q":"x"}`)},
			{ID: "c2", Name: "excluir", Args: []byte(`{"q":"x"}`)},
		}},
		{Text: "pronto"},
	}}
	tools := &testutil.MemToolSource{
		Tools: []harness.Tool{
			{Name: "consultar_saldo", InputSchema: turnToolSchema},
			{Name: "excluir", InputSchema: turnToolSchema},
		},
		Results: map[string]harness.ToolResult{
			"consultar_saldo": {Content: []harness.ResultContent{{Kind: harness.ResultText, Text: "saldo"}}},
			"excluir":         {Content: []harness.ResultContent{{Kind: harness.ResultText, Text: "ok"}}},
		},
	}
	cfg := harness.Config{
		Providers: map[string]harness.Provider{turnProviderKey: provider},
		Models: map[string]harness.ModelProfile{
			turnModelAlias: {Provider: turnProviderKey, Model: "fake-1", Capabilities: harness.Capabilities{ToolCalling: true, Streaming: true}},
		},
		Tools:         []harness.ToolSource{tools},
		Credentials:   &testutil.FakeCredentialProvider{Values: map[string]string{}},
		Sessions:      memory.New(),
		Logger:        slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
		DefaultModel:  turnModelAlias,
		ParallelTools: true,
		Policy: harness.PolicyConfig{Default: harness.PolicyAllow, Agents: map[string]harness.AgentPolicy{
			turnAgentID: {Mode: harness.PolicyAllow, ConfirmTools: []string{"excluir"}},
		}},
	}
	h, err := harness.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })

	// Act — o handler aprova a confirmação para o turno concluir.
	handler := &turnHandler{onToolResult: func(harness.ToolResultEvent) {}}
	result, err := h.Run(context.Background(), pergunta("x"), handler)

	// Assert
	require.NoError(t, err)
	require.Len(t, result.ToolCalls, 2)
	assert.Equal(t, "c1", result.ToolCalls[0].CallID)
	assert.Equal(t, "c2", result.ToolCalls[1].CallID)
}
