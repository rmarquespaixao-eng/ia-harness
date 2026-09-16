// Package openai implementa a porta harness.Embedder sobre o endpoint
// {BaseURL}/embeddings de APIs compatíveis com OpenAI (R6/T076): um POST
// {"model", "input": [...]} devolve data[].embedding, reordenado por índice.
// O núcleo não cria índice nem armazenamento (FR-029): o host injeta o
// Embedder e mantém o vector store.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"rmarquespaixao/ia-harness/harness"
)

const (
	// defaultBaseURL é usada quando Config.BaseURL é vazia.
	defaultBaseURL = "https://api.openai.com/v1"
	// defaultModel é usado quando Config.Model é vazio.
	defaultModel = "text-embedding-3-small"
	// maxErrorBodyBytes limita a leitura do corpo de erro do provedor.
	maxErrorBodyBytes = 8 << 10
	// maxErrorMessageRunes limita a mensagem de erro exposta ao host.
	maxErrorMessageRunes = 300
)

// Códigos estáveis de harness.ProviderError devolvidos pelo adaptador
// (mesma taxonomia do adaptador de chat para consistência do host).
const (
	codeRequest     = "provider_request"
	codeCredential  = "provider_credential"
	codeTransport   = "provider_transport"
	codeTimeout     = "provider_timeout"
	codeCanceled    = "provider_canceled"
	codeHTTP        = "provider_http"
	codeRateLimited = "provider_rate_limited"
	codeUnavailable = "provider_unavailable"
)

// Config descreve o endpoint de embeddings e os padrões do adaptador.
type Config struct {
	// BaseURL é a raiz da API compatível (ex.: "https://api.openai.com/v1").
	BaseURL string
	// CredentialRef é resolvida por request via Deps.Credentials; vazia
	// dispensa Authorization (servidores locais sem autenticação).
	CredentialRef string
	// Model é o modelo de embeddings; vazio usa o default do adaptador.
	Model string
	// Timeout é aplicado ao cliente HTTP (cria um quando Deps.HTTPClient é nil).
	Timeout time.Duration
}

// Deps reúne as dependências injetadas pelo host (D-12).
type Deps struct {
	// Credentials resolve Config.CredentialRef a cada chamada; nenhuma
	// credencial é lida em New (FR-004/constitution §4).
	Credentials harness.CredentialProvider
	// HTTPClient é o cliente do host; nil cria um com Config.Timeout.
	HTTPClient *http.Client
}

// Embedder é o adaptador OpenAI-compatible de embeddings.
type Embedder struct {
	cfg    Config
	deps   Deps
	client *http.Client
}

// Embedder implementa harness.Embedder.
var _ harness.Embedder = (*Embedder)(nil)

// New guarda configuração e dependências; não abre rede nem resolve credencial.
func New(cfg Config, deps Deps) *Embedder {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.Model == "" {
		cfg.Model = defaultModel
	}
	return &Embedder{
		cfg:    cfg,
		deps:   deps,
		client: newHTTPClient(cfg.Timeout, deps.HTTPClient),
	}
}

// Embed envia os textos em uma única chamada e devolve os vetores na ordem da
// entrada, reordenando data[] pelo índice reportado.
func (e *Embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, providerError(codeRequest, "Embed exige ao menos um texto", nil)
	}

	body, err := json.Marshal(embeddingRequest{Model: e.cfg.Model, Input: texts})
	if err != nil {
		return nil, providerError(codeRequest, "montar request de embeddings", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.BaseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, providerError(codeRequest, "criar request HTTP", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if err := e.authorize(ctx, httpReq); err != nil {
		return nil, err
	}

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return nil, transportError(ctx, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, statusError(resp)
	}
	return decodeEmbeddings(resp.Body, len(texts))
}

// embeddingRequest é o corpo do POST /embeddings.
type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// embeddingResponse é o subconjunto da resposta usado pelo adaptador.
type embeddingResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// decodeEmbeddings valida o corpo e devolve os vetores na ordem da entrada.
func decodeEmbeddings(body io.Reader, want int) ([][]float32, error) {
	var parsed embeddingResponse
	if err := json.NewDecoder(body).Decode(&parsed); err != nil {
		return nil, providerError(codeRequest, "decodificar resposta de embeddings", err)
	}
	if len(parsed.Data) != want {
		return nil, providerError(codeRequest, fmt.Sprintf("resposta com %d embeddings (esperado %d)", len(parsed.Data), want), nil)
	}
	out := make([][]float32, want)
	for _, item := range parsed.Data {
		if item.Index < 0 || item.Index >= want {
			return nil, providerError(codeRequest, fmt.Sprintf("índice de embedding fora da faixa: %d", item.Index), nil)
		}
		if out[item.Index] != nil {
			return nil, providerError(codeRequest, fmt.Sprintf("índice de embedding duplicado: %d", item.Index), nil)
		}
		out[item.Index] = item.Embedding
	}
	return out, nil
}

// authorize injeta Authorization: Bearer <credencial> quando há ref; ref vazia
// dispensa autenticação.
func (e *Embedder) authorize(ctx context.Context, req *http.Request) error {
	ref := strings.TrimSpace(e.cfg.CredentialRef)
	if ref == "" {
		return nil
	}
	if e.deps.Credentials == nil {
		return providerError(codeCredential, fmt.Sprintf("CredentialRef %q sem CredentialProvider injetado", ref), nil)
	}
	key, err := e.deps.Credentials.Resolve(ctx, ref)
	if err != nil {
		return providerError(codeCredential, fmt.Sprintf("resolver credencial %q", ref), err)
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	return nil
}

// newHTTPClient aplica Config.Timeout ao cliente injetado (cópia) ou cria um
// cliente novo quando Deps.HTTPClient é nil.
func newHTTPClient(timeout time.Duration, base *http.Client) *http.Client {
	if base == nil {
		return &http.Client{Timeout: timeout}
	}
	if timeout <= 0 {
		return base
	}
	clone := *base
	clone.Timeout = timeout
	return &clone
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
