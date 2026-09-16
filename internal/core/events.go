package core

import (
	"context"

	"rmarquespaixao/ia-harness/internal/platform/trace"
)

// Handler recebe os eventos do turno de forma síncrona e na ordem; o host decide o transporte.
type Handler interface {
	TextDelta(ctx context.Context, ev TextDelta)                              // fragmento incremental da resposta
	ReasoningDelta(ctx context.Context, ev ReasoningDelta)                    // fragmento de raciocínio (opcional, feature 012)
	ToolCallDelta(ctx context.Context, ev ToolCallArgsDelta)                  // fragmento de argumentos de tool (feature 012)
	ToolCall(ctx context.Context, ev ToolCallEvent)                           // tool escolhida e validada
	ToolResult(ctx context.Context, ev ToolResultEvent)                       // resultado, erro ou negativa
	Progress(ctx context.Context, ev ProgressEvent)                           // tools longas
	Confirmation(ctx context.Context, ev ConfirmationEvent) (Decision, error) // pausa até a decisão
	Usage(ctx context.Context, ev UsageEvent)                                 // fim de chamada de modelo
	Error(ctx context.Context, ev ErrorEvent)                                 // erro recuperável/parcial
}

// NopHandler implementa Handler sem efeito; embuta para sobrescrever só o necessário.
type NopHandler struct{}

func (NopHandler) TextDelta(context.Context, TextDelta)             {}
func (NopHandler) ReasoningDelta(context.Context, ReasoningDelta)   {}
func (NopHandler) ToolCallDelta(context.Context, ToolCallArgsDelta) {}
func (NopHandler) ToolCall(context.Context, ToolCallEvent)          {}
func (NopHandler) ToolResult(context.Context, ToolResultEvent)      {}
func (NopHandler) Progress(context.Context, ProgressEvent)          {}
func (NopHandler) Usage(context.Context, UsageEvent)                {}
func (NopHandler) Error(context.Context, ErrorEvent)                {}

// Confirmation devolve decisão vazia (sem aprovação do host).
func (NopHandler) Confirmation(context.Context, ConfirmationEvent) (Decision, error) {
	return Decision{}, nil
}

// TextDelta é um fragmento incremental da resposta do modelo.
type TextDelta struct {
	SessionID string `json:"session_id"`
	MessageID string `json:"message_id"`
	Text      string `json:"text"`
}

// ReasoningDelta é um fragmento incremental do raciocínio do modelo (feature 012).
type ReasoningDelta struct {
	SessionID string `json:"session_id"`
	MessageID string `json:"message_id"`
	Text      string `json:"text"`
}

// ToolCallArgsDelta é um fragmento incremental dos argumentos de uma tool
// (feature 012); o CallID pode vir vazio nos primeiros fragmentos.
type ToolCallArgsDelta struct {
	SessionID    string `json:"session_id"`
	CallID       string `json:"call_id,omitempty"`
	Tool         string `json:"tool,omitempty"`
	ArgsFragment string `json:"args_fragment"`
}

// ToolCallEvent anuncia uma tool escolhida e validada (sem segredos nos args).
type ToolCallEvent struct {
	SessionID    string `json:"session_id"`
	CallID       string `json:"call_id"`
	Tool         string `json:"tool"`
	Namespace    string `json:"namespace"`
	ArgsRedacted string `json:"args_redacted"`
	ArgsBytes    int    `json:"args_bytes"`
}

// ToolResultEvent reporta o desfecho de uma execução de tool.
type ToolResultEvent struct {
	CallID        string `json:"call_id"`
	Tool          string `json:"tool"`
	Status        string `json:"status"`
	IsError       bool   `json:"is_error,omitempty"`
	Denied        bool   `json:"denied,omitempty"`
	Truncated     bool   `json:"truncated,omitempty"`
	LatencyMS     int64  `json:"latency_ms"`
	ResultSummary string `json:"result_summary,omitempty"`
}

// ProgressEvent espelha notifications/progress do MCP.
type ProgressEvent struct {
	CallID   string  `json:"call_id"`
	Tool     string  `json:"tool"`
	Message  string  `json:"message,omitempty"`
	Progress float64 `json:"progress"`
	Total    float64 `json:"total,omitempty"`
}

// ConfirmationEvent pede a decisão humana sobre uma tool que exige confirmação.
type ConfirmationEvent struct {
	SessionID    string `json:"session_id"`
	CallID       string `json:"call_id"`
	Tool         string `json:"tool"`
	ArgsRedacted string `json:"args_redacted"`
	Reason       string `json:"reason"`
}

// UsageEvent reporta o consumo de uma chamada de modelo.
type UsageEvent struct {
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	Estimated    bool   `json:"estimated"`
	CostMicros   int64  `json:"cost_micros,omitempty"`
	Currency     string `json:"currency,omitempty"`
	// Prompt caching (feature 006): tokens lidos/escritos no cache.
	CachedInputTokens int64 `json:"cached_input_tokens,omitempty"`
	CacheWriteTokens  int64 `json:"cache_write_tokens,omitempty"`
}

// ErrorEvent reporta um erro sem derrubar o turno quando recuperável.
type ErrorEvent struct {
	Scope     ErrorScope `json:"scope"`
	Tool      string     `json:"tool,omitempty"`
	Message   string     `json:"message"`
	Retryable bool       `json:"retryable,omitempty"`
}

// ErrorScope indica a origem do erro do turno.
type ErrorScope string

const (
	ErrorModel ErrorScope = "model"
	ErrorTool  ErrorScope = "tool"
	ErrorMCP   ErrorScope = "mcp"
)

// Status dos resultados de tool (ToolResultEvent.Status e ToolExecution.Status).
const (
	StatusOK     = "ok"
	StatusError  = "error"
	StatusDenied = "denied"
	// StatusAwaitingConfirmation marca a tool que pausou o turno aguardando decisão.
	StatusAwaitingConfirmation = "awaiting_confirmation"
)

// WithTraceID injeta o trace_id no contexto (propagado à auditoria e ao log).
func WithTraceID(ctx context.Context, id string) context.Context {
	return trace.WithID(ctx, id)
}
