package mcpclient_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/adapters/mcpclient"
)

// implServidor é a identidade do servidor MCP usado nos testes.
func implServidor() *mcp.Implementation {
	return &mcp.Implementation{Name: "servidor-teste", Version: "v0.0.1"}
}

// esquemaObjeto é o input schema mínimo aceito pelo servidor de teste.
func esquemaObjeto() map[string]any {
	return map[string]any{"type": "object"}
}

// registraEco adiciona a tool "eco", que devolve o texto "eco!".
func registraEco(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{Name: "eco", Description: "Devolve um texto fixo", InputSchema: esquemaObjeto()},
		func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "eco!"}}}, nil, nil
		})
}

// conectaMemoria sobe um servidor MCP em memória (antes do cliente) e devolve o
// mcpclient pronto para uso, com Name default "financeiro".
func conectaMemoria(t *testing.T, cfg mcpclient.Config, registrar func(*mcp.Server)) *mcpclient.Client {
	t.Helper()
	if cfg.Name == "" {
		cfg.Name = "financeiro"
	}
	server := mcp.NewServer(implServidor(), nil)
	if registrar != nil {
		registrar(server)
	}
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	sessao, err := server.Connect(context.Background(), serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sessao.Close() })

	client := mcpclient.New(cfg, mcpclient.Deps{Transport: clientTransport})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// fakeCredentials resolve a referência para um valor fixo e registra os pedidos.
type fakeCredentials struct {
	mu       sync.Mutex
	value    string
	err      error
	resolved []string
}

// Resolve implementa harness.CredentialProvider.
func (f *fakeCredentials) Resolve(_ context.Context, ref string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resolved = append(f.resolved, ref)
	if f.err != nil {
		return "", f.err
	}
	return f.value, nil
}

// refs devolve uma cópia das referências já resolvidas.
func (f *fakeCredentials) refs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.resolved...)
}

// transporteQuebrado sempre falha ao conectar e conta as tentativas.
type transporteQuebrado struct {
	tentativas atomic.Int32
}

// Connect implementa mcp.Transport.
func (t *transporteQuebrado) Connect(context.Context) (mcp.Connection, error) {
	t.tentativas.Add(1)
	return nil, errors.New("falha simulada de transporte")
}

// dialerMemoria entrega um par de transportes em memória por conexão e avisa o
// supervisor, permitindo simular reconexão sem rede real.
type dialerMemoria struct {
	servidor chan *mcp.InMemoryTransport
}

// Connect implementa mcp.Transport.
func (d *dialerMemoria) Connect(ctx context.Context) (mcp.Connection, error) {
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	select {
	case d.servidor <- serverTransport:
		return clientTransport.Connect(ctx)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// supervisora aceita cada conexão do dialer em um servidor MCP e publica as
// sessões criadas para o teste inspecionar/derrubar.
type supervisora struct {
	sessoes chan *mcp.ServerSession
}

// novaSupervisora sobe a goroutine de aceitação do servidor.
func novaSupervisora(server *mcp.Server, dialer *dialerMemoria) *supervisora {
	sup := &supervisora{sessoes: make(chan *mcp.ServerSession, 8)}
	go func() {
		for serverTransport := range dialer.servidor {
			sessao, err := server.Connect(context.Background(), serverTransport, nil)
			if err == nil {
				sup.sessoes <- sessao
			}
		}
	}()
	return sup
}

// sessao aguarda a próxima sessão criada pela supervisora.
func (s *supervisora) sessao(t *testing.T) *mcp.ServerSession {
	t.Helper()
	select {
	case sessao := <-s.sessoes:
		return sessao
	case <-time.After(2 * time.Second):
		t.Fatal("supervisora: nenhuma conexão recebida")
		return nil
	}
}

func TestClient_NewNaoConectaECloseEIdempotente(t *testing.T) {
	// Arrange
	transport := &transporteQuebrado{}
	client := mcpclient.New(mcpclient.Config{Name: "financeiro"}, mcpclient.Deps{Transport: transport})

	// Act
	primeiroClose := client.Close()
	segundoClose := client.Close()

	// Assert
	require.NoError(t, primeiroClose)
	require.NoError(t, segundoClose)
	assert.Zero(t, transport.tentativas.Load(), "New/Close sem uso não devem conectar")
}

func TestList_InjetaCredencialEHeadersFixos(t *testing.T) {
	// Arrange
	var mu sync.Mutex
	var capturados []http.Header
	server := mcp.NewServer(implServidor(), nil)
	registraEco(server)
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturados = append(capturados, r.Header.Clone())
		mu.Unlock()
		mcpHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(httpServer.Close)

	credenciais := &fakeCredentials{value: "chave-secreta-123"}
	client := mcpclient.New(
		mcpclient.Config{
			Name:          "financeiro",
			Endpoint:      httpServer.URL,
			CredentialRef: "env:FINANCEIRO_MCP_KEY",
			Headers:       map[string]string{"X-Harness-Teste": "fixo"},
		},
		mcpclient.Deps{Credentials: credenciais, HTTPClient: httpServer.Client()},
	)
	t.Cleanup(func() { _ = client.Close() })

	// Act
	tools, err := client.List(context.Background())

	// Assert
	require.NoError(t, err)
	assert.Len(t, tools, 1)
	assert.Contains(t, credenciais.refs(), "env:FINANCEIRO_MCP_KEY")

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, capturados)
	for _, header := range capturados {
		assert.Equal(t, "chave-secreta-123", header.Get("x-api-key"))
		assert.Equal(t, "fixo", header.Get("X-Harness-Teste"))
	}
}

func TestList_SemCredentialRefNaoInjetaAuth(t *testing.T) {
	// Arrange
	var mu sync.Mutex
	var capturados []http.Header
	server := mcp.NewServer(implServidor(), nil)
	registraEco(server)
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturados = append(capturados, r.Header.Clone())
		mu.Unlock()
		mcpHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(httpServer.Close)

	client := mcpclient.New(
		mcpclient.Config{
			Name:     "financeiro",
			Endpoint: httpServer.URL,
			Headers:  map[string]string{"X-Harness-Teste": "fixo"},
		},
		mcpclient.Deps{HTTPClient: httpServer.Client()},
	)
	t.Cleanup(func() { _ = client.Close() })

	// Act
	tools, err := client.List(context.Background())

	// Assert
	require.NoError(t, err)
	assert.Len(t, tools, 1)

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, capturados)
	for _, header := range capturados {
		assert.Empty(t, header.Get("x-api-key"), "referência vazia não injeta auth")
		assert.Equal(t, "fixo", header.Get("X-Harness-Teste"))
	}
}

func TestList_FalhaDeConexaoReconectaUmaVez(t *testing.T) {
	// Arrange
	transport := &transporteQuebrado{}
	client := mcpclient.New(mcpclient.Config{Name: "financeiro"}, mcpclient.Deps{Transport: transport})
	t.Cleanup(func() { _ = client.Close() })

	// Act
	_, err := client.List(context.Background())

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "falha simulada de transporte")
	assert.EqualValues(t, 2, transport.tentativas.Load(), "uma tentativa original + uma reconexão")
}

func TestCall_ReconectaAposFalhaDeSessao(t *testing.T) {
	// Arrange
	server := mcp.NewServer(implServidor(), nil)
	registraEco(server)
	dialer := &dialerMemoria{servidor: make(chan *mcp.InMemoryTransport, 8)}
	sup := novaSupervisora(server, dialer)
	client := mcpclient.New(mcpclient.Config{Name: "financeiro"}, mcpclient.Deps{Transport: dialer})
	t.Cleanup(func() { _ = client.Close() })

	_, err := client.List(context.Background())
	require.NoError(t, err)
	primeira := sup.sessao(t)
	require.NoError(t, primeira.Close())

	// Act
	result, err := client.Call(context.Background(), "eco", nil, nil)

	// Assert
	require.NoError(t, err)
	require.NotEmpty(t, result.Content)
	assert.Equal(t, "eco!", result.Content[0].Text)
	segunda := sup.sessao(t)
	assert.NotNil(t, segunda, "a chamada deve reabrir a sessão")
}

func TestCall_ContextoCanceladoNaoReconecta(t *testing.T) {
	// Arrange
	server := mcp.NewServer(implServidor(), nil)
	registraEco(server)
	dialer := &dialerMemoria{servidor: make(chan *mcp.InMemoryTransport, 8)}
	sup := novaSupervisora(server, dialer)
	client := mcpclient.New(mcpclient.Config{Name: "financeiro"}, mcpclient.Deps{Transport: dialer})
	t.Cleanup(func() { _ = client.Close() })

	_, err := client.List(context.Background())
	require.NoError(t, err)
	primeira := sup.sessao(t)
	require.NoError(t, primeira.Close())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Act
	_, err = client.Call(ctx, "eco", nil, nil)

	// Assert
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Never(t, func() bool { return len(sup.sessoes) > 0 }, 200*time.Millisecond, 20*time.Millisecond,
		"contexto cancelado não deve reconectar")
}
