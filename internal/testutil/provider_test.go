package testutil_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/harness"
	"github.com/rmarquespaixao-eng/ia-harness/internal/testutil"
)

func TestScriptedProvider_ConsomeStepsNaOrdemEStreamaFatiado(t *testing.T) {
	// Arrange
	usage := harness.Usage{InputTokens: 11, OutputTokens: 7}
	chamada := harness.ToolCall{ID: "call-1", Name: "buscar", Namespace: "mcp.demo", Args: json.RawMessage(`{"q":"x"}`)}
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{
		{Text: "resposta", Usage: usage, StopReason: harness.StopCompleted},
		{Text: "seguinte", ToolCalls: []harness.ToolCall{chamada}, StopReason: harness.StopCompleted},
	}}
	var fragmentos []string
	var fragmentosSegundo []string

	// Act
	primeira, errPrimeiro := provider.Chat(context.Background(), harness.ChatRequest{Model: "modelo-1"}, func(s string) { fragmentos = append(fragmentos, s) })
	segunda, errSegundo := provider.Chat(context.Background(), harness.ChatRequest{Model: "modelo-1"}, func(s string) { fragmentosSegundo = append(fragmentosSegundo, s) })

	// Assert
	require.NoError(t, errPrimeiro)
	require.NoError(t, errSegundo)
	assert.Equal(t, []string{"resp", "osta"}, fragmentos)
	assert.Equal(t, []string{"segu", "inte"}, fragmentosSegundo)
	assert.Equal(t, harness.RoleAssistant, primeira.Message.Role)
	require.Len(t, primeira.Message.Parts, 1)
	assert.Equal(t, harness.PartText, primeira.Message.Parts[0].Kind)
	assert.Equal(t, "resposta", primeira.Message.Parts[0].Text)
	assert.Equal(t, usage, primeira.Usage)
	assert.Equal(t, harness.StopCompleted, primeira.StopReason)
	require.Len(t, segunda.ToolCalls, 1)
	assert.Equal(t, "call-1", segunda.ToolCalls[0].ID)
	assert.Equal(t, "buscar", segunda.ToolCalls[0].Name)
	assert.JSONEq(t, `{"q":"x"}`, string(segunda.ToolCalls[0].Args))
	assert.Equal(t, 2, provider.Calls)
	require.Len(t, provider.Requests, 2)
	assert.Equal(t, "modelo-1", provider.Requests[0].Model)
	assert.Equal(t, "modelo-1", provider.Requests[1].Model)
}

func TestScriptedProvider_TextoTemFragmentoUnicoAbaixoDeDoisCaracteres(t *testing.T) {
	// Arrange
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "x"}}}
	var fragmentos []string

	// Act
	_, err := provider.Chat(context.Background(), harness.ChatRequest{}, func(s string) { fragmentos = append(fragmentos, s) })

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"x"}, fragmentos)
}

func TestScriptedProvider_EsgotadoDevolveErro(t *testing.T) {
	// Arrange
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "oi"}}}
	_, errPrimeiro := provider.Chat(context.Background(), harness.ChatRequest{}, nil)
	require.NoError(t, errPrimeiro)

	// Act
	_, err := provider.Chat(context.Background(), harness.ChatRequest{}, nil)

	// Assert
	require.ErrorIs(t, err, testutil.ErrScriptExhausted)
	assert.ErrorContains(t, err, "script de provider esgotado")
	assert.Equal(t, 2, provider.Calls)
	assert.Len(t, provider.Requests, 2)
}

func TestScriptedProvider_StepComErroInterrompeSemStreaming(t *testing.T) {
	// Arrange
	sentinela := errors.New("falha do provedor")
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "não deve sair", Err: sentinela}}}
	var fragmentos []string

	// Act
	resposta, err := provider.Chat(context.Background(), harness.ChatRequest{}, func(s string) { fragmentos = append(fragmentos, s) })

	// Assert
	require.ErrorIs(t, err, sentinela)
	assert.Empty(t, fragmentos)
	assert.Equal(t, harness.ChatResponse{}, resposta)
	assert.Equal(t, 1, provider.Calls)
}

func TestScriptedProvider_Close(t *testing.T) {
	// Arrange
	provider := &testutil.ScriptedProvider{}

	// Act
	err := provider.Close()

	// Assert
	require.NoError(t, err)
	assert.True(t, provider.Closed)
}
