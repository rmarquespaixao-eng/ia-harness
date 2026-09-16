package core

import (
	"encoding/json"
	"log/slog"
	"time"
)

// Config é a composição do harness: adaptadores e portas injetados pelo host (DI).
type Config struct {
	Providers map[string]Provider // alias do provedor → adapter instanciado
	Models    map[string]ModelProfile
	Tools     []ToolSource // um por servidor MCP; vazio = agente sem tools

	Policy       PolicyConfig
	Pricing      Pricing
	Context      ContextPolicy
	Redaction    RedactionConfig
	DefaultModel string
	// SystemPrompt é o prompt de sistema global; o agente pode sobrepor
	// (AgentPolicy.SystemPrompt). Vai em ChatRequest.System, separado das
	// mensagens de sistema de memória/resumo (feature 005).
	SystemPrompt string
	// ToolResultMaxBytes limita o conteúdo de um resultado de tool que entra no
	// histórico enviado ao modelo (feature 013); excedente vira Truncated.
	ToolResultMaxBytes int
	// ParallelTools executa tool calls independentes do mesmo turno em paralelo
	// quando todas forem permitidas e sem confirmação (feature 015). O Handler
	// precisa ser seguro para concorrência (progresso).
	ParallelTools bool

	Credentials CredentialProvider // * obrigatória
	Sessions    SessionStore       // * obrigatória
	Logger      *slog.Logger       // * obrigatória
	Clock       Clock              // default: clock do sistema
	Audit       AuditSink          // default: sink nop (host costuma injetar adapters/audit/log)
	Memory      MemoryStore        // opcional
	Retriever   Retriever          // opcional
	Embedder    Embedder           // opcional
	Tokenizer   Tokenizer          // opcional: default heurístico por bytes
	Tracer      Tracer             // opcional: default no-op (OTel no host)

	// Middleware (feature 014): decoradores aplicados no boot aos providers e
	// tools injetados, e por turno ao Handler de eventos.
	ProviderMiddleware []ProviderMiddleware
	ToolMiddleware     []ToolSourceMiddleware
	HandlerMiddleware  []HandlerMiddleware
}

// ModelProfile descreve um alias de modelo e sua cadeia de fallback.
type ModelProfile struct {
	Provider     string         `json:"provider"`
	Model        string         `json:"model"`
	Capabilities Capabilities   `json:"capabilities"`
	Params       map[string]any `json:"params,omitempty"`
	Fallbacks    []string       `json:"fallbacks,omitempty"`
}

// Capabilities informa o que o modelo suporta e seus limites.
type Capabilities struct {
	ToolCalling      bool  `json:"tool_calling"`
	Streaming        bool  `json:"streaming"`
	MaxContextTokens int   `json:"max_context_tokens"`
	MaxOutputTokens  int   `json:"max_output_tokens"`
	Vision           bool  `json:"vision"`
	Documents        bool  `json:"documents"`
	MaxMediaBytes    int64 `json:"max_media_bytes"`
	// PromptCaching habilita breakpoints de cache do provedor (feature 006).
	PromptCaching bool `json:"prompt_caching"`
}

// PolicyMode é o modo de permissão de tools.
type PolicyMode string

const (
	PolicyDeny     PolicyMode = "deny"
	PolicyAllow    PolicyMode = "allow"
	PolicyReadOnly PolicyMode = "read_only"
)

// PolicyConfig é a política global; o default do núcleo é deny.
type PolicyConfig struct {
	Default PolicyMode             `json:"default"`
	Agents  map[string]AgentPolicy `json:"agents,omitempty"`
}

// AgentPolicy é a política de um agente (glob por nome ou servidor.nome).
type AgentPolicy struct {
	Mode         PolicyMode            `json:"mode,omitempty"`
	AllowTools   []string              `json:"allow_tools,omitempty"`
	DenyTools    []string              `json:"deny_tools,omitempty"`
	ConfirmTools []string              `json:"confirm_tools,omitempty"`
	Overrides    map[string]ToolPolicy `json:"overrides,omitempty"`
	// SystemPrompt sobrepõe o prompt global quando não vazio (feature 005).
	SystemPrompt string `json:"system_prompt,omitempty"`
}

// ToolPolicy é o ajuste fino de uma tool específica.
type ToolPolicy struct {
	Confirm    bool          `json:"confirm,omitempty"`
	ReadOnly   bool          `json:"read_only,omitempty"`
	Idempotent bool          `json:"idempotent,omitempty"`
	Timeout    time.Duration `json:"timeout,omitempty"`
}

// Pricing é a premissa de custo usada quando o provedor não reporta tokens.
type Pricing struct {
	Currency                    string  `json:"currency"`
	InputPriceMicrosPerMillion  int64   `json:"input_price_micros_per_million"`
	OutputPriceMicrosPerMillion int64   `json:"output_price_micros_per_million"`
	BytesPerToken               float64 `json:"bytes_per_token"`
	// CachedInputPriceMicrosPerMillion e CacheWritePriceMicrosPerMillion são os
	// preços de leitura/escrita de cache (feature 006); zero cai no preço de
	// input (sem desconto, nunca subestima).
	CachedInputPriceMicrosPerMillion int64 `json:"cached_input_price_micros_per_million"`
	CacheWritePriceMicrosPerMillion  int64 `json:"cache_write_price_micros_per_million"`
}

// ContextStrategy define como a janela de contexto é ajustada.
type ContextStrategy string

const (
	StrategyTruncateOldest ContextStrategy = "truncate_oldest"
	StrategySummarize      ContextStrategy = "summarize"
)

// ContextPolicy limita e ajusta o contexto montado por turno.
type ContextPolicy struct {
	MaxTokens  int             `json:"max_tokens,omitempty"`
	Strategy   ContextStrategy `json:"strategy,omitempty"`
	Summarizer Summarizer      `json:"-"`
}

// RedactionConfig controla a redação/truncamento dos campos auditados.
type RedactionConfig struct {
	SensitiveKeys []string `json:"sensitive_keys,omitempty"`
	MaxFieldBytes int      `json:"max_field_bytes,omitempty"`
}

// RunRequest é a entrada de um turno; SessionID vazio cria sessão nova.
type RunRequest struct {
	SessionID     string
	UserID        string
	AgentID       string
	Model         string
	Input         []Part
	MaxIterations int
	Budget        Budget
	// OutputSchema, quando presente, pede structured output: o provedor é
	// instruído a responder no schema e o resultado é validado (feature 009).
	OutputSchema json.RawMessage
}

// Budget limita tokens e tempo de parede do turno.
type Budget struct {
	MaxTokens int64
	MaxWall   time.Duration
}

// StopReason explica por que o turno terminou.
type StopReason string

const (
	StopCompleted     StopReason = "completed"
	StopMaxIterations StopReason = "max_iterations"
	StopBudget        StopReason = "budget"
	StopCancelled     StopReason = "cancelled"
	StopError         StopReason = "error"
)

// ToolExecution resume uma execução de tool no turno.
type ToolExecution struct {
	CallID    string `json:"call_id"`
	Tool      string `json:"tool"`
	Status    string `json:"status"`
	LatencyMS int64  `json:"latency_ms"`
}

// TurnResult é o desfecho de um turno.
type TurnResult struct {
	SessionID  string
	State      SessionState
	Output     []Part
	Pending    *PendingConfirmation
	Usage      Usage
	ToolCalls  []ToolExecution
	StopReason StopReason
	Model      string
}

// Decision é a resposta do host a um ConfirmationEvent.
type Decision struct {
	Approve bool   `json:"approve"`
	Reason  string `json:"reason,omitempty"`
}
