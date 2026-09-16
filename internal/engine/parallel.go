package engine

import (
	"context"
	"sync"
	"time"
)

// plannedCall é uma chamada de tool já resolvida, validada e autorizada.
type plannedCall struct {
	call ToolCall
	it   installedTool
}

// planParallel resolve, valida e autoriza todas as chamadas de um turno. Se
// qualquer chamada for desconhecida, tiver args inválidos, for negada ou exigir
// confirmação, devolve ok=false para o loop seguir no caminho sequencial
// (feature 015) — nunca paraleliza algo que exige decisão humana.
func (h *Harness) planParallel(catalog []installedTool, agentID string, calls []ToolCall) ([]plannedCall, bool) {
	plans := make([]plannedCall, 0, len(calls))
	for _, call := range calls {
		if call.ID == "" {
			call.ID = newID()
		}
		it, ok := findTool(catalog, callLookupName(call))
		if !ok {
			return nil, false
		}
		if it.validator != nil {
			if err := it.validator.Validate(call.Args); err != nil {
				return nil, false
			}
		}
		decision := h.authorize(agentID, it.tool)
		if !decision.Allowed || decision.RequiresConfirmation {
			return nil, false
		}
		plans = append(plans, plannedCall{call: call, it: it})
	}
	return plans, true
}

// parallelOutcome é o resultado de uma execução paralela.
type parallelOutcome struct {
	res     ToolResult
	err     error
	latency time.Duration
}

// executeParallel emite os ToolCallEvents em ordem, executa as tools
// concorrentemente e, após concluírem, emite resultado/auditoria e anexa ao
// histórico **na ordem original**. Progresso é emitido de forma concorrente
// pelo callback do MCP — o Handler precisa ser seguro para concorrência.
func (h *Harness) executeParallel(ctx context.Context, session *Session, handler Handler, plans []plannedCall, messages []Message, executions []ToolExecution) ([]Message, []ToolExecution) {
	for _, p := range plans {
		argsRedacted, _ := h.redactor.RedactJSON(p.call.Args)
		handler.ToolCall(ctx, ToolCallEvent{
			SessionID:    session.ID,
			CallID:       p.call.ID,
			Tool:         p.it.key,
			Namespace:    p.it.tool.Namespace,
			ArgsRedacted: argsRedacted,
			ArgsBytes:    len(p.call.Args),
		})
	}

	outcomes := make([]parallelOutcome, len(plans))
	var wg sync.WaitGroup
	for i, p := range plans {
		wg.Add(1)
		go func(i int, p plannedCall) {
			defer wg.Done()
			start := h.cfg.Clock.Now()
			res, err := h.callTool(ctx, p.it, p.call, h.effectiveToolTimeout(session.AgentID, p.it.tool), func(update ProgressUpdate) {
				handler.Progress(ctx, ProgressEvent{
					CallID:   p.call.ID,
					Tool:     p.it.key,
					Message:  update.Message,
					Progress: update.Progress,
					Total:    update.Total,
				})
			})
			outcomes[i] = parallelOutcome{res: res, err: err, latency: h.cfg.Clock.Now().Sub(start)}
		}(i, p)
	}
	wg.Wait()

	for i, p := range plans {
		out := outcomes[i]
		res := out.res
		status := StatusOK
		if out.err != nil {
			handler.Error(ctx, ErrorEvent{Scope: ErrorMCP, Tool: p.it.key, Message: "falha ao executar tool", Retryable: true})
			res = errorResult(p.call.ID, "falha de comunicação com o serviço da tool")
			status = StatusError
		} else if res.IsError {
			status = StatusError
		}
		h.emitToolResult(ctx, session, handler, p.it.tool, p.call, res, out.latency.Milliseconds())
		h.recordToolCall(ctx, session, handler, p.call, res, status, out.latency)
		session.Messages = append(session.Messages, toolMessage(p.call, res, h.cfg.Clock.Now(), h.cfg.ToolResultMaxBytes))
		messages = append(messages, session.Messages[len(session.Messages)-1])
		executions = append(executions, ToolExecution{CallID: p.call.ID, Tool: p.it.key, Status: status, LatencyMS: out.latency.Milliseconds()})
	}
	return messages, executions
}
