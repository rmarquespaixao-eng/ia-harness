// Package agents implementa a delegação entre agentes nomeados (feature 022):
// filtro de tools por glob, a tool sintética agents.delegate e o contexto de
// profundidade da delegação. A execução do sub-turno é porta do motor
// (core.AgentRunner), mantendo a direção das dependências.
package agents

import (
	"context"
	"encoding/json"
	"path"
	"sort"
	"strings"

	core "github.com/rmarquespaixao-eng/ia-harness/internal/core"
)

const (
	// Namespace da tool sintética de delegação.
	Namespace = "agents"
	// DelegateName é o nome puro da tool sintética.
	DelegateName = "delegate"
)

// delegateSchema é o JSON Schema autossuficiente dos argumentos da delegação.
var delegateSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "agent": { "type": "string", "description": "identificador do agente alvo" },
    "task": { "type": "string", "description": "tarefa a ser executada pelo agente alvo" }
  },
  "required": ["agent", "task"],
  "additionalProperties": false
}`)

// CallContext acompanha a delegação no contexto: profundidade atual, o handler
// do host (para emitir SubAgentEvent) e a sessão/usuário do pai.
type CallContext struct {
	Depth           int
	UserID          string
	ParentSessionID string
	Handler         core.Handler
}

type callContextKey struct{}

// WithCallContext injeta o contexto de delegação.
func WithCallContext(ctx context.Context, c CallContext) context.Context {
	return context.WithValue(ctx, callContextKey{}, c)
}

// FromCallContext recupera o contexto de delegação (zero quando ausente).
func FromCallContext(ctx context.Context) CallContext {
	if c, ok := ctx.Value(callContextKey{}).(CallContext); ok {
		return c
	}
	return CallContext{}
}

// DelegateTool devolve a tool sintética de delegação, com a descrição dinâmica
// dos agentes disponíveis (ordem determinística).
func DelegateTool(agents map[string]core.AgentSpec) core.Tool {
	names := make([]string, 0, len(agents))
	for name := range agents {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("Delega a tarefa a um agente especializado. Agentes disponíveis: ")
	for i, name := range names {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(name)
		if desc := strings.TrimSpace(agents[name].Description); desc != "" {
			b.WriteString(" (")
			b.WriteString(desc)
			b.WriteString(")")
		}
	}
	return core.Tool{
		Name:        DelegateName,
		Namespace:   Namespace,
		Description: b.String(),
		InputSchema: delegateSchema,
	}
}

// FilterTools restringe o catálogo aos padrões (glob por "namespace.nome" ou
// nome puro). Lista vazia herda o catálogo inteiro (FR-MA-006).
func FilterTools(tools []core.Tool, patterns []string) []core.Tool {
	if len(patterns) == 0 {
		return tools
	}
	out := make([]core.Tool, 0, len(tools))
	for _, tool := range tools {
		if matchesAny(patterns, tool) {
			out = append(out, tool)
		}
	}
	return out
}

// matchesAny informa se algum glob casa a tool.
func matchesAny(patterns []string, tool core.Tool) bool {
	key := keyOf(tool)
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		if pattern == key || pattern == tool.Name {
			return true
		}
		if ok, err := path.Match(pattern, key); err == nil && ok {
			return true
		}
		if ok, err := path.Match(pattern, tool.Name); err == nil && ok {
			return true
		}
	}
	return false
}

// keyOf monta a chave "namespace.nome" da tool.
func keyOf(tool core.Tool) string {
	if tool.Namespace != "" {
		return tool.Namespace + "." + tool.Name
	}
	return tool.Name
}
