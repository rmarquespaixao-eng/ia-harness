package openai_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/adapters/provider/openai"
	"github.com/rmarquespaixao-eng/ia-harness/harness"
)

// sseVazio encerra um stream sem conteúdo.
const sseVazio = "data: [DONE]\n\n"

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

func TestChat_ErroHTTP429_NaoVazaCorpoNemHeaders(t *testing.T) {
	// Arrange
	const corpo = `{"error":{"message":"rate limit excedido"}}`
	srv, _ := servidorStatus(t, http.StatusTooManyRequests, corpo, map[string]string{"X-Segredo": "super-segredo"})
	prov := openai.New(openai.Config{BaseURL: srv.URL, DefaultModel: "gpt-teste"}, openai.Deps{})
	t.Cleanup(func() { _ = prov.Close() })

	// Act
	_, err := prov.Chat(context.Background(), harness.ChatRequest{}, nil)

	// Assert
	var provErr *harness.ProviderError
	require.ErrorAs(t, err, &provErr)
	assert.Equal(t, "provider_rate_limited", provErr.Code)
	assert.Contains(t, provErr.Error(), "HTTP 429")
	assert.Contains(t, provErr.Error(), "rate limit excedido")
	assert.NotContains(t, provErr.Error(), "super-segredo")
}

func TestChat_ErroHTTP500_TruncaMensagemGrande(t *testing.T) {
	// Arrange
	corpo := `{"error":{"message":"` + strings.Repeat("x", 1000) + `"}}`
	srv, _ := servidorStatus(t, http.StatusInternalServerError, corpo, nil)
	prov := openai.New(openai.Config{BaseURL: srv.URL, DefaultModel: "gpt-teste"}, openai.Deps{})
	t.Cleanup(func() { _ = prov.Close() })

	// Act
	_, err := prov.Chat(context.Background(), harness.ChatRequest{}, nil)

	// Assert
	var provErr *harness.ProviderError
	require.ErrorAs(t, err, &provErr)
	assert.Equal(t, "provider_unavailable", provErr.Code)
	assert.Contains(t, provErr.Error(), "…")
	assert.Less(t, len([]rune(provErr.Error())), 360)
}

func TestChat_TimeoutDoCliente(t *testing.T) {
	// Arrange
	libera := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-libera
	}))
	t.Cleanup(func() {
		close(libera)
		srv.Close()
	})
	prov := openai.New(openai.Config{BaseURL: srv.URL, DefaultModel: "gpt-teste", Timeout: 30 * time.Millisecond}, openai.Deps{})
	t.Cleanup(func() { _ = prov.Close() })

	// Act
	_, err := prov.Chat(context.Background(), harness.ChatRequest{}, nil)

	// Assert
	var provErr *harness.ProviderError
	require.ErrorAs(t, err, &provErr)
	assert.Equal(t, "provider_timeout", provErr.Code)
}

func TestChat_HeadersDeConfigAplicados(t *testing.T) {
	// Arrange
	srv, req := servidorSSE(t, sseVazio)
	prov := openai.New(openai.Config{BaseURL: srv.URL, Headers: map[string]string{"X-Titulo": "ia-harness"}}, openai.Deps{})
	t.Cleanup(func() { _ = prov.Close() })

	// Act
	_, err := prov.Chat(context.Background(), harness.ChatRequest{Model: "gpt-teste"}, nil)

	// Assert
	require.NoError(t, err)
	metodo, caminho, header, _ := req.snapshot()
	assert.Equal(t, http.MethodPost, metodo)
	assert.Equal(t, "/chat/completions", caminho)
	assert.Equal(t, "ia-harness", header.Get("X-Titulo"))
	assert.Equal(t, "text/event-stream", header.Get("Accept"))
}

func TestChat_SemCredentialRef_NaoEnviaAuthorization(t *testing.T) {
	// Arrange
	srv, req := servidorSSE(t, sseVazio)
	prov := openai.New(openai.Config{BaseURL: srv.URL, DefaultModel: "gpt-teste"}, openai.Deps{})
	t.Cleanup(func() { _ = prov.Close() })

	// Act
	_, err := prov.Chat(context.Background(), harness.ChatRequest{}, nil)

	// Assert
	require.NoError(t, err)
	_, _, header, _ := req.snapshot()
	assert.Empty(t, header.Get("Authorization"))
}

func TestChat_CredentialRefSemProviderInjetado_Erro(t *testing.T) {
	// Arrange
	srv, req := servidorSSE(t, sseVazio)
	prov := openai.New(openai.Config{BaseURL: srv.URL, CredentialRef: "env:OPENAI_KEY"}, openai.Deps{})
	t.Cleanup(func() { _ = prov.Close() })

	// Act
	_, err := prov.Chat(context.Background(), harness.ChatRequest{Model: "gpt-teste"}, nil)

	// Assert
	var provErr *harness.ProviderError
	require.ErrorAs(t, err, &provErr)
	assert.Equal(t, "provider_credential", provErr.Code)
	metodo, _, _, corpo := req.snapshot()
	assert.Empty(t, metodo, "nenhum request deve chegar ao servidor")
	assert.Empty(t, corpo)
}

func TestChat_DefaultModelUsadoQuandoRequestVazio(t *testing.T) {
	// Arrange
	srv, req := servidorSSE(t, sseVazio)
	prov := openai.New(openai.Config{BaseURL: srv.URL, DefaultModel: "modelo-default"}, openai.Deps{})
	t.Cleanup(func() { _ = prov.Close() })

	// Act
	resp, err := prov.Chat(context.Background(), harness.ChatRequest{}, nil)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "modelo-default", resp.Model)
	_, _, _, corpo := req.snapshot()
	var body map[string]any
	require.NoError(t, json.Unmarshal(corpo, &body))
	assert.Equal(t, "modelo-default", body["model"])
}

func TestNew_NaoResolveCredencial(t *testing.T) {
	// Arrange
	creds := &fakeCredentials{valor: "chave-teste"}

	// Act
	_ = openai.New(openai.Config{CredentialRef: "env:OPENAI_KEY"}, openai.Deps{Credentials: creds})

	// Assert
	assert.Empty(t, creds.refs)
}

func TestClose_Idempotente(t *testing.T) {
	// Arrange
	prov := openai.New(openai.Config{}, openai.Deps{})

	// Act
	primeiro := prov.Close()
	segundo := prov.Close()

	// Assert
	require.NoError(t, primeiro)
	require.NoError(t, segundo)
}
