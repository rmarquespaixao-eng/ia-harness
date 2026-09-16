package openai_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/adapters/provider/openai"
	"github.com/rmarquespaixao-eng/ia-harness/harness"
)

// fixtureSSE é um stream completo: texto em dois pedaços, tool call com
// arguments fragmentado em dois chunks, finish_reason e usage final.
const fixtureSSE = `data: {"id":"1","object":"chat.completion.chunk","model":"gpt-teste","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}
data: {"id":"1","object":"chat.completion.chunk","model":"gpt-teste","choices":[{"index":0,"delta":{"content":"Vou "},"finish_reason":null}]}
data: {"id":"1","object":"chat.completion.chunk","model":"gpt-teste","choices":[{"index":0,"delta":{"content":"buscar."},"finish_reason":null}]}
data: {"id":"1","object":"chat.completion.chunk","model":"gpt-teste","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"buscar","arguments":"{\"q\":"}}]},"finish_reason":null}]}
data: {"id":"1","object":"chat.completion.chunk","model":"gpt-teste","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"x\"}"}}]},"finish_reason":null}]}
data: {"id":"1","object":"chat.completion.chunk","model":"gpt-teste","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}
data: {"id":"1","object":"chat.completion.chunk","model":"gpt-teste","choices":[],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}
data: [DONE]

`

func TestChat_StreamingComTextoEToolCallFragmentada(t *testing.T) {
	// Arrange
	srv, req := servidorSSE(t, fixtureSSE)
	creds := &fakeCredentials{valor: "chave-teste"}
	prov := openai.New(
		openai.Config{BaseURL: srv.URL, CredentialRef: "env:OPENAI_KEY"},
		openai.Deps{Credentials: creds},
	)
	t.Cleanup(func() { _ = prov.Close() })
	chatReq := harness.ChatRequest{
		Model:  "gpt-teste",
		System: "Seja breve.",
		Messages: []harness.Message{
			{ID: "m1", Role: harness.RoleUser, Parts: []harness.Part{{Kind: harness.PartText, Text: "onde está o recibo?"}}},
		},
		Tools: []harness.Tool{{
			Name:        "buscar",
			Namespace:   "financeiro",
			Description: "Busca recibos",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`),
		}},
	}
	var deltas []string

	// Act
	resp, err := prov.Chat(context.Background(), chatReq, func(texto string) {
		deltas = append(deltas, texto)
	})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"Vou ", "buscar."}, deltas)
	assert.Equal(t, harness.RoleAssistant, resp.Message.Role)
	assert.Equal(t, harness.StopCompleted, resp.StopReason)
	assert.Equal(t, "gpt-teste", resp.Model)
	assert.Equal(t, harness.Usage{InputTokens: 11, OutputTokens: 7, Estimated: false}, resp.Usage)
	require.Len(t, resp.ToolCalls, 1)
	assert.Equal(t, "call_1", resp.ToolCalls[0].ID)
	assert.Equal(t, "buscar", resp.ToolCalls[0].Name)
	assert.JSONEq(t, `{"q":"x"}`, string(resp.ToolCalls[0].Args))
	require.Len(t, resp.Message.Parts, 2)
	assert.Equal(t, harness.PartText, resp.Message.Parts[0].Kind)
	assert.Equal(t, "Vou buscar.", resp.Message.Parts[0].Text)
	require.NotNil(t, resp.Message.Parts[1].Call)
	assert.Equal(t, "call_1", resp.Message.Parts[1].Call.ID)

	assert.Equal(t, []string{"env:OPENAI_KEY"}, creds.refs)
	metodo, caminho, header, corpo := req.snapshot()
	assert.Equal(t, "POST", metodo)
	assert.Equal(t, "/chat/completions", caminho)
	assert.Equal(t, "Bearer chave-teste", header.Get("Authorization"))
	assert.JSONEq(t, `{
		"model": "gpt-teste",
		"messages": [
			{"role": "system", "content": "Seja breve."},
			{"role": "user", "content": "onde está o recibo?"}
		],
		"tools": [{
			"type": "function",
			"function": {
				"name": "buscar",
				"description": "Busca recibos",
				"parameters": {"type": "object", "properties": {"q": {"type": "string"}}}
			}
		}],
		"stream": true,
		"stream_options": {"include_usage": true}
	}`, string(corpo))
}

func TestChat_StreamOptionsIncluidoNoRequest(t *testing.T) {
	// Arrange
	srv, req := servidorSSE(t, sseVazio)
	prov := openai.New(openai.Config{BaseURL: srv.URL}, openai.Deps{})
	t.Cleanup(func() { _ = prov.Close() })

	// Act
	_, err := prov.Chat(context.Background(), harness.ChatRequest{Model: "gpt-teste"}, nil)

	// Assert
	require.NoError(t, err)
	_, _, _, corpo := req.snapshot()
	var body struct {
		Stream        bool `json:"stream"`
		StreamOptions struct {
			IncludeUsage bool `json:"include_usage"`
		} `json:"stream_options"`
	}
	require.NoError(t, json.Unmarshal(corpo, &body))
	assert.True(t, body.Stream)
	assert.True(t, body.StreamOptions.IncludeUsage)
}

func TestChat_StreamCortadoSemDone_ErroSemEventoDuplicado(t *testing.T) {
	// Arrange
	const sseCortado = `data: {"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"Vou "},"finish_reason":null}]}
data: {"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"buscar."},"finish_reason":null}]}
data: {"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

`
	srv, _ := servidorSSE(t, sseCortado)
	prov := openai.New(openai.Config{BaseURL: srv.URL}, openai.Deps{})
	t.Cleanup(func() { _ = prov.Close() })
	var deltas []string

	// Act
	_, err := prov.Chat(context.Background(), harness.ChatRequest{Model: "gpt-teste"}, func(texto string) {
		deltas = append(deltas, texto)
	})

	// Assert
	var provErr *harness.ProviderError
	require.ErrorAs(t, err, &provErr)
	assert.Equal(t, "provider_stream", provErr.Code)
	assert.Contains(t, provErr.Error(), "[DONE]")
	assert.Contains(t, provErr.Error(), "finish_reason=stop")
	assert.Equal(t, []string{"Vou ", "buscar."}, deltas)
}

func TestChat_ArgumentosInvalidos_DevolveComoVeio(t *testing.T) {
	// Arrange
	const sseInvalido = `data: {"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"buscar","arguments":"{\"q\":"}}]},"finish_reason":null}]}
data: {"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}
data: [DONE]

`
	srv, _ := servidorSSE(t, sseInvalido)
	prov := openai.New(openai.Config{BaseURL: srv.URL}, openai.Deps{})
	t.Cleanup(func() { _ = prov.Close() })

	// Act
	resp, err := prov.Chat(context.Background(), harness.ChatRequest{Model: "gpt-teste"}, nil)

	// Assert
	require.NoError(t, err)
	require.Len(t, resp.ToolCalls, 1)
	assert.Equal(t, `{"q":`, string(resp.ToolCalls[0].Args))
	assert.False(t, json.Valid(resp.ToolCalls[0].Args), "a validação é do harness/loop")
}

func TestChat_UsageAusente_MarcaEstimado(t *testing.T) {
	// Arrange
	const sseSemUsage = `data: {"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}]}
data: {"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}
data: [DONE]

`
	srv, _ := servidorSSE(t, sseSemUsage)
	prov := openai.New(openai.Config{BaseURL: srv.URL}, openai.Deps{})
	t.Cleanup(func() { _ = prov.Close() })

	// Act
	resp, err := prov.Chat(context.Background(), harness.ChatRequest{Model: "gpt-teste"}, nil)

	// Assert
	require.NoError(t, err)
	assert.True(t, resp.Usage.Estimated)
	assert.Zero(t, resp.Usage.InputTokens)
	assert.Zero(t, resp.Usage.OutputTokens)
}
