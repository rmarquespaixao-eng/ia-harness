package core

import (
	"encoding/json"
	"time"
)

// Role é o papel de uma mensagem no histórico canônico.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// PartKind distingue as variantes da união tagueada Part.
type PartKind string

const (
	PartText       PartKind = "text"
	PartToolCall   PartKind = "tool_call"
	PartToolResult PartKind = "tool_result"
	PartImage      PartKind = "image"
	PartDocument   PartKind = "document"
)

// Part é uma unidade de conteúdo de uma mensagem.
type Part struct {
	Kind   PartKind    `json:"kind"`
	Text   string      `json:"text,omitempty"`
	Call   *ToolCall   `json:"call,omitempty"`
	Result *ToolResult `json:"result,omitempty"`
	Media  *Media      `json:"media,omitempty"`
}

// Media é a mídia de uma Part image/document (feature 002). Exatamente uma
// fonte deve estar preenchida: Bytes (inline, base64) xor Reference (URL/ref do
// host). MIME passa por allowlist e SizeBytes por teto (media.Validate).
type Media struct {
	MIME      string `json:"mime"`
	Name      string `json:"name,omitempty"`
	SizeBytes int64  `json:"size_bytes"`
	Bytes     []byte `json:"bytes,omitempty"`
	Reference string `json:"reference,omitempty"`
}

// ToolCall é a chamada de tool escolhida pelo modelo (args já validados na execução).
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Namespace string          `json:"namespace"`
	Args      json.RawMessage `json:"args"`
}

// ResultContentKind distingue texto de JSON estruturado no resultado.
type ResultContentKind string

const (
	ResultText ResultContentKind = "text"
	ResultJSON ResultContentKind = "json"
)

// ResultContent é um item do resultado de uma tool (binário nunca é propagado).
type ResultContent struct {
	Kind ResultContentKind `json:"kind"`
	Text string            `json:"text,omitempty"`
	JSON json.RawMessage   `json:"json,omitempty"`
}

// ToolResult é o resultado, erro ou recusa de uma chamada de tool.
type ToolResult struct {
	CallID    string          `json:"call_id"`
	Content   []ResultContent `json:"content"`
	IsError   bool            `json:"is_error,omitempty"`
	Denied    bool            `json:"denied,omitempty"`
	Truncated bool            `json:"truncated,omitempty"`
}

// Message é uma mensagem do histórico da sessão.
type Message struct {
	ID        string    `json:"id"`
	Role      Role      `json:"role"`
	Parts     []Part    `json:"parts"`
	CreatedAt time.Time `json:"created_at"`
}

// SessionState é o estado da sessão.
type SessionState string

const (
	SessionActive               SessionState = "active"
	SessionAwaitingConfirmation SessionState = "awaiting_confirmation"
	SessionClosed               SessionState = "closed"
)

// PendingConfirmation é a confirmação humana pendente de uma tool.
type PendingConfirmation struct {
	CallID       string    `json:"call_id"`
	ToolName     string    `json:"tool_name"`
	ArgsRedacted string    `json:"args_redacted"`
	Reason       string    `json:"reason"`
	RequestedAt  time.Time `json:"requested_at"`
}

// Usage é o consumo de uma chamada de modelo; campos estimados vêm da premissa.
type Usage struct {
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	Estimated    bool   `json:"estimated"`
	CostMicros   int64  `json:"cost_micros,omitempty"`
	Currency     string `json:"currency,omitempty"`
	// Tokens de prompt caching (feature 006): leitura e escrita de cache.
	CachedInputTokens int64 `json:"cached_input_tokens,omitempty"`
	CacheWriteTokens  int64 `json:"cache_write_tokens,omitempty"`
}

// UsageTotals é o consumo acumulado da sessão.
type UsageTotals = Usage

// Session é o histórico canônico persistido via SessionStore.
type Session struct {
	ID        string               `json:"id"`
	UserID    string               `json:"user_id"`
	AgentID   string               `json:"agent_id"`
	Model     string               `json:"model"`
	State     SessionState         `json:"state"`
	Messages  []Message            `json:"messages"`
	Pending   *PendingConfirmation `json:"pending,omitempty"`
	Usage     UsageTotals          `json:"usage"`
	CreatedAt time.Time            `json:"created_at"`
	UpdatedAt time.Time            `json:"updated_at"`
}
