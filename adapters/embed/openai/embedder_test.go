package openai_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/adapters/embed/openai"
	"github.com/rmarquespaixao-eng/ia-harness/harness"
)

// fakeCredentials resolve a credencial fixa e registra as refs resolvidas.
type fakeCredentials struct {
	value string
	err   error
	refs  []string
}

// Resolve implementa harness.CredentialProvider.
func (f *fakeCredentials) Resolve(_ context.Context, ref string) (string, error) {
	f.refs = append(f.refs, ref)
	if f.err != nil {
		return "", f.err
	}
	return f.value, nil
}

var _ harness.CredentialProvider = (*fakeCredentials)(nil)

// capture guarda o último request recebido pelo servidor de teste.
type capture struct {
	mu     sync.Mutex
	hits   int
	method string
	path   string
	header http.Header
	body   []byte
}

// read lê e guarda método, caminho, headers e corpo do request.
func (c *capture) read(r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hits++
	c.method = r.Method
	c.path = r.URL.Path
	c.header = r.Header.Clone()
	c.body = body
}

// snapshot devolve o estado capturado até agora.
func (c *capture) snapshot() (int, string, string, http.Header, []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits, c.method, c.path, c.header.Clone(), append([]byte(nil), c.body...)
}

// servidorEmbeddings sobe um servidor que captura o request e responde status e
// corpo fixos.
func servidorEmbeddings(t *testing.T, status int, corpo string, header map[string]string) (*httptest.Server, *capture) {
	t.Helper()
	cap := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.read(r)
		for nome, valor := range header {
			w.Header().Set(nome, valor)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, corpo)
	}))
	t.Cleanup(srv.Close)
	return srv, cap
}

func TestEmbedSucessoReordenaPorIndiceEEnviaCredencial(t *testing.T) {
	// Arrange
	const resposta = `{"data":[{"index":1,"embedding":[3,4]},{"index":0,"embedding":[1,2]}]}`
	srv, cap := servidorEmbeddings(t, http.StatusOK, resposta, nil)
	creds := &fakeCredentials{value: "chave-secreta"}
	emb := openai.New(openai.Config{
		BaseURL:       srv.URL + "/",
		CredentialRef: "env:EMBED_KEY",
		Model:         "text-embedding-3-small",
	}, openai.Deps{Credentials: creds})

	// Act
	vectors, err := emb.Embed(context.Background(), []string{"a", "b"})

	// Assert
	require.NoError(t, err)
	require.Len(t, vectors, 2)
	assert.Equal(t, []float32{1, 2}, vectors[0], "data[] é reordenado pelo índice")
	assert.Equal(t, []float32{3, 4}, vectors[1])

	hits, method, path, header, body := cap.snapshot()
	require.Equal(t, 1, hits)
	assert.Equal(t, http.MethodPost, method)
	assert.Equal(t, "/embeddings", path, "BaseURL com barra final não duplica o separador")
	assert.Equal(t, "Bearer chave-secreta", header.Get("Authorization"))
	assert.Equal(t, "application/json", header.Get("Content-Type"))
	assert.Equal(t, []string{"env:EMBED_KEY"}, creds.refs)

	var sent struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}
	require.NoError(t, json.Unmarshal(body, &sent))
	assert.Equal(t, "text-embedding-3-small", sent.Model)
	assert.Equal(t, []string{"a", "b"}, sent.Input)
}

func TestEmbedErroHTTP429NaoVazaCorpoNemHeaders(t *testing.T) {
	// Arrange
	const corpo = `{"error":{"message":"rate limit excedido"}}`
	srv, _ := servidorEmbeddings(t, http.StatusTooManyRequests, corpo, map[string]string{"X-Segredo": "super-segredo"})
	emb := openai.New(openai.Config{BaseURL: srv.URL}, openai.Deps{})

	// Act
	_, err := emb.Embed(context.Background(), []string{"a"})

	// Assert
	var provErr *harness.ProviderError
	require.ErrorAs(t, err, &provErr)
	assert.Equal(t, "provider_rate_limited", provErr.Code)
	assert.Contains(t, provErr.Error(), "HTTP 429")
	assert.Contains(t, provErr.Error(), "rate limit excedido")
	assert.NotContains(t, provErr.Error(), "super-segredo")
}

func TestEmbedSemCredentialProviderFalhaAntesDoHTTP(t *testing.T) {
	// Arrange
	srv, cap := servidorEmbeddings(t, http.StatusOK, `{"data":[]}`, nil)
	emb := openai.New(openai.Config{BaseURL: srv.URL, CredentialRef: "env:EMBED_KEY"}, openai.Deps{})

	// Act
	_, err := emb.Embed(context.Background(), []string{"a"})

	// Assert
	var provErr *harness.ProviderError
	require.ErrorAs(t, err, &provErr)
	assert.Equal(t, "provider_credential", provErr.Code)
	hits, _, _, _, _ := cap.snapshot()
	assert.Zero(t, hits, "sem resolvedor, nenhum request sai")
}

func TestEmbedErroDeCredencialPreservaCausa(t *testing.T) {
	// Arrange
	sentinela := errors.New("segredo indisponível")
	emb := openai.New(
		openai.Config{BaseURL: "http://127.0.0.1:1", CredentialRef: "env:EMBED_KEY"},
		openai.Deps{Credentials: &fakeCredentials{err: sentinela}},
	)

	// Act
	_, err := emb.Embed(context.Background(), []string{"a"})

	// Assert
	var provErr *harness.ProviderError
	require.ErrorAs(t, err, &provErr)
	assert.Equal(t, "provider_credential", provErr.Code)
	assert.ErrorIs(t, err, sentinela, "causa preservada para errors.Is")
}

func TestEmbedSemTextosNaoFazHTTP(t *testing.T) {
	// Arrange
	srv, cap := servidorEmbeddings(t, http.StatusOK, `{"data":[]}`, nil)
	emb := openai.New(openai.Config{BaseURL: srv.URL}, openai.Deps{})

	// Act
	_, err := emb.Embed(context.Background(), nil)

	// Assert
	var provErr *harness.ProviderError
	require.ErrorAs(t, err, &provErr)
	assert.Equal(t, "provider_request", provErr.Code)
	hits, _, _, _, _ := cap.snapshot()
	assert.Zero(t, hits)
}

func TestEmbedRespostaInvalida(t *testing.T) {
	tests := []struct {
		name   string
		corpo  string
		trecho string
	}{
		{"json inválido", `{`, "decodificar"},
		{"quantidade divergente", `{"data":[{"index":0,"embedding":[1]}]}`, "esperado 2"},
		{"índice fora da faixa", `{"data":[{"index":5,"embedding":[1]},{"index":0,"embedding":[2]}]}`, "fora da faixa"},
		{"índice duplicado", `{"data":[{"index":0,"embedding":[1]},{"index":0,"embedding":[2]}]}`, "duplicado"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			srv, _ := servidorEmbeddings(t, http.StatusOK, tt.corpo, nil)
			emb := openai.New(openai.Config{BaseURL: srv.URL}, openai.Deps{})

			// Act
			_, err := emb.Embed(context.Background(), []string{"a", "b"})

			// Assert
			var provErr *harness.ProviderError
			require.ErrorAs(t, err, &provErr)
			assert.Equal(t, "provider_request", provErr.Code)
			assert.Contains(t, provErr.Error(), tt.trecho)
		})
	}
}
