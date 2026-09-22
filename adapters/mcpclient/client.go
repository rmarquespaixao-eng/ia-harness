// Package mcpclient implementa a porta harness.ToolSource sobre o SDK MCP
// oficial (github.com/modelcontextprotocol/go-sdk): conexão preguiçosa via
// Streamable HTTP, autenticação resolvida por request, catálogo por sessão,
// progresso de tools longas e reconexão única (FR-004/FR-005/FR-009/FR-011).
package mcpclient

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rmarquespaixao-eng/ia-harness/harness"
	"github.com/rmarquespaixao-eng/ia-harness/internal/platform/httpx"
)

// implementationName identifica o harness perante os servidores MCP.
const implementationName = "ia-harness"

// implementationVersion é a versão anunciada na inicialização da sessão.
const implementationVersion = "v0.1.0"

// Config descreve o servidor MCP alvo e os padrões do catálogo.
type Config struct {
	// Name é o namespace das tools publicadas (o host monta "namespace.name").
	Name string
	// Endpoint é a URL do endpoint Streamable HTTP; ignorado quando
	// Deps.Transport é injetado ou Command está definido.
	Endpoint string
	// CredentialRef é a referência resolvida por Deps.Credentials a cada
	// request; vazia significa servidor sem autenticação (FR-004).
	CredentialRef string
	// ToolTimeout é o timeout sugerido por tool, exposto em harness.Tool.Timeout.
	ToolTimeout time.Duration
	// Headers são headers fixos acrescentados a cada request do transporte.
	Headers map[string]string
	// Command é o executável do servidor stdio; mutuamente exclusivo com Endpoint.
	Command string
	// Args são os argumentos do executável (argv explícito, sem shell).
	Args []string
	// Env são variáveis de ambiente adicionais do processo filho.
	Env map[string]string
	// EnvCredentials mapeia variável → CredentialRef; resolvidas a cada início.
	EnvCredentials map[string]string
	// Dir é o diretório de trabalho do filho; vazio usa o do host.
	Dir string
	// TerminateTimeout é o tempo máximo para encerrar o processo; 0 = 5s.
	TerminateTimeout time.Duration
}

// Deps reúne as dependências injetadas pelo host (D-12).
type Deps struct {
	// Credentials resolve Config.CredentialRef a cada request (FR-004).
	Credentials harness.CredentialProvider
	// HTTPClient é o cliente HTTP do transporte Streamable; nil usa o default.
	HTTPClient *http.Client
	// Logger é o log estruturado repassado ao SDK; nil desativa.
	Logger *slog.Logger
	// Transport é um transporte pronto (testes com mcp.NewInMemoryTransports);
	// não-nulo é usado direto e dispensa Endpoint/HTTPClient.
	Transport mcp.Transport
}

// Client é um ToolSource MCP com conexão preguiçosa, cache de catálogo por
// sessão e reconexão única por operação (T027–T030).
type Client struct {
	cfg  Config
	deps Deps

	mu      sync.Mutex
	session *mcp.ClientSession
	catalog []harness.Tool

	progress *progressRegistry
}

// Client implementa harness.ToolSource.
var _ harness.ToolSource = (*Client)(nil)

// New guarda configuração e dependências; a conexão acontece na primeira
// operação (List/Call).
func New(cfg Config, deps Deps) *Client {
	return &Client{
		cfg:      cfg,
		deps:     deps,
		progress: newProgressRegistry(),
	}
}

// Close encerra a sessão atual, se houver; é idempotente.
func (c *Client) Close() error {
	c.mu.Lock()
	cs := c.session
	c.session = nil
	c.catalog = nil
	c.mu.Unlock()

	if cs == nil {
		return nil
	}
	if err := cs.Close(); err != nil {
		return fmt.Errorf("mcpclient: fechar sessão de %q: %w", c.cfg.Name, err)
	}
	return nil
}

// ensureSession devolve a sessão ativa, conectando sob demanda (lazy).
func (c *Client) ensureSession(ctx context.Context) (*mcp.ClientSession, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session != nil {
		return c.session, nil
	}
	return c.connectLocked(ctx)
}

// connectLocked cria o cliente/sessão do SDK e publica em c.session. Requer
// c.mu travado pelo chamador.
func (c *Client) connectLocked(ctx context.Context) (*mcp.ClientSession, error) {
	sdk := mcp.NewClient(
		&mcp.Implementation{Name: implementationName, Version: implementationVersion},
		&mcp.ClientOptions{
			Logger:                      c.deps.Logger,
			ProgressNotificationHandler: c.handleProgress,
		},
	)

	transport, err := c.transport()
	if err != nil {
		return nil, err
	}
	cs, err := sdk.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcpclient: conectar a %q: %w", c.cfg.Name, err)
	}
	c.session = cs
	c.catalog = nil
	return cs, nil
}

// transport devolve o transporte injetado, monta o Streamable HTTP ou o stdio.
func (c *Client) transport() (mcp.Transport, error) {
	if c.deps.Transport != nil {
		return c.deps.Transport, nil
	}
	if err := c.cfg.validateStdio(); err != nil {
		return nil, err
	}
	if c.cfg.Command != "" {
		return &stdioTransport{cfg: c.cfg, deps: c.deps}, nil
	}
	return &mcp.StreamableClientTransport{
		Endpoint:   c.cfg.Endpoint,
		HTTPClient: c.httpClient(),
	}, nil
}

// httpClient clona o cliente HTTP do host e instala o RoundTripper de auth
// quando há credencial ou headers fixos.
func (c *Client) httpClient() *http.Client {
	base := httpx.NoCrossHostRedirect(c.deps.HTTPClient)
	if c.cfg.CredentialRef == "" && len(c.cfg.Headers) == 0 {
		return base
	}
	clone := *base
	clone.Transport = &authTransport{
		base:     base.Transport,
		provider: c.deps.Credentials,
		ref:      c.cfg.CredentialRef,
		headers:  c.cfg.Headers,
	}
	return &clone
}

// handleProgress converte a notificação do SDK no evento canônico e o entrega
// ao callback registrado para o progressToken da chamada (T029).
func (c *Client) handleProgress(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
	if req == nil || req.Params == nil {
		return
	}
	c.progress.dispatch(req.Params.ProgressToken, harness.ProgressUpdate{
		Message:  req.Params.Message,
		Progress: req.Params.Progress,
		Total:    req.Params.Total,
	})
}

// authTransport resolve a credencial a cada request e injeta x-api-key e os
// headers fixos, sem guardar segredo em memória (FR-004).
type authTransport struct {
	base     http.RoundTripper
	provider harness.CredentialProvider
	ref      string
	headers  map[string]string
}

// RoundTrip implementa http.RoundTripper.
func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())

	if t.ref != "" {
		if t.provider == nil {
			return nil, fmt.Errorf("mcpclient: CredentialRef %q sem CredentialProvider injetado", t.ref)
		}
		key, err := t.provider.Resolve(req.Context(), t.ref)
		if err != nil {
			return nil, fmt.Errorf("mcpclient: resolver credencial %q: %w", t.ref, err)
		}
		if key != "" {
			clone.Header.Set("x-api-key", key)
		}
	}
	for name, value := range t.headers {
		clone.Header.Set(name, value)
	}

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}

var _ http.RoundTripper = (*authTransport)(nil)
