package eval_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmarquespaixao/ia-harness/adapters/session/memory"
	"rmarquespaixao/ia-harness/eval"
	"rmarquespaixao/ia-harness/harness"
	"rmarquespaixao/ia-harness/internal/testutil"
)

func newEvalHarness(t *testing.T, provider *testutil.ScriptedProvider, tools *testutil.MemToolSource) *harness.Harness {
	t.Helper()
	cfg := harness.Config{
		Providers: map[string]harness.Provider{"p": provider},
		Models: map[string]harness.ModelProfile{
			"m": {Provider: "p", Model: "fake-1", Capabilities: harness.Capabilities{ToolCalling: true, Streaming: true}},
		},
		Credentials:  &testutil.FakeCredentialProvider{Values: map[string]string{}},
		Sessions:     memory.New(),
		Logger:       slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
		DefaultModel: "m",
		Policy:       harness.PolicyConfig{Default: harness.PolicyAllow},
	}
	if tools != nil {
		cfg.Tools = []harness.ToolSource{tools}
	}
	h, err := harness.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	return h
}

func TestRun_Relatorio(t *testing.T) {
	// Arrange — um caso só texto e um caso com tool.
	tools := &testutil.MemToolSource{
		Tools:   []harness.Tool{{Name: "consultar_saldo", InputSchema: []byte(`{"type":"object"}`)}},
		Results: map[string]harness.ToolResult{"consultar_saldo": {Content: []harness.ResultContent{{Kind: harness.ResultText, Text: "R$ 10"}}}},
	}
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{
		{Text: "seu saldo é R$ 10"},
		{ToolCalls: []harness.ToolCall{{ID: "c1", Name: "consultar_saldo", Args: []byte(`{}`)}}},
		{Text: "pronto"},
	}}
	h := newEvalHarness(t, provider, tools)
	cases := []eval.Case{
		{Name: "texto", Input: []harness.Part{{Kind: harness.PartText, Text: "saldo?"}}, Check: eval.ContainsText("R$ 10")},
		{Name: "tool", Input: []harness.Part{{Kind: harness.PartText, Text: "consulte"}}, Check: eval.UsedTool("consultar_saldo")},
	}

	// Act
	report := eval.Run(context.Background(), h, "user-1", "ag-1", cases)

	// Assert
	assert.True(t, report.OK())
	assert.Equal(t, 2, report.Passed)
	assert.Equal(t, 0, report.Failed)
	require.Len(t, report.Results, 2)
	assert.True(t, report.Results[0].Passed)
	assert.True(t, report.Results[1].Passed)
}

func TestRun_FalhaDeVerificacao(t *testing.T) {
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "nada"}}}
	h := newEvalHarness(t, provider, nil)
	report := eval.Run(context.Background(), h, "u", "a", []eval.Case{
		{Name: "esperado", Input: []harness.Part{{Kind: harness.PartText, Text: "?"}}, Check: eval.ContainsText("algo")},
	})
	assert.False(t, report.OK())
	assert.Equal(t, 1, report.Failed)
	require.Len(t, report.Results, 1)
	assert.Error(t, report.Results[0].Err)
}
