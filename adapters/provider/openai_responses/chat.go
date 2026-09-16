package openai_responses

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rmarquespaixao-eng/ia-harness/harness"
	"github.com/rmarquespaixao-eng/ia-harness/internal/platform/httpx"
	"github.com/rmarquespaixao-eng/ia-harness/internal/platform/providerhttp"
	"github.com/rmarquespaixao-eng/ia-harness/internal/platform/retry"
)

const (
	// defaultBaseURL é usada quando Config.BaseURL é vazia (OpenAI oficial).
	defaultBaseURL = "https://api.openai.com/v1"
	// maxErrorBodyBytes limita a leitura do corpo de erro do provedor.
	maxErrorBodyBytes = 8 << 10
	// maxErrorMessageRunes limita a mensagem de erro exposta ao host.
	maxErrorMessageRunes = 300
)

// Códigos estáveis de harness.ProviderError devolvidos pelo adaptador.
const (
	codeRequest     = "provider_request"
	codeCredential  = "provider_credential"
	codeTransport   = "provider_transport"
	codeTimeout     = "provider_timeout"
	codeCanceled    = "provider_canceled"
	codeHTTP        = "provider_http"
	codeRateLimited = "provider_rate_limited"
	codeUnavailable = "provider_unavailable"
	codeStream      = "provider_stream"
)

// Config descreve o endpoint e os padrões do adaptador da Responses API.
type Config struct {
	// BaseURL é a raiz da API (ex.: "https://opencode.ai/zen/go/v1"); o
	// adaptador acrescenta "/responses".
	BaseURL string
	// CredentialRef é resolvida por request via Deps.Credentials; vazia
	// dispensa Authorization.
	CredentialRef string
	// DefaultModel é usado quando ChatRequest.Model é vazio.
	DefaultModel string
	// Headers são headers fixos acrescentados a cada request (ex.: User-Agent).
	Headers map[string]string
	// SessionHeader, se não vazio, recebe o SessionID do turno (ex.:
	// "x-opencode-session" exigido pelo OpenCode Go — FR-ZG-004).
	SessionHeader string
	// Timeout é aplicado ao cliente HTTP (cria um quando Deps.HTTPClient é nil).
	Timeout time.Duration
	// MaxAttempts é o total de tentativas por chamada (0 = default 3); o retry
	// só ocorre antes de consumir o stream (429/timeout/5xx/transporte — 007).
	MaxAttempts int
	// RetryBaseDelay/RetryMaxDelay controlam o backoff (defaults 200ms/2s).
	RetryBaseDelay time.Duration
	RetryMaxDelay  time.Duration
}

// Deps reúne as dependências injetadas pelo host (D-12).
type Deps struct {
	Credentials harness.CredentialProvider
	HTTPClient  *http.Client
	Clock       harness.Clock
	Logger      *slog.Logger
}

// Provider é o adaptador da Responses API.
type Provider struct {
	cfg    Config
	deps   Deps
	client *http.Client

	mu     sync.Mutex
	closed bool
}

// Provider implementa harness.Provider.
var _ harness.Provider = (*Provider)(nil)

// New guarda configuração e dependências; a credencial só é resolvida na
// chamada, nunca aqui.
func New(cfg Config, deps Deps) *Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = defaultMaxAttempts
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &Provider{
		cfg:    cfg,
		deps:   deps,
		client: newHTTPClient(cfg.Timeout, deps.HTTPClient),
	}
}

// defaultMaxAttempts é o total de tentativas quando a config não define.
const defaultMaxAttempts = 3

// retryPolicy monta a política de retry do adaptador.
func (p *Provider) retryPolicy() retry.Policy {
	return retry.Policy{
		MaxAttempts: p.cfg.MaxAttempts,
		BaseDelay:   p.cfg.RetryBaseDelay,
		MaxDelay:    p.cfg.RetryMaxDelay,
		Jitter:      true,
	}
}

// newHTTPClient aplica Config.Timeout ao cliente injetado (cópia) ou cria um
// quando Deps.HTTPClient é nil; impede redirect cross-host com credencial.
func newHTTPClient(timeout time.Duration, base *http.Client) *http.Client {
	if base == nil {
		base = &http.Client{}
	}
	client := httpx.NoCrossHostRedirect(base)
	if timeout > 0 {
		client.Timeout = timeout
	}
	return client
}

// Close encerra as conexões ociosas do cliente HTTP; é idempotente.
func (p *Provider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	p.client.CloseIdleConnections()
	return nil
}

// Chat implementa harness.Provider delegando ao ChatStream com um sink de texto.
func (p *Provider) Chat(ctx context.Context, req harness.ChatRequest, onText func(string)) (harness.ChatResponse, error) {
	return p.ChatStream(ctx, req, harness.TextSink(onText))
}

// ChatStream faz a chamada em streaming para {BaseURL}/responses e devolve a
// resposta canônica; texto, raciocínio e fragmentos de argumentos de tool
// chegam ao sink (feature 012).
func (p *Provider) ChatStream(ctx context.Context, req harness.ChatRequest, sink harness.StreamSink) (harness.ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.DefaultModel
	}

	body, err := buildBody(model, req)
	if err != nil {
		return harness.ChatResponse{}, providerError(codeRequest, fmt.Sprintf("montar request de %q", model), err)
	}

	start := p.now()
	resp, err := providerhttp.Do(ctx, p.retryPolicy(), func(ctx context.Context) (*http.Response, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL+"/responses", bytes.NewReader(body))
		if err != nil {
			return nil, providerError(codeRequest, "criar request HTTP", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "text/event-stream")
		if err := p.authorize(ctx, httpReq); err != nil {
			return nil, err
		}
		if p.cfg.SessionHeader != "" && req.SessionID != "" {
			httpReq.Header.Set(p.cfg.SessionHeader, req.SessionID)
		}
		for name, value := range p.cfg.Headers {
			httpReq.Header.Set(name, value)
		}
		out, err := p.client.Do(httpReq)
		if err != nil {
			return nil, transportError(ctx, err)
		}
		if out.StatusCode >= http.StatusBadRequest {
			statusErr := statusError(out)
			out.Body.Close()
			return nil, statusErr
		}
		return out, nil
	})
	if err != nil {
		p.logCall(ctx, model, start, err)
		return harness.ChatResponse{}, err
	}
	defer resp.Body.Close()

	out, err := parseStream(ctx, resp.Body, model, sink)
	p.logCall(ctx, model, start, err)
	if err != nil {
		return harness.ChatResponse{}, err
	}
	return out, nil
}

// authorize injeta Authorization: Bearer <credencial> quando há ref.
func (p *Provider) authorize(ctx context.Context, req *http.Request) error {
	ref := strings.TrimSpace(p.cfg.CredentialRef)
	if ref == "" {
		return nil
	}
	if p.deps.Credentials == nil {
		return providerError(codeCredential, fmt.Sprintf("CredentialRef %q sem CredentialProvider injetado", ref), nil)
	}
	key, err := p.deps.Credentials.Resolve(ctx, ref)
	if err != nil {
		return providerError(codeCredential, fmt.Sprintf("resolver credencial %q", ref), err)
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	return nil
}

// now devolve o instante do Clock injetado; zero quando ausente.
func (p *Provider) now() time.Time {
	if p.deps.Clock == nil {
		return time.Time{}
	}
	return p.deps.Clock.Now()
}

// logCall registra a chamada com latência em DEBUG/WARN.
func (p *Provider) logCall(ctx context.Context, model string, start time.Time, err error) {
	if p.deps.Logger == nil {
		return
	}
	attrs := []any{"provider", "openai_responses", "model", model}
	if !start.IsZero() {
		attrs = append(attrs, "duration_ms", p.deps.Clock.Now().Sub(start).Milliseconds())
	}
	if err != nil {
		attrs = append(attrs, "error", err)
		p.deps.Logger.WarnContext(ctx, "openai_responses: chamada de modelo falhou", attrs...)
		return
	}
	p.deps.Logger.DebugContext(ctx, "openai_responses: chamada de modelo concluída", attrs...)
}

// providerError monta o erro canônico do adaptador preservando a causa.
func providerError(code, message string, cause error) error {
	return &harness.ProviderError{Code: code, Message: message, Err: cause}
}

// transportError classifica a falha de transporte (timeout/cancelamento).
func transportError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return contextError(ctxErr)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return providerError(codeTimeout, "timeout na chamada", err)
	}
	return providerError(codeTransport, "falha de transporte", err)
}

// contextError converte ctx.Err() em erro canônico curto.
func contextError(err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return providerError(codeTimeout, "timeout na chamada", err)
	case errors.Is(err, context.Canceled):
		return providerError(codeCanceled, "chamada cancelada", err)
	default:
		return providerError(codeTransport, "falha de transporte", err)
	}
}

// statusError lê no máximo maxErrorBodyBytes do corpo e devolve erro curto.
func statusError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	message := fmt.Sprintf("HTTP %d", resp.StatusCode)
	if detail := providerMessage(body); detail != "" {
		message += ": " + detail
	}
	return providerError(statusCode(resp.StatusCode), message, nil)
}

// statusCode classifica o status HTTP em código estável do adaptador.
func statusCode(status int) string {
	switch {
	case status == http.StatusTooManyRequests:
		return codeRateLimited
	case status == http.StatusRequestTimeout || status == http.StatusGatewayTimeout:
		return codeTimeout
	case status >= http.StatusInternalServerError:
		return codeUnavailable
	default:
		return codeHTTP
	}
}

// providerMessage extrai a mensagem de erro do JSON do provedor e a trunca.
func providerMessage(body []byte) string {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return ""
	}
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(trimmed, &parsed); err == nil {
		if msg := strings.TrimSpace(parsed.Error.Message); msg != "" {
			return truncate(msg, maxErrorMessageRunes)
		}
		if msg := strings.TrimSpace(parsed.Message); msg != "" {
			return truncate(msg, maxErrorMessageRunes)
		}
	}
	return truncate(string(trimmed), maxErrorMessageRunes)
}

// truncate corta a string em max runas, sinalizando o corte.
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
