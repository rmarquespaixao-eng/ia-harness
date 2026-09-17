package session

import core "github.com/rmarquespaixao-eng/ia-harness/internal/core"

type (
	Session             = core.Session
	Message             = core.Message
	Part                = core.Part
	PartKind            = core.PartKind
	Media               = core.Media
	ToolCall            = core.ToolCall
	ToolResult          = core.ToolResult
	ResultContent       = core.ResultContent
	ResultContentKind   = core.ResultContentKind
	PendingConfirmation = core.PendingConfirmation
	TurnCheckpoint      = core.TurnCheckpoint
	CheckpointStatus    = core.CheckpointStatus
	PendingCall         = core.PendingCall
	UsageTotals         = core.UsageTotals
	SessionState        = core.SessionState
	Role                = core.Role
)

const (
	SessionActive               = core.SessionActive
	SessionAwaitingConfirmation = core.SessionAwaitingConfirmation
	SessionClosed               = core.SessionClosed
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
)
