package openai_responses_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmarquespaixao/ia-harness/adapters/provider/openai_responses"
	"rmarquespaixao/ia-harness/harness"
)

// fixtureSSE é um stream completo da Responses API: texto em dois pedaços, uma
// function_call com argumentos fragmentados e usage final.
const fixtureSSE = `data: {"type":"response.created"}
data: {"type":"response.output_text.delta","delta":"Vou "}
data: {"type":"response.output_text.delta","delta":"buscar."}
data: {"type":"response.output_item.added","item":{"type":"function_call","id":"item_1","call_id":"call_1","name":"buscar","arguments":""}}
data: {"type":"response.function_call_arguments.delta","item_id":"item_1","delta":"{\"q\":"}
data: {"type":"response.function_call_arguments.delta","item_id":"item_1","delta":"\"x\"}"}
data: {"type":"response.output_item.done","item":{"type":"function_call","id":"item_1","call_id":"call_1","name":"buscar"}}
data: {"type":"response.completed","response":{"usage":{"input_tokens":11,"output_tokens":7}}}
data: [DONE]

`

// fakeCredentials resolve uma ref fixa e registra as resoluções.
type fakeCredentials struct {
	valor string
	refs  []string
}

func (f *fakeCredentials) Resolve(_ context.Context, ref string) (string, error) {
	f.refs = append(f.refs, ref)
	return f.valor, nil
}

// captura guarda a requisição recebida pelo servidor de teste.
type captura struct {
	mu      sync.Mutex
	metodo  string
	caminho string
	header  http.Header
	corpo   []byte
}

func (c *captura) snapshot() (string, string, http.Header, []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.metodo, c.caminho, c.header, c.corpo
}

// servidorSSE sobe um httptest que devolve o corpo SSE informado e captura a
// requisição; status permite simular erro HTTP.
func servidorSSE(t *testing.T, status int, corpo string) (*httptest.Server, *captura) {
	t.Helper()
	cap := &captura{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		cap.mu.Lock()
		cap.metodo = r.Method
		cap.caminho = r.URL.Path
		cap.header = r.Header.Clone()
		cap.corpo = raw
		cap.mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, corpo)
	}))
	t.Cleanup(srv.Close)
	return srv, cap
}

func TestChat_StreamingComTextoEToolCallFragmentada(t *testing.T) {
	// Arrange
	srv, cap := servidorSSE(t, http.StatusOK, fixtureSSE)
	creds := &fakeCredentials{valor: "chave-teste"}
	prov := openai_responses.New(
		openai_responses.Config{BaseURL: srv.URL, CredentialRef: "env:OPENCODE_GO_API_KEY"},
		openai_responses.Deps{Credentials: creds},
	)
	t.Cleanup(func() { _ = prov.Close() })
	req := harness.ChatRequest{
		Model:     "gpt-5.6-luna",
		SessionID: "sess-1",
		System:    "Seja breve.",
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
	resp, err := prov.Chat(context.Background(), req, func(texto string) { deltas = append(deltas, texto) })

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"Vou ", "buscar."}, deltas)
	assert.Equal(t, harness.RoleAssistant, resp.Message.Role)
	assert.Equal(t, harness.StopCompleted, resp.StopReason)
	assert.Equal(t, "gpt-5.6-luna", resp.Model)
	assert.Equal(t, harness.Usage{InputTokens: 11, OutputTokens: 7, Estimated: false}, resp.Usage)
	require.Len(t, resp.ToolCalls, 1)
	assert.Equal(t, "call_1", resp.ToolCalls[0].ID)
	assert.Equal(t, "buscar", resp.ToolCalls[0].Name)
	assert.JSONEq(t, `{"q":"x"}`, string(resp.ToolCalls[0].Args))

	assert.Equal(t, []string{"env:OPENCODE_GO_API_KEY"}, creds.refs)
	metodo, caminho, header, corpo := cap.snapshot()
	assert.Equal(t, "POST", metodo)
	assert.Equal(t, "/responses", caminho)
	assert.Equal(t, "Bearer chave-teste", header.Get("Authorization"))
	var body map[string]any
	require.NoError(t, json.Unmarshal(corpo, &body))
	assert.Equal(t, "gpt-5.6-luna", body["model"])
	assert.Equal(t, true, body["stream"])
	assert.Equal(t, false, body["store"])
	tools, ok := body["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 1)
	first := tools[0].(map[string]any)
	assert.Equal(t, "function", first["type"])
	assert.Equal(t, "buscar", first["name"])
}

func TestChat_SemUsage_MarcaEstimado(t *testing.T) {
	// Arrange: response.completed sem usage.
	const semUso = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\ndata: {\"type\":\"response.completed\",\"response\":{}}\n\n"
	srv, _ := servidorSSE(t, http.StatusOK, semUso)
	prov := openai_responses.New(openai_responses.Config{BaseURL: srv.URL}, openai_responses.Deps{})
	t.Cleanup(func() { _ = prov.Close() })

	// Act
	resp, err := prov.Chat(context.Background(), harness.ChatRequest{Model: "m"}, nil)

	// Assert
	require.NoError(t, err)
	assert.True(t, resp.Usage.Estimated)
	assert.Equal(t, int64(0), resp.Usage.InputTokens)
}

func TestChat_StreamCortado_ErroDeStream(t *testing.T) {
	// Arrange: EOF antes de response.completed.
	const cortado = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"parcial\"}\n\n"
	srv, _ := servidorSSE(t, http.StatusOK, cortado)
	prov := openai_responses.New(openai_responses.Config{BaseURL: srv.URL}, openai_responses.Deps{})
	t.Cleanup(func() { _ = prov.Close() })

	// Act
	_, err := prov.Chat(context.Background(), harness.ChatRequest{Model: "m"}, nil)

	// Assert
	var provErr *harness.ProviderError
	require.ErrorAs(t, err, &provErr)
	assert.Equal(t, "provider_stream", provErr.Code)
}

func TestChat_EventoDesconhecido_Ignorado(t *testing.T) {
	// Arrange: eventos de reasoning futuros não quebram o parse.
	const comDesconhecido = "data: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"pensei\"}\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"resposta\"}\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}}\n\n"
	srv, _ := servidorSSE(t, http.StatusOK, comDesconhecido)
	prov := openai_responses.New(openai_responses.Config{BaseURL: srv.URL}, openai_responses.Deps{})
	t.Cleanup(func() { _ = prov.Close() })

	// Act
	resp, err := prov.Chat(context.Background(), harness.ChatRequest{Model: "m"}, nil)

	// Assert
	require.NoError(t, err)
	require.Len(t, resp.Message.Parts, 1)
	assert.Equal(t, "resposta", resp.Message.Parts[0].Text)
}

func TestChat_RateLimited_CodigoEstavel(t *testing.T) {
	// Arrange
	srv, _ := servidorSSE(t, http.StatusTooManyRequests, `{"error":{"message":"slow down"}}`)
	prov := openai_responses.New(openai_responses.Config{BaseURL: srv.URL}, openai_responses.Deps{})
	t.Cleanup(func() { _ = prov.Close() })

	// Act
	_, err := prov.Chat(context.Background(), harness.ChatRequest{Model: "m"}, nil)

	// Assert
	var provErr *harness.ProviderError
	require.ErrorAs(t, err, &provErr)
	assert.Equal(t, "provider_rate_limited", provErr.Code)
	assert.Contains(t, provErr.Message, "slow down")
}

func TestChat_HeadersDeSessaoEUserAgent(t *testing.T) {
	// Arrange
	srv, cap := servidorSSE(t, http.StatusOK, fixtureSSE)
	prov := openai_responses.New(openai_responses.Config{
		BaseURL:       srv.URL,
		Headers:       map[string]string{"User-Agent": "ia-harness/0.1"},
		SessionHeader: "x-opencode-session",
	}, openai_responses.Deps{})
	t.Cleanup(func() { _ = prov.Close() })

	// Act
	_, err := prov.Chat(context.Background(), harness.ChatRequest{Model: "m", SessionID: "sess-42"}, nil)

	// Assert
	require.NoError(t, err)
	_, _, header, _ := cap.snapshot()
	assert.Equal(t, "sess-42", header.Get("x-opencode-session"))
	assert.Equal(t, "ia-harness/0.1", header.Get("User-Agent"))
}

func TestChat_ContextoCancelado_NaoReconecta(t *testing.T) {
	// Arrange
	srv, _ := servidorSSE(t, http.StatusOK, fixtureSSE)
	prov := openai_responses.New(openai_responses.Config{BaseURL: srv.URL}, openai_responses.Deps{})
	t.Cleanup(func() { _ = prov.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()

	// Act
	_, err := prov.Chat(ctx, harness.ChatRequest{Model: "m"}, nil)

	// Assert
	require.Error(t, err)
	var provErr *harness.ProviderError
	require.ErrorAs(t, err, &provErr)
	assert.Contains(t, []string{"provider_canceled", "provider_timeout"}, provErr.Code)
}
