package session

import core "rmarquespaixao/ia-harness/internal/core"

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
