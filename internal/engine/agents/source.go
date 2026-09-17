package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	core "github.com/rmarquespaixao-eng/ia-harness/internal/core"
)

// Source é um ToolSource sintético que expõe agents.delegate e executa o
// sub-turno pelo core.AgentRunner injetado (feature 022).
type Source struct {
	runner   core.AgentRunner
	agents   map[string]core.AgentSpec
	maxDepth int
}

// NewSource monta a fonte; maxDepth ≤ 0 vira 1.
func NewSource(runner core.AgentRunner, agents map[string]core.AgentSpec, maxDepth int) *Source {
	if maxDepth <= 0 {
		maxDepth = 1
	}
	return &Source{runner: runner, agents: agents, maxDepth: maxDepth}
}

// List devolve a tool de delegação (vazia quando não há agentes).
func (s *Source) List(ctx context.Context) ([]core.Tool, error) {
	if s == nil || len(s.agents) == 0 {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []core.Tool{DelegateTool(s.agents)}, nil
}

// Call executa a delegação: valida o agente e a profundidade, abre o sub-turno
// pelo runner e converte o texto final em resultado. Erros de uso são resultado
// de erro (nunca erro de transporte).
func (s *Source) Call(ctx context.Context, name string, args json.RawMessage, onProgress func(core.ProgressUpdate)) (core.ToolResult, error) {
	if s == nil || len(s.agents) == 0 {
		return errorResult("delegação indisponível: nenhum agente configurado"), nil
	}
	if !isDelegate(name) {
		return errorResult(fmt.Sprintf("tool desconhecida: %s", name)), nil
	}
	var parsed delegateArgs
	if err := json.Unmarshal(args, &parsed); err != nil {
		return errorResult("argumentos inválidos: " + err.Error()), nil
	}
	if _, ok := s.agents[parsed.Agent]; !ok {
		return errorResult(fmt.Sprintf("agente %q não configurado", parsed.Agent)), nil
	}
	if parsed.Task == "" {
		return errorResult("campo task é obrigatório"), nil
	}
	call := FromCallContext(ctx)
	if call.Depth+1 > s.maxDepth {
		return errorResult(fmt.Sprintf("profundidade máxima de delegação (%d) atingida", s.maxDepth)), nil
	}

	childCtx := WithCallContext(ctx, CallContext{
		Depth:           call.Depth + 1,
		UserID:          call.UserID,
		ParentSessionID: call.ParentSessionID,
		Handler:         call.Handler,
	})
	result, err := s.runner.RunSubAgent(childCtx, core.SubAgentRequest{
		ParentSessionID: call.ParentSessionID,
		UserID:          call.UserID,
		AgentID:         parsed.Agent,
		Input:           []core.Part{{Kind: core.PartText, Text: parsed.Task}},
	})
	if err != nil {
		return errorResult("falha no sub-agente: " + err.Error()), nil
	}
	if len(result.Output) == 0 {
		return core.ToolResult{Content: []core.ResultContent{{Kind: core.ResultText, Text: fmt.Sprintf("agente %q concluiu sem texto", parsed.Agent)}}}, nil
	}
	return core.ToolResult{Content: []core.ResultContent{{Kind: core.ResultText, Text: outputText(result.Output)}}}, nil
}

// Close não mantém recursos.
func (s *Source) Close() error { return nil }

// AgentNames devolve os agentes configurados em ordem determinística.
func (s *Source) AgentNames() []string {
	names := make([]string, 0, len(s.agents))
	for name := range s.agents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Spec do agente pelo nome.
func (s *Source) Spec(agent string) (core.AgentSpec, bool) {
	spec, ok := s.agents[agent]
	return spec, ok
}

// delegateArgs são os argumentos aceitos por agents.delegate.
type delegateArgs struct {
	Agent string `json:"agent"`
	Task  string `json:"task"`
}

// isDelegate informa se o nome recebido é a tool de delegação.
func isDelegate(name string) bool {
	return name == DelegateName || name == Namespace+"."+DelegateName
}

// outputText concatena as partes de texto do sub-turno.
func outputText(parts []core.Part) string {
	var b []byte
	for _, part := range parts {
		if part.Kind == core.PartText {
			b = append(b, part.Text...)
		}
	}
	return string(b)
}

// errorResult monta um resultado de erro de uso da delegação.
func errorResult(message string) core.ToolResult {
	return core.ToolResult{IsError: true, Content: []core.ResultContent{{Kind: core.ResultText, Text: message}}}
}
