package policy

import core "rmarquespaixao/ia-harness/internal/core"

type (
	PolicyConfig = core.PolicyConfig
	AgentPolicy  = core.AgentPolicy
	ToolPolicy   = core.ToolPolicy
	PolicyMode   = core.PolicyMode
	Tool         = core.Tool
)

const (
	PolicyDeny     = core.PolicyDeny
	PolicyAllow    = core.PolicyAllow
	PolicyReadOnly = core.PolicyReadOnly
)
