package openai_responses

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/harness"
)

func TestBuildBody_HistoricoComToolCallEResultado(t *testing.T) {
	// Arrange
	req := harness.ChatRequest{
		Model:  "gpt-5.6-luna",
		System: "Seja breve.",
		Messages: []harness.Message{
			{ID: "m1", Role: harness.RoleUser, Parts: []harness.Part{{Kind: harness.PartText, Text: "saldo?"}}},
			{ID: "m2", Role: harness.RoleAssistant, Parts: []harness.Part{
				{Kind: harness.PartText, Text: "vou consultar"},
				{Kind: harness.PartToolCall, Call: &harness.ToolCall{ID: "call_1", Name: "buscar", Args: json.RawMessage(`{"q":"saldo"}`)}},
			}},
			{ID: "m3", Role: harness.RoleTool, Parts: []harness.Part{
				{Kind: harness.PartToolResult, Result: &harness.ToolResult{
					CallID:  "call_1",
					Content: []harness.ResultContent{{Kind: harness.ResultText, Text: "R$ 10"}},
				}},
			}},
		},
		Tools: []harness.Tool{{Name: "buscar", Description: "Busca", InputSchema: json.RawMessage(`{"type":"object"}`)}},
	}

	// Act
	raw, err := buildBody("gpt-5.6-luna", req)

	// Assert
	require.NoError(t, err)
	var body struct {
		Store bool  `json:"store"`
		Input []any `json:"input"`
		Tools []struct {
			Type       string `json:"type"`
			Name       string `json:"name"`
			Parameters any    `json:"parameters"`
		} `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(raw, &body))
	assert.False(t, body.Store, "Responses usa store=false (histórico é do host)")
	require.Len(t, body.Input, 5, "system + user + assistant + function_call + function_call_output")
	// function_call carrega call_id/name/arguments
	call := body.Input[3].(map[string]any)
	assert.Equal(t, "function_call", call["type"])
	assert.Equal(t, "call_1", call["call_id"])
	assert.Equal(t, "buscar", call["name"])
	assert.JSONEq(t, `{"q":"saldo"}`, call["arguments"].(string))
	// function_call_output carrega call_id/output
	out := body.Input[4].(map[string]any)
	assert.Equal(t, "function_call_output", out["type"])
	assert.Equal(t, "call_1", out["call_id"])
	assert.Equal(t, "R$ 10", out["output"])
	// tools no formato achatado da Responses
	require.Len(t, body.Tools, 1)
	assert.Equal(t, "function", body.Tools[0].Type)
	assert.Equal(t, "buscar", body.Tools[0].Name)
}

func TestBuildBody_ParamsPrevaleceComProtocolo(t *testing.T) {
	// Arrange: Params traz max_output_tokens; o protocolo sobrescreve com o valor do request.
	req := harness.ChatRequest{
		Model:           "m",
		Params:          map[string]any{"temperature": 0.2, "max_output_tokens": 1},
		MaxOutputTokens: 2048,
	}

	// Act
	raw, err := buildBody("m", req)

	// Assert
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))
	assert.Equal(t, 0.2, body["temperature"])
	assert.Equal(t, float64(2048), body["max_output_tokens"])
}
