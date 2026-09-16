package anthropic_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmarquespaixao/ia-harness/adapters/provider/anthropic"
	"rmarquespaixao/ia-harness/harness"
)

// sseVazio encerra um stream sem conteúdo.
const sseVazio = "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"

// fixtureSSE é um stream completo: texto em dois pedaços, bloco tool_use com
// input_json_delta fragmentado, usage em message_start/message_delta e ping.
const fixtureSSE = `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":25,"output_tokens":1}}}

event: ping
data: {"type":"ping"}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Vou "}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"buscar."}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"buscar","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"q\":"}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"x\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":42}}

event: message_stop
data: {"type":"message_stop"}

`

// fakeCredentials resolve a credencial fixa e registra as refs resolvidas.
type fakeCredentials struct {
	valor string
	err   error
	refs  []string
}

// Resolve implementa harness.CredentialProvider.
func (f *fakeCredentials) Resolve(_ context.Context, ref string) (string, error) {
	f.refs = append(f.refs, ref)
	if f.err != nil {
		return "", f.err
	}
	return f.valor, nil
}

var _ harness.CredentialProvider = (*fakeCredentials)(nil)

// requisicao guarda o último request recebido pelo servidor de teste.
type requisicao struct {
	mu      sync.Mutex
	metodo  string
	caminho string
	header  http.Header
	corpo   []byte
}

// capturar lê e guarda método, caminho, headers e corpo do request.
func (r *requisicao) capturar(req *http.Request) {
	corpo, _ := io.ReadAll(req.Body)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metodo = req.Method
	r.caminho = req.URL.Path
	r.header = req.Header.Clone()
	r.corpo = corpo
}

// snapshot devolve método, caminho, headers e corpo já capturados.
func (r *requisicao) snapshot() (string, string, http.Header, []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.metodo, r.caminho, r.header.Clone(), append([]byte(nil), r.corpo...)
}

// servidorSSE sobe um servidor que captura o request e responde o SSE fixo.
func servidorSSE(t *testing.T, sse string) (*httptest.Server, *requisicao) {
	t.Helper()
	req := &requisicao{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req.capturar(r)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, sse)
	}))
	t.Cleanup(srv.Close)
	return srv, req
}

// servidorStatus sobe um servidor que responde status e corpo fixos.
func servidorStatus(t *testing.T, status int, corpo string, header map[string]string) (*httptest.Server, *requisicao) {
	t.Helper()
	req := &requisicao{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req.capturar(r)
		for nome, valor := range header {
			w.Header().Set(nome, valor)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, corpo)
	}))
	t.Cleanup(srv.Close)
	return srv, req
}

func TestChat_StreamingComTextoEToolUse(t *testing.T) {
	// Arrange
	srv, req := servidorSSE(t, fixtureSSE)
	creds := &fakeCredentials{valor: "chave-teste"}
	prov := anthropic.New(
		anthropic.Config{BaseURL: srv.URL, CredentialRef: "env:ANTHROPIC_KEY"},
		anthropic.Deps{Credentials: creds},
	)
	t.Cleanup(func() { _ = prov.Close() })
	chatReq := harness.ChatRequest{
		Model:  "claude-teste",
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
	assert.Equal(t, "claude-teste", resp.Model)
	assert.Equal(t, harness.Usage{InputTokens: 25, OutputTokens: 42, Estimated: false}, resp.Usage)
	require.Len(t, resp.ToolCalls, 1)
	assert.Equal(t, "toolu_1", resp.ToolCalls[0].ID)
	assert.Equal(t, "buscar", resp.ToolCalls[0].Name)
	assert.JSONEq(t, `{"q":"x"}`, string(resp.ToolCalls[0].Args))
	require.Len(t, resp.Message.Parts, 2)
	assert.Equal(t, harness.PartText, resp.Message.Parts[0].Kind)
	assert.Equal(t, "Vou buscar.", resp.Message.Parts[0].Text)
	require.NotNil(t, resp.Message.Parts[1].Call)
	assert.Equal(t, "toolu_1", resp.Message.Parts[1].Call.ID)

	assert.Equal(t, []string{"env:ANTHROPIC_KEY"}, creds.refs)
	metodo, caminho, header, corpo := req.snapshot()
	assert.Equal(t, http.MethodPost, metodo)
	assert.Equal(t, "/v1/messages", caminho)
	assert.Equal(t, "chave-teste", header.Get("x-api-key"))
	assert.Equal(t, "2023-06-01", header.Get("anthropic-version"))
	assert.JSONEq(t, `{
		"model": "claude-teste",
		"system": "Seja breve.",
		"messages": [{"role": "user", "content": [{"type": "text", "text": "onde está o recibo?"}]}],
		"max_tokens": 4096,
		"stream": true,
		"tools": [{
			"name": "buscar",
			"description": "Busca recibos",
			"input_schema": {"type": "object", "properties": {"q": {"type": "string"}}}
		}]
	}`, string(corpo))
}

func TestChat_EventoDesconhecidoIgnorado(t *testing.T) {
	// Arrange
	const sseDesconhecido = `event: mystery
data: {"type":"mystery","payload":{"algo":1}}

event: ping
data: {"type":"ping"}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_stop
data: {"type":"message_stop"}

`
	srv, _ := servidorSSE(t, sseDesconhecido)
	prov := anthropic.New(anthropic.Config{BaseURL: srv.URL}, anthropic.Deps{})
	t.Cleanup(func() { _ = prov.Close() })

	// Act
	resp, err := prov.Chat(context.Background(), harness.ChatRequest{Model: "claude-teste"}, nil)

	// Assert
	require.NoError(t, err)
	require.Len(t, resp.Message.Parts, 1)
	assert.Equal(t, "ok", resp.Message.Parts[0].Text)
}

func TestChat_UsageAusente_MarcaEstimado(t *testing.T) {
	// Arrange
	const sseSemUsage = `event: message_start
data: {"type":"message_start","message":{"id":"msg_1"}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}

event: message_stop
data: {"type":"message_stop"}

`
	srv, _ := servidorSSE(t, sseSemUsage)
	prov := anthropic.New(anthropic.Config{BaseURL: srv.URL}, anthropic.Deps{})
	t.Cleanup(func() { _ = prov.Close() })

	// Act
	resp, err := prov.Chat(context.Background(), harness.ChatRequest{Model: "claude-teste"}, nil)

	// Assert
	require.NoError(t, err)
	assert.True(t, resp.Usage.Estimated)
	assert.Zero(t, resp.Usage.InputTokens)
	assert.Zero(t, resp.Usage.OutputTokens)
}

func TestChat_MapeiaConversaParaMessagesAPI(t *testing.T) {
	// Arrange
	srv, req := servidorSSE(t, sseVazio)
	prov := anthropic.New(anthropic.Config{BaseURL: srv.URL}, anthropic.Deps{})
	t.Cleanup(func() { _ = prov.Close() })
	chatReq := harness.ChatRequest{
		Model: "claude-teste",
		Messages: []harness.Message{
			{ID: "m1", Role: harness.RoleUser, Parts: []harness.Part{{Kind: harness.PartText, Text: "onde está o recibo?"}}},
			{ID: "a1", Role: harness.RoleAssistant, Parts: []harness.Part{
				{Kind: harness.PartText, Text: "Vou buscar."},
				{Kind: harness.PartToolCall, Call: &harness.ToolCall{ID: "toolu_9", Name: "buscar", Args: json.RawMessage(`{"q":"x"}`)}},
			}},
			{ID: "t1", Role: harness.RoleTool, Parts: []harness.Part{
				{Kind: harness.PartToolResult, Result: &harness.ToolResult{
					CallID:  "toolu_9",
					Content: []harness.ResultContent{{Kind: harness.ResultText, Text: "achou 2"}},
				}},
			}},
			{ID: "m2", Role: harness.RoleUser, Parts: []harness.Part{{Kind: harness.PartText, Text: "obrigado"}}},
		},
	}

	// Act
	_, err := prov.Chat(context.Background(), chatReq, nil)

	// Assert
	require.NoError(t, err)
	_, _, _, corpo := req.snapshot()
	assert.JSONEq(t, `{
		"model": "claude-teste",
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "onde está o recibo?"}]},
			{"role": "assistant", "content": [
				{"type": "text", "text": "Vou buscar."},
				{"type": "tool_use", "id": "toolu_9", "name": "buscar", "input": {"q": "x"}}
			]},
			{"role": "user", "content": [
				{"type": "tool_result", "tool_use_id": "toolu_9", "content": "achou 2"},
				{"type": "text", "text": "obrigado"}
			]}
		],
		"max_tokens": 4096,
		"stream": true
	}`, string(corpo))
}

func TestChat_StreamCortadoSemMessageStop_Erro(t *testing.T) {
	// Arrange
	const sseCortado = `event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}

`
	srv, _ := servidorSSE(t, sseCortado)
	prov := anthropic.New(anthropic.Config{BaseURL: srv.URL}, anthropic.Deps{})
	t.Cleanup(func() { _ = prov.Close() })
	var deltas []string

	// Act
	_, err := prov.Chat(context.Background(), harness.ChatRequest{Model: "claude-teste"}, func(texto string) {
		deltas = append(deltas, texto)
	})

	// Assert
	var provErr *harness.ProviderError
	require.ErrorAs(t, err, &provErr)
	assert.Equal(t, "provider_stream", provErr.Code)
	assert.Contains(t, provErr.Error(), "message_stop")
	assert.Equal(t, []string{"ok"}, deltas)
}

func TestChat_ErroHTTP429_NaoVazaCorpoNemHeaders(t *testing.T) {
	// Arrange
	const corpo = `{"type":"error","error":{"type":"rate_limit_error","message":"rate limit excedido"}}`
	srv, _ := servidorStatus(t, http.StatusTooManyRequests, corpo, map[string]string{"X-Segredo": "super-segredo"})
	prov := anthropic.New(anthropic.Config{BaseURL: srv.URL}, anthropic.Deps{})
	t.Cleanup(func() { _ = prov.Close() })

	// Act
	_, err := prov.Chat(context.Background(), harness.ChatRequest{Model: "claude-teste"}, nil)

	// Assert
	var provErr *harness.ProviderError
	require.ErrorAs(t, err, &provErr)
	assert.Equal(t, "provider_rate_limited", provErr.Code)
	assert.Contains(t, provErr.Error(), "HTTP 429")
	assert.Contains(t, provErr.Error(), "rate limit excedido")
	assert.NotContains(t, provErr.Error(), "super-segredo")
}

func TestChat_VersionEMaxTokensCustomizados(t *testing.T) {
	// Arrange
	srv, req := servidorSSE(t, sseVazio)
	prov := anthropic.New(anthropic.Config{BaseURL: srv.URL, Version: "2024-01-01", MaxTokens: 512}, anthropic.Deps{})
	t.Cleanup(func() { _ = prov.Close() })

	// Act
	_, err := prov.Chat(context.Background(), harness.ChatRequest{Model: "claude-teste", MaxOutputTokens: 128}, nil)

	// Assert
	require.NoError(t, err)
	_, _, header, corpo := req.snapshot()
	assert.Equal(t, "2024-01-01", header.Get("anthropic-version"))
	var body map[string]any
	require.NoError(t, json.Unmarshal(corpo, &body))
	assert.Equal(t, float64(128), body["max_tokens"])

	// Act (sem override: vale o default da configuração)
	_, err = prov.Chat(context.Background(), harness.ChatRequest{Model: "claude-teste"}, nil)

	// Assert
	require.NoError(t, err)
	_, _, _, corpo = req.snapshot()
	require.NoError(t, json.Unmarshal(corpo, &body))
	assert.Equal(t, float64(512), body["max_tokens"])
}

func TestChat_SemCredentialRef_NaoEnviaApiKey(t *testing.T) {
	// Arrange
	srv, req := servidorSSE(t, sseVazio)
	prov := anthropic.New(anthropic.Config{BaseURL: srv.URL}, anthropic.Deps{})
	t.Cleanup(func() { _ = prov.Close() })

	// Act
	_, err := prov.Chat(context.Background(), harness.ChatRequest{Model: "claude-teste"}, nil)

	// Assert
	require.NoError(t, err)
	_, _, header, _ := req.snapshot()
	assert.Empty(t, header.Get("x-api-key"))
}

func TestChat_ErroComMensagemTruncada(t *testing.T) {
	// Arrange
	corpo := `{"type":"error","error":{"type":"invalid_request_error","message":"` + strings.Repeat("x", 1000) + `"}}`
	srv, _ := servidorStatus(t, http.StatusBadRequest, corpo, nil)
	prov := anthropic.New(anthropic.Config{BaseURL: srv.URL}, anthropic.Deps{})
	t.Cleanup(func() { _ = prov.Close() })

	// Act
	_, err := prov.Chat(context.Background(), harness.ChatRequest{Model: "claude-teste"}, nil)

	// Assert
	var provErr *harness.ProviderError
	require.ErrorAs(t, err, &provErr)
	assert.Equal(t, "provider_http", provErr.Code)
	assert.Contains(t, provErr.Error(), "…")
	assert.Less(t, len([]rune(provErr.Error())), 360)
}

func TestNew_NaoResolveCredencial(t *testing.T) {
	// Arrange
	creds := &fakeCredentials{valor: "chave-teste"}

	// Act
	_ = anthropic.New(anthropic.Config{CredentialRef: "env:ANTHROPIC_KEY"}, anthropic.Deps{Credentials: creds})

	// Assert
	assert.Empty(t, creds.refs)
}

func TestClose_Idempotente(t *testing.T) {
	// Arrange
	prov := anthropic.New(anthropic.Config{}, anthropic.Deps{})

	// Act
	primeiro := prov.Close()
	segundo := prov.Close()

	// Assert
	require.NoError(t, primeiro)
	require.NoError(t, segundo)
}
