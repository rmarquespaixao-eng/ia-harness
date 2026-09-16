// Package anthropic implementa a porta harness.Provider sobre a Messages API
// nativa (https://api.anthropic.com/v1/messages): system top-level, content
// blocks (text/tool_use/tool_result), streaming SSE com content_block_* e
// input_json_delta, e usage reportado em message_start/message_delta (T042,
// research R3/R4).
package anthropic

import (
	"bufio"
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
	defaultBaseURL = "https://api.anthropic.com"
	// defaultVersion é a versão da Messages API usada quando Config.Version é vazia.
	defaultVersion = "2023-06-01"
	// defaultMaxTokens é o teto de saída usado quando Config.MaxTokens é zero
	// (a Messages API exige max_tokens).
	defaultMaxTokens = 4096
	// maxErrorBodyBytes limita a leitura do corpo de erro do provedor.
	maxErrorBodyBytes = 8 << 10
	// maxErrorMessageRunes limita a mensagem de erro exposta ao host.
	maxErrorMessageRunes = 300
	// maxSSELineBytes limita uma linha do SSE (tool calls grandes cabem).
	maxSSELineBytes = 4 << 20
	// maxSSESnippetRunes limita o trecho do evento inválido no erro.
	maxSSESnippetRunes = 120
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

// Prefixos do SSE da Messages API.
const (
	sseEventPrefix = "event:"
	sseDataPrefix  = "data:"
)

// Config descreve o endpoint e os padrões do adaptador.
type Config struct {
	// BaseURL é a raiz da API (default https://api.anthropic.com).
	BaseURL string
	// CredentialRef é resolvida por request via Deps.Credentials; vazia
	// dispensa x-api-key (proxies autenticados fora do adaptador).
	CredentialRef string
	// DefaultModel é usado quando ChatRequest.Model é vazio.
	DefaultModel string
	// Version é o valor de anthropic-version (default 2023-06-01).
	Version string
	// MaxTokens é o teto de saída default quando o request não pede outro
	// (default 4096).
	MaxTokens int
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

// Provider é o adaptador da Messages API.
type Provider struct {
	cfg    Config
	deps   Deps
	client *http.Client

	mu     sync.Mutex
	closed bool
}

// Provider implementa harness.Provider.
var _ harness.Provider = (*Provider)(nil)

// New aplica os defaults e guarda configuração e dependências; a credencial
// só é resolvida na chamada, nunca aqui (T043).
func New(cfg Config, deps Deps) *Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if cfg.Version == "" {
		cfg.Version = defaultVersion
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = defaultMaxTokens
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

// ChatStream faz a chamada em streaming para {BaseURL}/v1/messages e devolve a
// resposta canônica; texto, raciocínio e fragmentos de argumentos de tool
// chegam ao sink (feature 012). A credencial é resolvida a cada request.
func (p *Provider) ChatStream(ctx context.Context, req harness.ChatRequest, sink harness.StreamSink) (harness.ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.DefaultModel
	}

	body, err := p.buildBody(model, req)
	if err != nil {
		return harness.ChatResponse{}, providerError(codeRequest, fmt.Sprintf("montar request de %q", model), err)
	}

	start := p.now()
	resp, err := providerhttp.Do(ctx, p.retryPolicy(), func(ctx context.Context) (*http.Response, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL+"/v1/messages", bytes.NewReader(body))
		if err != nil {
			return nil, providerError(codeRequest, "criar request HTTP", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "text/event-stream")
		httpReq.Header.Set("anthropic-version", p.cfg.Version)
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

// authorize injeta x-api-key quando há ref; ref vazia dispensa a chave.
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
		req.Header.Set("x-api-key", key)
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
	attrs := []any{"provider", "anthropic", "model", model}
	if !start.IsZero() {
		attrs = append(attrs, "duration_ms", p.deps.Clock.Now().Sub(start).Milliseconds())
	}
	if err != nil {
		attrs = append(attrs, "error", err)
		p.deps.Logger.WarnContext(ctx, "anthropic: chamada de modelo falhou", attrs...)
		return
	}
	p.deps.Logger.DebugContext(ctx, "anthropic: chamada de modelo concluída", attrs...)
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
// campos do protocolo prevalecem (model, system, messages, tools, max_tokens,
// stream). A Messages API exige max_tokens em toda chamada.
func (p *Provider) buildBody(model string, req harness.ChatRequest) ([]byte, error) {
	body := make(map[string]any, len(req.Params)+6)
	for key, value := range req.Params {
		body[key] = value
	}
	if system := systemText(req); system != "" {
		body["system"] = systemBlocks(system, req.PromptCaching)
	}
	body["model"] = model
	body["messages"] = wireMessages(req)
	body["max_tokens"] = p.maxTokens(req)
	body["stream"] = true
	if len(req.Tools) > 0 {
		tools, err := wireTools(req.Tools, req.PromptCaching)
		if err != nil {
			return nil, err
		}
		body["tools"] = tools
	}
	return json.Marshal(body)
}

// systemBlocks devolve o system como string (sem cache) ou como bloco de texto
// com cache_control ephemeral (feature 006).
func systemBlocks(system string, cache bool) any {
	if !cache {
		return system
	}
	return []wireBlock{{Type: "text", Text: system, CacheControl: &wireCacheControl{Type: "ephemeral"}}}
}

// maxTokens devolve o teto de saída do request ou o default da configuração.
func (p *Provider) maxTokens(req harness.ChatRequest) int {
	if req.MaxOutputTokens > 0 {
		return req.MaxOutputTokens
	}
	return p.cfg.MaxTokens
}

// systemText concatena o system do request e das mensagens RoleSystem.
func systemText(req harness.ChatRequest) string {
	var parts []string
	if system := strings.TrimSpace(req.System); system != "" {
		parts = append(parts, system)
	}
	for _, m := range req.Messages {
		if m.Role != harness.RoleSystem {
			continue
		}
		var text strings.Builder
		for _, part := range m.Parts {
			if part.Kind == harness.PartText {
				text.WriteString(part.Text)
			}
		}
		if text.Len() > 0 {
			parts = append(parts, text.String())
		}
	}
	return strings.Join(parts, "\n\n")
}

// wireMessage é uma mensagem no formato da Messages API.
type wireMessage struct {
	Role    string      `json:"role"`
	Content []wireBlock `json:"content"`
}

// wireBlock é um content block; os campos usados variam pelo Type.
type wireBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// tool_use
	ID           string            `json:"id,omitempty"`
	Name         string            `json:"name,omitempty"`
	Input        any               `json:"input,omitempty"`
	Source       *wireSource       `json:"source,omitempty"`
	CacheControl *wireCacheControl `json:"cache_control,omitempty"`
	// tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

// wireCacheControl marca um breakpoint de prompt caching (feature 006).
type wireCacheControl struct {
	Type string `json:"type"`
}

// wireSource é a fonte de um bloco image/document (base64, url ou text).
type wireSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

// wireTool publica a tool com o input_schema dinâmico.
type wireTool struct {
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	InputSchema  any               `json:"input_schema"`
	CacheControl *wireCacheControl `json:"cache_control,omitempty"`
}

// wireMessages converte o histórico canônico e funde mensagens consecutivas
// do mesmo papel (a API espera papéis alternados; tool results viram blocos de
// user).
func wireMessages(req harness.ChatRequest) []wireMessage {
	var msgs []wireMessage
	appendBlocks := func(role string, blocks []wireBlock) {
		if len(blocks) == 0 {
			return
		}
		if n := len(msgs); n > 0 && msgs[n-1].Role == role {
			msgs[n-1].Content = append(msgs[n-1].Content, blocks...)
			return
		}
		msgs = append(msgs, wireMessage{Role: role, Content: blocks})
	}
	for _, m := range req.Messages {
		switch m.Role {
		case harness.RoleSystem:
			// system é top-level; não entra na lista de mensagens.
		case harness.RoleUser:
			appendBlocks("user", userBlocks(m))
		case harness.RoleAssistant:
			appendBlocks("assistant", assistantBlocks(m))
		case harness.RoleTool:
			appendBlocks("user", resultBlocks(messageResults(m)))
		}
	}
	return msgs
}

// userBlocks converte texto, mídia (imagem/documento) e resultados de tool de
// uma mensagem de usuário (feature 002/FR-MM-006).
func userBlocks(m harness.Message) []wireBlock {
	blocks := make([]wireBlock, 0, len(m.Parts))
	for _, part := range m.Parts {
		switch part.Kind {
		case harness.PartText:
			if part.Text != "" {
				blocks = append(blocks, wireBlock{Type: "text", Text: part.Text})
			}
		case harness.PartImage:
			if part.Media != nil {
				blocks = append(blocks, wireBlock{Type: "image", Source: mediaSource(part.Media)})
			}
		case harness.PartDocument:
			if part.Media != nil {
				blocks = append(blocks, documentBlock(part.Media))
			}
		case harness.PartToolResult:
			if part.Result != nil {
				blocks = append(blocks, resultBlock(part.Result))
			}
		}
	}
	return blocks
}

// mediaSource monta a fonte de uma imagem: base64 inline ou URL.
func mediaSource(m *harness.Media) *wireSource {
	if m.Reference != "" {
		return &wireSource{Type: "url", URL: m.Reference}
	}
	return &wireSource{Type: "base64", MediaType: m.MIME, Data: base64.StdEncoding.EncodeToString(m.Bytes)}
}

// documentBlock mapeia documento: textual vira bloco text; binário (PDF) vira
// bloco document nativo (FR-MM-006/007).
func documentBlock(m *harness.Media) wireBlock {
	if strings.HasPrefix(strings.ToLower(m.MIME), "text/") {
		text := m.Reference
		if len(m.Bytes) > 0 {
			text = string(m.Bytes)
		}
		return wireBlock{Type: "text", Text: text}
	}
	return wireBlock{Type: "document", Source: mediaSource(m)}
}

// assistantBlocks converte texto e tool calls de uma mensagem do assistente.
func assistantBlocks(m harness.Message) []wireBlock {
	blocks := make([]wireBlock, 0, len(m.Parts))
	for _, part := range m.Parts {
		switch part.Kind {
		case harness.PartText:
			if part.Text != "" {
				blocks = append(blocks, wireBlock{Type: "text", Text: part.Text})
			}
		case harness.PartToolCall:
			if part.Call == nil {
				continue
			}
			blocks = append(blocks, wireBlock{
				Type:  "tool_use",
				ID:    part.Call.ID,
				Name:  part.Call.Name,
				Input: decodeInput(part.Call.Args),
			})
		}
	}
	return blocks
}

// resultBlocks converte resultados de tool em blocos tool_result.
func resultBlocks(results []*harness.ToolResult) []wireBlock {
	blocks := make([]wireBlock, 0, len(results))
	for _, result := range results {
		blocks = append(blocks, resultBlock(result))
	}
	return blocks
}

// resultBlock converte um resultado de tool em bloco tool_result.
func resultBlock(result *harness.ToolResult) wireBlock {
	return wireBlock{
		Type:      "tool_result",
		ToolUseID: result.CallID,
		Content:   toolResultText(result),
		IsError:   result.IsError || result.Denied,
	}
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

// wireTools publica as tools com o input_schema decodificado; com cache ligado,
// marca o último tool como breakpoint (cacheia system+tools — feature 006).
func wireTools(tools []harness.Tool, cache bool) ([]wireTool, error) {
	out := make([]wireTool, 0, len(tools))
	for _, tool := range tools {
		schema, err := decodeSchema(tool.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("schema da tool %q: %w", tool.Name, err)
		}
		out = append(out, wireTool{Name: tool.Name, Description: tool.Description, InputSchema: schema})
	}
	if cache && len(out) > 0 {
		out[len(out)-1].CacheControl = &wireCacheControl{Type: "ephemeral"}
	}
	return out, nil
}

// decodeSchema decodifica o InputSchema para um objeto JSON; schema ausente
// vira o objeto vazio aceito pela API.
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

// decodeInput converte os argumentos da tool call para o input da API;
// fragmento inválido vira objeto vazio (a validação é do núcleo) para não
// derrubar a chamada seguinte que reenvia o histórico.
func decodeInput(raw json.RawMessage) any {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return emptySchema()
	}
	var input any
	if err := json.Unmarshal(trimmed, &input); err != nil {
		return emptySchema()
	}
	if _, ok := input.(map[string]any); !ok {
		return emptySchema()
	}
	return input
}

// emptySchema devolve o schema/input objeto vazio.
func emptySchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

// streamEvent é um evento do SSE da Messages API.
type streamEvent struct {
	Type    string `json:"type"`
	Message *struct {
		Usage *streamUsage `json:"usage"`
	} `json:"message"`
	Index   int `json:"index"`
	Content *struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"content_block"`
	Delta *struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Thinking    string `json:"thinking"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
	Usage *streamUsage `json:"usage"`
}

// streamUsage é o usage reportado em message_start/message_delta.
type streamUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

// blockAccum acumula um content block por índice.
type blockAccum struct {
	kind string
	id   string
	name string
	json strings.Builder
}

// streamState acumula o stream até montar a resposta canônica.
type streamState struct {
	text         strings.Builder
	blocks       []*blockAccum
	stopReason   string
	inputTokens  int64
	outputTokens int64
	cacheRead    int64
	cacheWrite   int64
	hasUsage     bool
	done         bool
}

// parseStream consome o SSE da Messages API e monta a resposta canônica; EOF
// antes de message_stop é stream cortado (T042, R4). O sink recebe texto,
// raciocínio (thinking) e fragmentos de argumentos (feature 012).
func parseStream(ctx context.Context, body io.Reader, model string, sink harness.StreamSink) (harness.ChatResponse, error) {
	state := &streamState{}
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxSSELineBytes)
	eventName := ""

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return harness.ChatResponse{}, contextError(err)
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			eventName = ""
			continue
		}
		switch {
		case strings.HasPrefix(line, sseEventPrefix):
			eventName = strings.TrimSpace(strings.TrimPrefix(line, sseEventPrefix))
		case strings.HasPrefix(line, sseDataPrefix):
			payload := strings.TrimSpace(strings.TrimPrefix(line, sseDataPrefix))
			if payload == "" {
				continue
			}
			var event streamEvent
			if err := json.Unmarshal([]byte(payload), &event); err != nil {
				return harness.ChatResponse{}, providerError(codeStream,
					fmt.Sprintf("evento SSE inválido (%s)", truncate(payload, maxSSESnippetRunes)), err)
			}
			if event.Type == "" {
				event.Type = eventName
			}
			state.consume(event, sink)
		default:
			// Comentários e campos desconhecidos (id:, retry:) são ignorados.
		}
	}
	if err := scanner.Err(); err != nil {
		return harness.ChatResponse{}, transportError(ctx, err)
	}
	if !state.done {
		message := "stream encerrado sem message_stop"
		if state.stopReason != "" {
			message += " (stop_reason=" + state.stopReason + ")"
		}
		return harness.ChatResponse{}, providerError(codeStream, message, io.ErrUnexpectedEOF)
	}
	return state.response(model), nil
}

// consume aplica um evento ao estado: text_delta vai para o sink e
// input_json_delta é acumulado por bloco (emitido como ToolCallArgs);
// thinking_delta vira Reasoning; ping e eventos desconhecidos são ignorados
// (forward-compatible — feature 012).
func (s *streamState) consume(event streamEvent, sink harness.StreamSink) {
	switch event.Type {
	case "message_start":
		if event.Message != nil && event.Message.Usage != nil {
			s.inputTokens = event.Message.Usage.InputTokens
			s.cacheRead = event.Message.Usage.CacheReadInputTokens
			s.cacheWrite = event.Message.Usage.CacheCreationInputTokens
			s.hasUsage = true
		}
	case "content_block_start":
		acc := s.at(event.Index)
		if event.Content != nil {
			acc.kind = event.Content.Type
			acc.id = event.Content.ID
			acc.name = event.Content.Name
		}
	case "content_block_delta":
		if event.Delta == nil {
			return
		}
		switch event.Delta.Type {
		case "text_delta":
			if event.Delta.Text != "" {
				s.text.WriteString(event.Delta.Text)
				if sink != nil {
					sink.Text(event.Delta.Text)
				}
			}
		case "thinking_delta":
			if event.Delta.Thinking != "" && sink != nil {
				sink.Reasoning(event.Delta.Thinking)
			}
		case "input_json_delta":
			acc := s.at(event.Index)
			acc.json.WriteString(event.Delta.PartialJSON)
			if sink != nil && event.Delta.PartialJSON != "" {
				sink.ToolCallArgs(acc.id, acc.name, event.Delta.PartialJSON)
			}
		}
	case "content_block_stop":
		// O acumulado do bloco já está pronto.
	case "message_delta":
		if event.Delta != nil && event.Delta.StopReason != "" {
			s.stopReason = event.Delta.StopReason
		}
		if event.Usage != nil {
			s.outputTokens = event.Usage.OutputTokens
			if event.Usage.InputTokens > 0 {
				s.inputTokens = event.Usage.InputTokens
			}
			s.hasUsage = true
		}
	case "message_stop":
		s.done = true
	default:
		// ping, error e eventos futuros: ignorados.
	}
}

// at devolve (criando) o acumulador do índice, preservando a ordem dos blocos.
func (s *streamState) at(index int) *blockAccum {
	for len(s.blocks) <= index {
		s.blocks = append(s.blocks, &blockAccum{})
	}
	return s.blocks[index]
}

// response monta a resposta canônica: texto + tool_use (fragmento de
// argumento inválido é devolvido como veio; a validação é do núcleo) e usage
// reportado ou marcado como estimado (R5).
func (s *streamState) response(model string) harness.ChatResponse {
	out := harness.ChatResponse{
		Message:    harness.Message{Role: harness.RoleAssistant},
		StopReason: mapStopReason(s.stopReason),
		Model:      model,
	}
	if text := s.text.String(); text != "" {
		out.Message.Parts = append(out.Message.Parts, harness.Part{Kind: harness.PartText, Text: text})
	}
	for _, acc := range s.blocks {
		if acc == nil || acc.kind != "tool_use" {
			continue
		}
		args := strings.TrimSpace(acc.json.String())
		if args == "" {
			args = "{}"
		}
		out.ToolCalls = append(out.ToolCalls, harness.ToolCall{ID: acc.id, Name: acc.name, Args: json.RawMessage(args)})
	}
	for i := range out.ToolCalls {
		out.Message.Parts = append(out.Message.Parts, harness.Part{Kind: harness.PartToolCall, Call: &out.ToolCalls[i]})
	}
	if s.hasUsage {
		out.Usage = harness.Usage{
			// InputTokens é o total (não cacheado + leitura + escrita de cache).
			InputTokens:       s.inputTokens + s.cacheRead + s.cacheWrite,
			OutputTokens:      s.outputTokens,
			CachedInputTokens: s.cacheRead,
			CacheWriteTokens:  s.cacheWrite,
			Estimated:         false,
		}
	} else {
		out.Usage = harness.Usage{Estimated: true}
	}
	return out
}

// mapStopReason traduz stop_reason nativo (end_turn, tool_use, max_tokens,
// stop_sequence) para o canônico: o adaptador sempre conclui o turno do
// provedor — tool_use carrega as ToolCalls e max_tokens é truncamento do
// provedor, não estouro de orçamento do núcleo.
func mapStopReason(string) harness.StopReason { return harness.StopCompleted }
