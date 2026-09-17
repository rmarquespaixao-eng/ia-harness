package harness

import (
	gen "github.com/rmarquespaixao-eng/ia-harness/contracts/gen"
	core "github.com/rmarquespaixao-eng/ia-harness/internal/core"
	"github.com/rmarquespaixao-eng/ia-harness/internal/engine/telemetry"
)

type (
	Config               = core.Config
	ModelProfile         = core.ModelProfile
	Capabilities         = core.Capabilities
	PolicyMode           = core.PolicyMode
	PolicyConfig         = core.PolicyConfig
	AgentPolicy          = core.AgentPolicy
	ToolPolicy           = core.ToolPolicy
	Pricing              = core.Pricing
	ContextStrategy      = core.ContextStrategy
	ContextPolicy        = core.ContextPolicy
	RedactionConfig      = core.RedactionConfig
	RateLimit            = core.RateLimit
	SemanticCacheConfig  = core.SemanticCacheConfig
	AgentSpec            = core.AgentSpec
	RunRequest           = core.RunRequest
	Budget               = core.Budget
	StopReason           = core.StopReason
	ToolExecution        = core.ToolExecution
	TurnResult           = core.TurnResult
	CompactionInfo       = core.CompactionInfo
	CompactionEvent      = core.CompactionEvent
	CompactionHandler    = core.CompactionHandler
	CacheInfo            = core.CacheInfo
	CacheEvent           = core.CacheEvent
	CacheHandler         = core.CacheHandler
	RateLimitEvent       = core.RateLimitEvent
	RateLimitHandler     = core.RateLimitHandler
	CheckpointEvent      = core.CheckpointEvent
	CheckpointHandler    = core.CheckpointHandler
	SubAgentEvent        = core.SubAgentEvent
	SubAgentHandler      = core.SubAgentHandler
	Decision             = core.Decision
	Provider             = core.Provider
	ToolSource           = core.ToolSource
	SessionStore         = core.SessionStore
	AuditSink            = core.AuditSink
	MemoryStore          = core.MemoryStore
	Retriever            = core.Retriever
	Embedder             = core.Embedder
	CredentialProvider   = core.CredentialProvider
	Summarizer           = core.Summarizer
	ModelSummarizer      = core.ModelSummarizer
	Clock                = core.Clock
	Waiter               = core.Waiter
	SemanticCache        = core.SemanticCache
	CacheQuery           = core.CacheQuery
	CacheEntry           = core.CacheEntry
	AgentRunner          = core.AgentRunner
	SubAgentRequest      = core.SubAgentRequest
	SubAgentResult       = core.SubAgentResult
	Tokenizer            = core.Tokenizer
	Tracer               = core.Tracer
	Span                 = core.Span
	TurnAttrs            = core.TurnAttrs
	ModelAttrs           = core.ModelAttrs
	ToolAttrs            = core.ToolAttrs
	ProviderMiddleware   = core.ProviderMiddleware
	ToolSourceMiddleware = core.ToolSourceMiddleware
	HandlerMiddleware    = core.HandlerMiddleware
	StreamSink           = core.StreamSink
	StreamingProvider    = core.StreamingProvider
	ReasoningDelta       = core.ReasoningDelta
	ToolCallArgsDelta    = core.ToolCallArgsDelta
	ChatRequest          = core.ChatRequest
	ChatResponse         = core.ChatResponse
	Tool                 = core.Tool
	ProgressUpdate       = core.ProgressUpdate
	FactKind             = core.FactKind
	Fact                 = core.Fact
	RetrievedItem        = core.RetrievedItem
	AuditEvent           = core.AuditEvent
	Handler              = core.Handler
	NopHandler           = core.NopHandler
	TextDelta            = core.TextDelta
	ToolCallEvent        = core.ToolCallEvent
	ToolResultEvent      = core.ToolResultEvent
	ProgressEvent        = core.ProgressEvent
	ConfirmationEvent    = core.ConfirmationEvent
	UsageEvent           = core.UsageEvent
	ErrorEvent           = core.ErrorEvent
	ErrorScope           = core.ErrorScope
	Role                 = core.Role
	PartKind             = core.PartKind
	Part                 = core.Part
	Media                = core.Media
	ToolCall             = core.ToolCall
	ResultContentKind    = core.ResultContentKind
	ResultContent        = core.ResultContent
	ToolResult           = core.ToolResult
	Message              = core.Message
	SessionState         = core.SessionState
	PendingConfirmation  = core.PendingConfirmation
	Usage                = core.Usage
	UsageTotals          = core.UsageTotals
	ConfigError          = core.ConfigError
	PolicyError          = core.PolicyError
	ToolError            = core.ToolError
	ProviderError        = core.ProviderError
	OutputError          = core.OutputError
	Session              = core.Session
	TurnCheckpoint       = core.TurnCheckpoint
	CheckpointStatus     = core.CheckpointStatus
	PendingCall          = core.PendingCall
)

type SessionSnapshot = gen.SessionSnapshot

// Redactor redige chaves sensíveis e trunca campos antes de log/auditoria.
type Redactor = telemetry.Redactor

const (
	PolicyDeny                  = core.PolicyDeny
	PolicyAllow                 = core.PolicyAllow
	PolicyReadOnly              = core.PolicyReadOnly
	StrategyTruncateOldest      = core.StrategyTruncateOldest
	StrategySummarize           = core.StrategySummarize
	StopCompleted               = core.StopCompleted
	StopMaxIterations           = core.StopMaxIterations
	StopBudget                  = core.StopBudget
	StopCancelled               = core.StopCancelled
	StopError                   = core.StopError
	FactKindFact                = core.FactKindFact
	FactKindPreference          = core.FactKindPreference
	RoleSystem                  = core.RoleSystem
	RoleUser                    = core.RoleUser
	RoleAssistant               = core.RoleAssistant
	RoleTool                    = core.RoleTool
	PartText                    = core.PartText
	PartToolCall                = core.PartToolCall
	PartToolResult              = core.PartToolResult
	PartImage                   = core.PartImage
	PartDocument                = core.PartDocument
	ResultText                  = core.ResultText
	ResultJSON                  = core.ResultJSON
	SessionActive               = core.SessionActive
	SessionAwaitingConfirmation = core.SessionAwaitingConfirmation
	SessionClosed               = core.SessionClosed
	ErrorModel                  = core.ErrorModel
	ErrorTool                   = core.ErrorTool
	ErrorMCP                    = core.ErrorMCP
	StatusOK                    = core.StatusOK
	StatusError                 = core.StatusError
	StatusDenied                = core.StatusDenied
	StatusAwaitingConfirmation  = core.StatusAwaitingConfirmation
	CheckpointRunning           = core.CheckpointRunning
	CheckpointAwaitingConfirm   = core.CheckpointAwaitingConfirmation
	CheckpointCompleted         = core.CheckpointCompleted
)
