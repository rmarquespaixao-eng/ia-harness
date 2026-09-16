package testutil_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmarquespaixao/ia-harness/harness"
	"rmarquespaixao/ia-harness/internal/testutil"
)

func TestMemToolSource_ListaExecutaComProgressoNaOrdem(t *testing.T) {
	// Arrange
	tools := []harness.Tool{{Name: "buscar", Namespace: "mcp.demo", Description: "busca"}}
	resultado := harness.ToolResult{CallID: "call-1", Content: []harness.ResultContent{{Kind: harness.ResultText, Text: "ok"}}}
	source := &testutil.MemToolSource{
		Tools:    tools,
		Results:  map[string]harness.ToolResult{"buscar": resultado},
		Progress: map[string][]harness.ProgressUpdate{"buscar": {{Message: "a", Progress: 1}, {Message: "b", Progress: 2}}},
	}
	var updates []harness.ProgressUpdate

	// Act
	listados, errList := source.List(context.Background())
	obtido, errCall := source.Call(context.Background(), "buscar", json.RawMessage(`{"q":"x"}`), func(u harness.ProgressUpdate) { updates = append(updates, u) })

	// Assert
	require.NoError(t, errList)
	require.NoError(t, errCall)
	assert.Equal(t, tools, listados)
	assert.Equal(t, resultado, obtido)
	require.Len(t, updates, 2)
	assert.Equal(t, "a", updates[0].Message)
	assert.Equal(t, 1.0, updates[0].Progress)
	assert.Equal(t, "b", updates[1].Message)
	assert.Equal(t, 2.0, updates[1].Progress)
	require.Len(t, source.Calls, 1)
	assert.Equal(t, "buscar", source.Calls[0].Name)
	assert.JSONEq(t, `{"q":"x"}`, string(source.Calls[0].Args))
}

func TestMemToolSource_ErroConfiguradoEDesconhecida(t *testing.T) {
	// Arrange
	sentinela := errors.New("tool falhou")
	source := &testutil.MemToolSource{
		Results: map[string]harness.ToolResult{"falha": {CallID: "c1"}},
		Errors:  map[string]error{"falha": sentinela},
	}

	// Act
	_, errFalha := source.Call(context.Background(), "falha", nil, nil)
	_, errDesconhecida := source.Call(context.Background(), "ausente", nil, nil)

	// Assert
	require.ErrorIs(t, errFalha, sentinela)
	require.Error(t, errDesconhecida)
	assert.ErrorContains(t, errDesconhecida, `tool "ausente" não registrada`)
	assert.Len(t, source.Calls, 2)
}

func TestMemToolSource_Close(t *testing.T) {
	// Arrange
	source := &testutil.MemToolSource{}

	// Act
	err := source.Close()

	// Assert
	require.NoError(t, err)
	assert.True(t, source.Closed)
}
