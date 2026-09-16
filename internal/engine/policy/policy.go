package policy

import (
	"fmt"
	"path"
	"time"
)

// policyDecision é o veredito da política do agente para uma tool.
type policyDecision struct {
	Allowed              bool
	RequiresConfirmation bool
	Reason               string
}

// evaluatePolicy é o motor puro de autorização de tools (CU-HAR-3/FR-016).
// Não faz I/O nem consulta conteúdo de conversa ou de resultado de tool: o
// veredito é função apenas de PolicyConfig, agentID e do catálogo (FR-019 —
// conteúdo é dado, nunca instrução, e não eleva privilégio).
//
// Ordem das regras:
//  1. Agente ausente em Agents usa Default (deny/allow; vazio equivale a deny).
//  2. DenyTools (glob) vence qualquer permissão do agente.
//  3. Mode read_only só permite tool com override ReadOnly=true — não há
//     heurística por nome/destrutividade; a marcação explícita é a única fonte
//     de leitura (definição adotada do CU-HAR-3).
//  4. Mode allow permite tudo quando AllowTools é vazio; com globs, só o que casa.
//  5. AllowTools, quando presente, restringe também o modo read_only (interseção:
//     a tool precisa ser marcada como leitura E casar a allowlist).
//  6. ConfirmTools (glob) ou override Confirm marca RequiresConfirmation.
//  7. Mode vazio do agente herda Default; Default vazio = deny (constitution §4).
//
// Globs casam contra "servidor.nome" e contra o nome puro via path.Match, sem
// diferenciar caixa; padrão vazio ou malformado nunca casa.
//
// Nota: ToolPolicy.Timeout ainda não é consultado pelo loop (callTool usa
// Tool.Timeout do catálogo) e entra junto da evolução do loop para timeouts por
// override; por ora não há helper de timeout para evitar API morta.
func evaluatePolicy(policy PolicyConfig, agentID string, tool Tool) policyDecision {
	key := toolKey(tool)
	agent, declared := policy.Agents[agentID]
	mode := policy.Default
	if declared && agent.Mode != "" {
		mode = agent.Mode
	}

	if declared && matchesAny(agent.DenyTools, key, tool.Name) {
		return policyDecision{Reason: fmt.Sprintf("tool %q negada por deny_tools do agente %q", key, agentID)}
	}

	switch mode {
	case PolicyAllow:
		if !allowListMatches(agent, key, tool.Name) {
			return policyDecision{Reason: fmt.Sprintf("tool %q fora da allowlist do agente %q", key, agentID)}
		}
	case PolicyReadOnly:
		if !readOnlyTool(agent, declared, key, tool.Name) {
			return policyDecision{Reason: fmt.Sprintf("tool %q negada pelo modo somente-leitura do agente %q", key, agentID)}
		}
		if !allowListMatches(agent, key, tool.Name) {
			return policyDecision{Reason: fmt.Sprintf("tool %q fora da allowlist do agente %q", key, agentID)}
		}
	default:
		return policyDecision{Reason: fmt.Sprintf("tool %q negada pela política do agente %q", key, agentID)}
	}

	decision := policyDecision{Allowed: true}
	if confirmationRequired(agent, key, tool.Name) {
		decision.RequiresConfirmation = true
		decision.Reason = fmt.Sprintf("tool %q exige confirmação", key)
	}
	return decision
}

// allowListMatches informa se a allowlist do agente permite a tool: lista vazia
// permite tudo; com entradas, exige casar ao menos um glob.
func allowListMatches(agent AgentPolicy, key, name string) bool {
	if len(agent.AllowTools) == 0 {
		return true
	}
	return matchesAny(agent.AllowTools, key, name)
}

// readOnlyTool informa se a tool executa em modo somente-leitura: exige o
// override ReadOnly do agente (o catálogo não publica read-only; a marcação é
// sempre explícita na política).
func readOnlyTool(agent AgentPolicy, declared bool, key, name string) bool {
	if !declared {
		return false
	}
	override, ok := toolOverride(agent, key, name)
	return ok && override.ReadOnly
}

// confirmationRequired informa se a tool exige confirmação humana: override
// Confirm ou glob em ConfirmTools.
func confirmationRequired(agent AgentPolicy, key, name string) bool {
	if override, ok := toolOverride(agent, key, name); ok && override.Confirm {
		return true
	}
	return matchesAny(agent.ConfirmTools, key, name)
}

// toolOverride busca o ajuste fino da tool por "servidor.nome" e, na ausência,
// pelo nome puro (as duas formas são aceitas como chave).
func toolOverride(agent AgentPolicy, key, name string) (ToolPolicy, bool) {
	if len(agent.Overrides) == 0 {
		return ToolPolicy{}, false
	}
	if override, ok := agent.Overrides[key]; ok {
		return override, true
	}
	if key != name {
		if override, ok := agent.Overrides[name]; ok {
			return override, true
		}
	}
	return ToolPolicy{}, false
}

// matchesAny informa se algum glob casa com a tool.
func matchesAny(patterns []string, key, name string) bool {
	for _, pattern := range patterns {
		if matchTool(pattern, key, name) {
			return true
		}
	}
	return false
}

// matchTool casa o glob contra "servidor.nome" e "nome" (path.Match, case
// sensitive). Padrão vazio ou malformado nunca casa.
func matchTool(pattern, key, name string) bool {
	if pattern == "" {
		return false
	}
	for _, candidate := range []string{key, name} {
		if pattern == candidate {
			return true
		}
		if ok, err := path.Match(pattern, candidate); err == nil && ok {
			return true
		}
	}
	return false
}

// toolIdempotent devolve a idempotência efetiva para retry/fallback (FR-014):
// o override do agente habilita a repetição, mas não desabilita a declaração
// publicada pela tool — sem presença por campo no schema, um override parcial
// (ex.: só Confirm) não pode zerar o idempotent do MCP.
func toolIdempotent(policy PolicyConfig, agentID string, tool Tool) bool {
	if agent, ok := policy.Agents[agentID]; ok {
		if override, found := toolOverride(agent, toolKey(tool), tool.Name); found && override.Idempotent {
			return true
		}
	}
	return tool.Idempotent
}

// Timeout devolve o timeout do override da política (0 = usar o da tool).
func Timeout(policy PolicyConfig, agentID string, tool Tool) time.Duration {
	agent, ok := policy.Agents[agentID]
	if !ok {
		return 0
	}
	if override, found := toolOverride(agent, toolKey(tool), tool.Name); found && override.Timeout > 0 {
		return override.Timeout
	}
	return 0
}

// Decision é o veredito da política do agente para uma tool, exposto ao motor.
type Decision = policyDecision

// Evaluate é o wrapper exportado do motor puro de autorização (default deny,
// deny_list, allowlist glob, read_only e confirmação).
func Evaluate(policy PolicyConfig, agentID string, tool Tool) Decision {
	return evaluatePolicy(policy, agentID, tool)
}

// ToolKey devolve a chave canônica "servidor.nome" (ou o nome puro sem namespace).
func ToolKey(t Tool) string { return toolKey(t) }

// toolKey monta a chave "servidor.nome" usada nos globs de política.
func toolKey(t Tool) string {
	if t.Namespace != "" {
		return t.Namespace + "." + t.Name
	}
	return t.Name
}
