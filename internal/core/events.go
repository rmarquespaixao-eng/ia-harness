package core

import (
	"context"

	"github.com/rmarquespaixao-eng/ia-harness/internal/platform/trace"
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

// CompactionEvent reporta ao host que o contexto foi compactado (feature 018).
// É opcional: o motor só o emite quando o Handler implementa CompactionHandler.
type CompactionEvent struct {
	SessionID       string `json:"session_id"`
	TokensBefore    int    `json:"tokens_before"`
	TokensAfter     int    `json:"tokens_after"`
	MessagesRemoved int    `json:"messages_removed"`
	Summarized      bool   `json:"summarized"`
}

// CompactionHandler é a extensão opcional de Handler para observar a
// compactação de contexto; quem não a implementa simplesmente não a recebe.
type CompactionHandler interface {
	Compaction(ctx context.Context, ev CompactionEvent)
}

// RateLimitEvent reporta que uma chamada de provider foi espaçada pelo limite de
// vazão (feature 019). É opcional via RateLimitHandler.
type RateLimitEvent struct {
	Provider string `json:"provider"`
	WaitMS   int64  `json:"wait_ms"`
	// Reason descreve a origem do throttle (ex.: "requests_per_minute").
	Reason string `json:"reason,omitempty"`
}

// RateLimitHandler é a extensão opcional de Handler para observar o throttling
// de provider (feature 019).
type RateLimitHandler interface {
	RateLimited(ctx context.Context, ev RateLimitEvent)
}

// CacheEvent reporta o uso do cache semântico no turno (feature 020). É
// opcional via CacheHandler.
type CacheEvent struct {
	SessionID string  `json:"session_id"`
	Model     string  `json:"model"`
	Hit       bool    `json:"hit"`
	Score     float64 `json:"score,omitempty"`
	Key       string  `json:"key,omitempty"`
	// SavedMicros é a economia estimada quando há hit (custo que não foi gasto).
	SavedMicros int64 `json:"saved_micros,omitempty"`
}

// CacheHandler é a extensão opcional de Handler para observar o cache (020).
type CacheHandler interface {
	Cache(ctx context.Context, ev CacheEvent)
}

// CheckpointEvent reporta um ponto durável persistido (feature 021). É opcional
// via CheckpointHandler.
type CheckpointEvent struct {
	SessionID string `json:"session_id"`
	TurnID    string `json:"turn_id"`
	Status    string `json:"status"`
	Step      int    `json:"step"`
}

// CheckpointHandler é a extensão opcional de Handler para observar os
// checkpoints duráveis (feature 021).
type CheckpointHandler interface {
	Checkpoint(ctx context.Context, ev CheckpointEvent)
}

// SubAgentEvent reporta uma delegação a um agente nomeado (feature 022). É
// opcional via SubAgentHandler.
type SubAgentEvent struct {
	ParentSessionID string `json:"parent_session_id"`
	ChildSessionID  string `json:"child_session_id,omitempty"`
	Agent           string `json:"agent"`
	Depth           int    `json:"depth"`
	Status          string `json:"status"`
}

// SubAgentHandler é a extensão opcional de Handler para observar as delegações
// entre agentes (feature 022).
type SubAgentHandler interface {
	SubAgent(ctx context.Context, ev SubAgentEvent)
}

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
