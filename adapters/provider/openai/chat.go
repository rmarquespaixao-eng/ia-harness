// Package openai implementa a porta harness.Provider sobre APIs compatíveis
// com OpenAI Chat Completions (OpenAI, OpenRouter, Groq, DeepSeek, Ollama,
// vLLM, LM Studio): mapeamento canônico de request/response (T040) e
// streaming SSE com tool calls fragmentadas e usage opcional (T041, R3/R4).
package openai

import (
	"bytes"
	"context"
	"encoding/base64"
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
	// defaultBaseURL é usada quando Config.BaseURL é vazia.
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

// Config descreve o endpoint e os padrões do adaptador.
type Config struct {
	// BaseURL é a raiz da API compatível (ex.: "https://openrouter.ai/api/v1").
	BaseURL string
	// CredentialRef é resolvida por request via Deps.Credentials; vazia
	// dispensa Authorization (servidores locais sem autenticação).
	CredentialRef string
	// DefaultModel é usado quando ChatRequest.Model é vazio.
	DefaultModel string
	// Headers são headers fixos acrescentados a cada request.
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
	// Credentials resolve Config.CredentialRef a cada chamada; nenhuma
	// credencial é lida em New (FR-004/constitution §4).
	Credentials harness.CredentialProvider
	// HTTPClient é o cliente do host; nil cria um com Config.Timeout.
	HTTPClient *http.Client
	// Clock mede a latência registrada no log estruturado.
	Clock harness.Clock
	// Logger registra as chamadas em DEBUG/WARN; nil desativa.
	Logger *slog.Logger
}

// Provider é o adaptador OpenAI-compatible.
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
// chamada, nunca aqui (T043).
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
// cliente novo quando Deps.HTTPClient é nil.
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

// ChatStream faz a chamada em streaming para {BaseURL}/chat/completions e
// devolve a resposta canônica; texto, raciocínio e fragmentos de argumentos de
// tool chegam ao sink (feature 012). A credencial é resolvida a cada request.
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
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
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

// authorize injeta Authorization: Bearer <credencial> quando há ref; ref
// vazia dispensa autenticação.
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

// logCall registra a chamada com latência em DEBUG/WARN (R13).
func (p *Provider) logCall(ctx context.Context, model string, start time.Time, err error) {
	if p.deps.Logger == nil {
		return
	}
	attrs := []any{"provider", "openai", "model", model}
	if !start.IsZero() {
		attrs = append(attrs, "duration_ms", p.deps.Clock.Now().Sub(start).Milliseconds())
	}
	if err != nil {
		attrs = append(attrs, "error", err)
		p.deps.Logger.WarnContext(ctx, "openai: chamada de modelo falhou", attrs...)
		return
	}
	p.deps.Logger.DebugContext(ctx, "openai: chamada de modelo concluída", attrs...)
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

// statusError lê no máximo maxErrorBodyBytes do corpo e devolve um erro curto
// com o status; a mensagem do provedor entra truncada e nenhum header vaza.
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

// buildBody monta o corpo do request: Params do perfil entram primeiro e os
// campos do protocolo prevalecem (model, messages, stream, tools).
func buildBody(model string, req harness.ChatRequest) ([]byte, error) {
	body := make(map[string]any, len(req.Params)+5)
	for key, value := range req.Params {
		body[key] = value
	}
	messages, err := wireMessages(req)
	if err != nil {
		return nil, err
	}
	body["model"] = model
	body["messages"] = messages
	body["stream"] = true
	body["stream_options"] = map[string]any{"include_usage": true}
	if req.MaxOutputTokens > 0 {
		body["max_tokens"] = req.MaxOutputTokens
	}
	if len(req.Tools) > 0 {
		tools, err := wireTools(req.Tools)
		if err != nil {
			return nil, err
		}
		body["tools"] = tools
	}
	if len(req.OutputSchema) > 0 {
		// Structured output (feature 009).
		var schema any
		if err := json.Unmarshal(req.OutputSchema, &schema); err != nil {
			return nil, fmt.Errorf("schema de saída: %w", err)
		}
		body["response_format"] = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "output",
				"schema": schema,
				"strict": true,
			},
		}
	}
	return json.Marshal(body)
}

// wireMessage é a mensagem no formato Chat Completions.
type wireMessage struct {
	Role       string         `json:"role"`
	Content    any            `json:"content,omitempty"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

// wireToolCall é a tool call do histórico do assistente.
type wireToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function wireFunctionCall `json:"function"`
}

// wireFunctionCall carrega nome e argumentos serializados da função.
type wireFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// wireTool publica a tool no formato function calling.
type wireTool struct {
	Type     string           `json:"type"`
	Function wireToolFunction `json:"function"`
}

// wireToolFunction descreve a função publicada.
type wireToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters"`
}

// wireMessages converte o histórico canônico nas mensagens do protocolo.
func wireMessages(req harness.ChatRequest) ([]wireMessage, error) {
	msgs := make([]wireMessage, 0, len(req.Messages)+1)
	if system := strings.TrimSpace(req.System); system != "" {
		msgs = append(msgs, wireMessage{Role: "system", Content: system})
	}
	for _, m := range req.Messages {
		switch m.Role {
		case harness.RoleSystem:
			if text := messageText(m); text != "" {
				msgs = append(msgs, wireMessage{Role: "system", Content: text})
			}
		case harness.RoleUser:
			_, results := userParts(m)
			if content := userContent(m); content != nil {
				msgs = append(msgs, wireMessage{Role: "user", Content: content})
			}
			msgs = append(msgs, toolMessages(results)...)
		case harness.RoleAssistant:
			msg := wireMessage{Role: "assistant", Content: messageText(m)}
			for _, part := range m.Parts {
				if part.Kind != harness.PartToolCall || part.Call == nil {
					continue
				}
				args := part.Call.Args
				if len(bytes.TrimSpace(args)) == 0 {
					args = json.RawMessage("{}")
				}
				msg.ToolCalls = append(msg.ToolCalls, wireToolCall{
					ID:       part.Call.ID,
					Type:     "function",
					Function: wireFunctionCall{Name: part.Call.Name, Arguments: string(args)},
				})
			}
			msgs = append(msgs, msg)
		case harness.RoleTool:
			msgs = append(msgs, toolMessages(messageResults(m))...)
		}
	}
	return msgs, nil
}

// messageText concatena as partes de texto de uma mensagem.
func messageText(m harness.Message) string {
	var text strings.Builder
	for _, part := range m.Parts {
		if part.Kind == harness.PartText {
			text.WriteString(part.Text)
		}
	}
	return text.String()
}

// userContent monta o content do usuário: string quando só há texto; array de
// partes quando há mídia (feature 002/FR-MM-006). nil quando não há conteúdo.
func userContent(m harness.Message) any {
	var text strings.Builder
	var media []any
	for _, part := range m.Parts {
		switch part.Kind {
		case harness.PartText:
			text.WriteString(part.Text)
		case harness.PartImage:
			if part.Media != nil {
				media = append(media, map[string]any{
					"type":      "image_url",
					"image_url": map[string]any{"url": mediaURL(part.Media)},
				})
			}
		case harness.PartDocument:
			if part.Media != nil {
				media = append(media, documentContent(part.Media))
			}
		}
	}
	if len(media) == 0 {
		if text.Len() == 0 {
			return nil
		}
		return text.String()
	}
	parts := make([]any, 0, len(media)+1)
	if text.Len() > 0 {
		parts = append(parts, map[string]any{"type": "text", "text": text.String()})
	}
	parts = append(parts, media...)
	return parts
}

// mediaURL devolve a referência (URL) ou um data URI base64 da mídia.
func mediaURL(m *harness.Media) string {
	if m.Reference != "" {
		return m.Reference
	}
	return dataURI(m)
}

// dataURI monta data:<mime>;base64,<...>.
func dataURI(m *harness.Media) string {
	return "data:" + m.MIME + ";base64," + base64.StdEncoding.EncodeToString(m.Bytes)
}

// documentContent mapeia documento: textual vira texto inline; binário vira o
// bloco `file` do Chat Completions (FR-MM-007).
func documentContent(m *harness.Media) map[string]any {
	if strings.HasPrefix(strings.ToLower(m.MIME), "text/") {
		text := m.Reference
		if len(m.Bytes) > 0 {
			text = string(m.Bytes)
		}
		return map[string]any{"type": "text", "text": text}
	}
	data := m.Reference
	if len(m.Bytes) > 0 {
		data = dataURI(m)
	}
	return map[string]any{
		"type": "file",
		"file": map[string]any{"file_data": data, "filename": m.Name},
	}
}

// userParts separa o texto dos resultados de tool de uma mensagem de usuário.
func userParts(m harness.Message) ([]string, []*harness.ToolResult) {
	var text []string
	var results []*harness.ToolResult
	for _, part := range m.Parts {
		switch part.Kind {
		case harness.PartText:
			text = append(text, part.Text)
		case harness.PartToolResult:
			if part.Result != nil {
				results = append(results, part.Result)
			}
		}
	}
	return text, results
}

// messageResults extrai os resultados de tool de uma mensagem de tool.
func messageResults(m harness.Message) []*harness.ToolResult {
	var results []*harness.ToolResult
	for _, part := range m.Parts {
		if part.Kind == harness.PartToolResult && part.Result != nil {
			results = append(results, part.Result)
		}
	}
	return results
}

// toolMessages converte resultados de tool em mensagens role "tool".
func toolMessages(results []*harness.ToolResult) []wireMessage {
	msgs := make([]wireMessage, 0, len(results))
	for _, result := range results {
		msgs = append(msgs, wireMessage{
			Role:       "tool",
			Content:    toolResultText(result),
			ToolCallID: result.CallID,
		})
	}
	return msgs
}

// toolResultText serializa o resultado para o content; erro/negativa sem
// conteúdo recebem texto estável para o modelo não ver vazio.
func toolResultText(result *harness.ToolResult) string {
	parts := make([]string, 0, len(result.Content))
	for _, content := range result.Content {
		switch content.Kind {
		case harness.ResultText:
			if content.Text != "" {
				parts = append(parts, content.Text)
			}
		case harness.ResultJSON:
			if len(content.JSON) > 0 {
				parts = append(parts, string(content.JSON))
			}
		}
	}
	if len(parts) == 0 {
		switch {
		case result.Denied:
			return "chamada negada pela política"
		case result.IsError:
			return "erro na execução da tool"
		}
	}
	return strings.Join(parts, "\n")
}

// wireTools publica as tools no formato function calling.
func wireTools(tools []harness.Tool) ([]wireTool, error) {
	out := make([]wireTool, 0, len(tools))
	for _, tool := range tools {
		params, err := decodeSchema(tool.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("schema da tool %q: %w", tool.Name, err)
		}
		out = append(out, wireTool{
			Type: "function",
			Function: wireToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  params,
			},
		})
	}
	return out, nil
}

// decodeSchema decodifica o InputSchema para um objeto JSON; schema ausente
// vira o objeto vazio aceito pelos provedores.
func decodeSchema(raw json.RawMessage) (any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return emptySchema(), nil
	}
	var schema any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, err
	}
	if schema == nil {
		return emptySchema(), nil
	}
	return schema, nil
}

// emptySchema devolve o schema objeto vazio.
func emptySchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
