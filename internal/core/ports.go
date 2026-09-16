package core

import (
	"context"
	"encoding/json"
	"time"

	gen "github.com/rmarquespaixao-eng/ia-harness/contracts/gen"
)

// AuditEvent é o evento da trilha de auditoria (contrato JSON Schema gerado).
type AuditEvent = gen.AuditEvent

// Provider é um adaptador de modelo (OpenAI-compatible, Anthropic, …) injetado por DI.
type Provider interface {
	// Chat faz uma chamada de modelo com streaming de texto opcional.
	Chat(ctx context.Context, req ChatRequest, onText func(string)) (ChatResponse, error)
	Close() error
}

// ToolSource é um cliente MCP: catálogo e execução de tools.
type ToolSource interface {
	List(ctx context.Context) ([]Tool, error)
	Call(ctx context.Context, name string, args json.RawMessage, onProgress func(ProgressUpdate)) (ToolResult, error)
	Close() error
}

// SessionStore persiste o snapshot da sessão (host); o núcleo fornece impl. em memória.
type SessionStore interface {
	Load(ctx context.Context, sessionID string) (*Session, error)
	Save(ctx context.Context, s *Session) error
}

// AuditSink recebe um evento por chamada de modelo e por execução/negativa de tool.
type AuditSink interface {
	Emit(ctx context.Context, ev AuditEvent) error
}

// MemoryStore guarda fatos duráveis por usuário.
type MemoryStore interface {
	Put(ctx context.Context, f Fact) error
	List(ctx context.Context, userID string) ([]Fact, error)
	Delete(ctx context.Context, userID, factID string) error
}

// Retriever busca trechos relevantes no índice do host.
type Retriever interface {
	Retrieve(ctx context.Context, userID, query string, k int) ([]RetrievedItem, error)
}

// Embedder gera embeddings de texto (opcional).
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// CredentialProvider resolve referências de credencial ("env:VAR", "file:/path").
type CredentialProvider interface {
	Resolve(ctx context.Context, ref string) (string, error)
}

// Summarizer resume mensagens para a janela de contexto (opcional).
type Summarizer interface {
	Summarize(ctx context.Context, msgs []Message, maxTokens int) (string, error)
}

// ModelSummarizer é a extensão opcional de Summarizer que recebe o alias do
// modelo do turno (feature 018), permitindo ao host resumir com o mesmo modelo
// do perfil. Quando implementada, tem precedência sobre Summarize.
type ModelSummarizer interface {
	SummarizeForModel(ctx context.Context, model string, msgs []Message, maxTokens int) (string, error)
}

// Clock devolve o instante atual; time.Now() é proibido fora da impl. de sistema.
type Clock interface {
	Now() time.Time
}

// Tokenizer conta tokens de um texto (feature 010). Opcional: quando o host não
// injeta um, o núcleo usa a heurística de bytes/token da Pricing.
type Tokenizer interface {
	Count(text string) int
}

// Span é uma unidade observável (turno, chamada de modelo, execução de tool);
// End encerra o span com o erro (nil = sucesso). O host liga ao OTel GenAI.
type Span interface {
	End(err error)
}

// Tracer cria spans no padrão GenAI (feature 011). Opcional: default no-op.
type Tracer interface {
	StartTurn(ctx context.Context, attrs TurnAttrs) (context.Context, Span)
	StartModel(ctx context.Context, attrs ModelAttrs) (context.Context, Span)
	StartTool(ctx context.Context, attrs ToolAttrs) (context.Context, Span)
}

// TurnAttrs identifica o turno no span de agente.
type TurnAttrs struct {
	SessionID string
	UserID    string
	AgentID   string
	Model     string
}

// ModelAttrs identifica a chamada de modelo no span de inferência.
type ModelAttrs struct {
	Provider string
	Model    string
}

// ToolAttrs identifica a execução de tool no span de tool.
type ToolAttrs struct {
	Tool   string
	CallID string
}

// ProviderMiddleware envolve um Provider (decorator), permitindo logging,
// guardrails ou telemetria sem alterar o adaptador (feature 014).
type ProviderMiddleware func(Provider) Provider

// ToolSourceMiddleware envolve um ToolSource (decorator; feature 014).
type ToolSourceMiddleware func(ToolSource) ToolSource

// HandlerMiddleware envolve o Handler de eventos do turno (feature 014).
type HandlerMiddleware func(Handler) Handler

// StreamSink recebe eventos incrementais opcionais da chamada de modelo
// (feature 012): texto, raciocínio e fragmentos de argumentos de tool.
type StreamSink interface {
	Text(text string)
	Reasoning(text string)
	ToolCallArgs(callID, name, argsFragment string)
}

// StreamingProvider é implementado por adapters que expõem os deltas adicionais
// (reasoning e argumentos de tool). Opcional: o engine cai em Provider.Chat.
type StreamingProvider interface {
	ChatStream(ctx context.Context, req ChatRequest, sink StreamSink) (ChatResponse, error)
}

// textOnlySink adapta um callback de texto a um StreamSink.
type textOnlySink struct{ onText func(string) }

func (s textOnlySink) Text(t string) {
	if s.onText != nil {
		s.onText(t)
	}
}
func (textOnlySink) Reasoning(string)                    {}
func (textOnlySink) ToolCallArgs(string, string, string) {}

// TextSink devolve um StreamSink que só entrega texto (compatibilidade com o
// callback onText dos adaptadores).
func TextSink(onText func(string)) StreamSink { return textOnlySink{onText: onText} }

// ChatRequest é a chamada canônica enviada ao Provider.
type ChatRequest struct {
	Model           string         `json:"model"`
	SessionID       string         `json:"session_id,omitempty"`
	System          string         `json:"system,omitempty"`
	Messages        []Message      `json:"messages"`
	Tools           []Tool         `json:"tools,omitempty"`
	Params          map[string]any `json:"params,omitempty"`
	MaxOutputTokens int            `json:"max_output_tokens,omitempty"`
	// PromptCaching liga os breakpoints de cache do provedor (feature 006),
	// derivado de Capabilities.PromptCaching no turno.
	PromptCaching bool `json:"prompt_caching,omitempty"`
	// OutputSchema é o JSON Schema do structured output (feature 009).
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
}

// ChatResponse é a resposta canônica devolvida pelo Provider.
type ChatResponse struct {
	Message    Message    `json:"message"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	Usage      Usage      `json:"usage"`
	StopReason StopReason `json:"stop_reason"`
	Model      string     `json:"model"`
}

// Tool é uma tool publicada por um ToolSource, com política derivada do host.
type Tool struct {
	Name        string          `json:"name"`
	Namespace   string          `json:"namespace"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
	Destructive bool            `json:"destructive,omitempty"`
	Idempotent  bool            `json:"idempotent,omitempty"`
	Timeout     time.Duration   `json:"timeout,omitempty"`
}

// ProgressUpdate espelha o progresso reportado pela tool.
type ProgressUpdate struct {
	Message  string  `json:"message,omitempty"`
	Progress float64 `json:"progress"`
	Total    float64 `json:"total,omitempty"`
}

// FactKind classifica um fato durável de memória.
type FactKind string

const (
	FactKindFact       FactKind = "fact"
	FactKindPreference FactKind = "preference"
)

// Fact é um fato durável da memória do usuário.
type Fact struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Kind      FactKind  `json:"kind"`
	Text      string    `json:"text"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RetrievedItem é um trecho recuperado pelo Retriever, com proveniência citável.
type RetrievedItem struct {
	ID     string  `json:"id"`
	Text   string  `json:"text"`
	Source string  `json:"source"`
	Score  float64 `json:"score"`
}
