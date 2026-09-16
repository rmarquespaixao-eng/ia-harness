package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"rmarquespaixao/ia-harness/internal/engine/budget"
	"rmarquespaixao/ia-harness/internal/engine/media"
	"rmarquespaixao/ia-harness/internal/engine/policy"
	"rmarquespaixao/ia-harness/internal/platform/schema"
)

// installedTool é uma tool do catálogo do turno com a fonte dona e o validador
// de argumentos já compilado (o schema é compilado uma vez por turno — R2).
type installedTool struct {
	key       string
	tool      Tool
	source    ToolSource
	validator *schema.Validator
}

// resumeState carrega a decisão de uma confirmação pendente (retomada).
type resumeState struct {
	call     ToolCall
	decision Decision
}

func newID() string { return uuid.NewString() }

// outputText concatena as partes de texto da saída final (validação do
// structured output — feature 009).
func outputText(parts []Part) string {
	var b strings.Builder
	for _, part := range parts {
		if part.Kind == PartText {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}

// turnSink encaminha os deltas do provider para o Handler do turno (feature 012).
type turnSink struct {
	ctx       context.Context
	sessionID string
	messageID string
	handler   Handler
}

func (s turnSink) Text(text string) {
	s.handler.TextDelta(s.ctx, TextDelta{SessionID: s.sessionID, MessageID: s.messageID, Text: text})
}

func (s turnSink) Reasoning(text string) {
	s.handler.ReasoningDelta(s.ctx, ReasoningDelta{SessionID: s.sessionID, MessageID: s.messageID, Text: text})
}

func (s turnSink) ToolCallArgs(callID, name, fragment string) {
	s.handler.ToolCallDelta(s.ctx, ToolCallArgsDelta{SessionID: s.sessionID, CallID: callID, Tool: name, ArgsFragment: fragment})
}

func toolKey(t Tool) string {
	if t.Namespace != "" {
		return t.Namespace + "." + t.Name
	}
	return t.Name
}

func callLookupName(call ToolCall) string {
	if call.Namespace != "" {
		return call.Namespace + "." + call.Name
	}
	return call.Name
}

// catalog soma as tools de todas as fontes, compilando cada schema uma vez.
func (h *Harness) catalog(ctx context.Context) ([]installedTool, error) {
	var out []installedTool
	for _, src := range h.cfg.Tools {
		tools, err := src.List(ctx)
		if err != nil {
			return nil, fmt.Errorf("harness: listar tools: %w", err)
		}
		for _, t := range tools {
			it := installedTool{key: toolKey(t), tool: t, source: src}
			if len(t.InputSchema) > 0 {
				validator, err := schema.Compile(t.InputSchema)
				if err != nil {
					return nil, fmt.Errorf("harness: schema da tool %q: %w", it.key, err)
				}
				it.validator = validator
			}
			out = append(out, it)
		}
	}
	return out, nil
}

func findTool(catalog []installedTool, name string) (installedTool, bool) {
	for _, it := range catalog {
		if it.key == name || it.tool.Name == name {
			return it, true
		}
	}
	return installedTool{}, false
}

func textContent(text string) []ResultContent {
	return []ResultContent{{Kind: ResultText, Text: text}}
}

func errorResult(callID, message string) ToolResult {
	return ToolResult{CallID: callID, IsError: true, Content: textContent(message)}
}

func deniedResult(callID, reason string) ToolResult {
	if reason == "" {
		reason = "operação negada pela política do agente"
	}
	return ToolResult{CallID: callID, IsError: true, Denied: true, Content: textContent(reason)}
}

func toolMessage(call ToolCall, res ToolResult, now time.Time, maxBytes int) Message {
	result := truncateToolResult(res, maxBytes)
	return Message{
		ID:        newID(),
		Role:      RoleTool,
		Parts:     []Part{{Kind: PartToolResult, Result: &result}},
		CreatedAt: now,
	}
}

// callTool executa a tool na fonte dona, aplicando timeout e progresso, envolta
// num span de tool (feature 011). Erro de transporte é devolvido cru para o
// chamador emitir ErrorEvent{Scope: mcp}.
func (h *Harness) callTool(ctx context.Context, it installedTool, call ToolCall, timeout time.Duration, onProgress func(ProgressUpdate)) (ToolResult, error) {
	_, span := h.cfg.Tracer.StartTool(ctx, ToolAttrs{Tool: it.key, CallID: call.ID})
	res, err := h.invokeTool(ctx, it, call, timeout, onProgress)
	span.End(err)
	return res, err
}

// invokeTool é a execução efetiva da tool (sem span).
func (h *Harness) invokeTool(ctx context.Context, it installedTool, call ToolCall, timeout time.Duration, onProgress func(ProgressUpdate)) (ToolResult, error) {
	callCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	res, err := it.source.Call(callCtx, it.tool.Name, call.Args, onProgress)
	if err != nil {
		return ToolResult{CallID: call.ID}, err
	}
	res.CallID = call.ID
	return res, nil
}

// effectiveToolTimeout resolve o timeout da chamada (override da política vence
// o timeout publicado pela tool — FR-016/data-model).
func (h *Harness) effectiveToolTimeout(agentID string, tool Tool) time.Duration {
	if override := policy.Timeout(h.cfg.Policy, agentID, tool); override > 0 {
		return override
	}
	return tool.Timeout
}

// effectiveSystemPrompt resolve o system prompt do turno: o override do agente
// (quando não vazio) vence o global (feature 005/FR-SP-001).
func (h *Harness) effectiveSystemPrompt(agentID string) string {
	if agent, ok := h.cfg.Policy.Agents[agentID]; ok && strings.TrimSpace(agent.SystemPrompt) != "" {
		return agent.SystemPrompt
	}
	return h.cfg.SystemPrompt
}

// emitToolResult emite o evento de resultado com o resumo redigido.
func (h *Harness) emitToolResult(ctx context.Context, session *Session, handler Handler, tool Tool, call ToolCall, res ToolResult, latencyMS int64) {
	status := StatusOK
	if res.Denied {
		status = StatusDenied
	} else if res.IsError {
		status = StatusError
	}
	summary := ""
	if !res.Denied {
		summary = h.resultSummary(res)
	}
	handler.ToolResult(ctx, ToolResultEvent{
		CallID:        call.ID,
		Tool:          toolKey(tool),
		Status:        status,
		IsError:       res.IsError,
		Denied:        res.Denied,
		Truncated:     res.Truncated,
		LatencyMS:     latencyMS,
		ResultSummary: summary,
	})
}

func (h *Harness) resultSummary(res ToolResult) string {
	const maxSummary = 4096
	var b strings.Builder
	for _, content := range res.Content {
		if content.Kind == ResultJSON {
			redacted, _ := h.redactor.RedactJSON(content.JSON)
			b.WriteString(redacted)
		} else {
			redacted, _ := h.redactor.RedactString(content.Text)
			b.WriteString(redacted)
		}
		if b.Len() >= maxSummary {
			break
		}
	}
	return b.String()
}

// runTurn executa o turno do agente (CU-HAR-1). resume != nil retoma uma sessão
// que pausou aguardando confirmação (CU-HAR-3).
func (h *Harness) runTurn(ctx context.Context, session *Session, req RunRequest, handler Handler, resume *resumeState) (TurnResult, error) {
	if handler == nil {
		handler = NopHandler{}
	}
	// Middleware do handler (feature 014): decoradores aplicados em ordem.
	for _, mw := range h.cfg.HandlerMiddleware {
		handler = mw(handler)
	}
	turnBudget := budget.New(req.Budget, req.MaxIterations, h.cfg.Clock.Now())

	catalog, err := h.catalog(ctx)
	if err != nil {
		handler.Error(ctx, ErrorEvent{Scope: ErrorMCP, Message: "catálogo de tools indisponível; seguindo sem tools", Retryable: true})
		catalog = nil
	}
	toolDefs := make([]Tool, 0, len(catalog))
	for _, it := range catalog {
		toolDefs = append(toolDefs, it.tool)
	}

	if len(req.Input) > 0 {
		session.Messages = append(session.Messages, Message{
			ID:        newID(),
			Role:      RoleUser,
			Parts:     req.Input,
			CreatedAt: h.cfg.Clock.Now(),
		})
	}
	messages := append(h.memoryMessages(ctx, session, req.Input), h.buildMessages(ctx, session, nil)...)

	result := TurnResult{SessionID: session.ID, Model: session.Model}
	var executions []ToolExecution

	// Structured output (feature 009): compila o schema pedido uma vez por turno.
	var outputValidator *schema.Validator
	if len(req.OutputSchema) > 0 {
		validator, compileErr := schema.Compile(req.OutputSchema)
		if compileErr != nil {
			return result, &ConfigError{Code: "run/output-schema-invalido", Message: "schema de saída inválido", Err: compileErr}
		}
		outputValidator = validator
	}

	finish := func(stop StopReason) (TurnResult, error) {
		result.State = session.State
		result.StopReason = stop
		result.ToolCalls = executions
		result.Usage = session.Usage
		return result, nil
	}

	// Retomada: aplica a decisão pendente antes do próximo passo do modelo.
	if resume != nil {
		it, ok := findTool(catalog, callLookupName(resume.call))
		exec := ToolExecution{CallID: resume.call.ID, Tool: callLookupName(resume.call)}
		start := h.cfg.Clock.Now()
		var res ToolResult
		switch {
		case !ok:
			res = errorResult(resume.call.ID, "tool não está mais disponível")
			exec.Status = StatusError
		case !resume.decision.Approve:
			res = deniedResult(resume.call.ID, "operação negada: "+resume.decision.Reason)
			exec.Status = StatusDenied
		default:
			var callErr error
			res, callErr = h.callTool(ctx, it, resume.call, h.effectiveToolTimeout(session.AgentID, it.tool), nil)
			if callErr != nil {
				// FR-011: falha de transporte vira resultado de erro, nunca resposta fabricada.
				handler.Error(ctx, ErrorEvent{Scope: ErrorMCP, Tool: it.key, Message: "falha ao executar tool", Retryable: true})
				res = errorResult(resume.call.ID, "falha de comunicação com o serviço da tool")
				exec.Status = StatusError
			} else if res.IsError {
				exec.Status = StatusError
			} else {
				exec.Status = StatusOK
			}
		}
		latency := h.cfg.Clock.Now().Sub(start)
		exec.LatencyMS = latency.Milliseconds()
		h.emitToolResult(ctx, session, handler, it.tool, resume.call, res, latency.Milliseconds())
		h.recordToolCall(ctx, session, handler, resume.call, res, exec.Status, latency)
		session.Messages = append(session.Messages, toolMessage(resume.call, res, h.cfg.Clock.Now(), h.cfg.ToolResultMaxBytes))
		messages = append(messages, session.Messages[len(session.Messages)-1])
		executions = append(executions, exec)
	}

	for {
		if ctx.Err() != nil {
			session.UpdatedAt = h.cfg.Clock.Now()
			return finish(StopCancelled)
		}
		if reason, exceeded := turnBudget.Exceeded(h.cfg.Clock.Now()); exceeded {
			return finish(reason)
		}
		if !turnBudget.CanIterate() {
			return finish(StopMaxIterations)
		}
		turnBudget.StartIteration()

		profile, providerKey, err := h.resolveModel(session.Model)
		if err != nil {
			return result, err
		}
		if len(toolDefs) > 0 && !profile.Capabilities.ToolCalling {
			// FR-015: nunca simular suporte a tool-calling quando o perfil não declara.
			return result, &ConfigError{
				Code:    "model/tool-calling-nao-suportado",
				Message: fmt.Sprintf("modelo %q não declara suporte a tool-calling e o turno tem %d tool(s)", session.Model, len(toolDefs)),
			}
		}
		// FR-MM-003/004/005: mídia validada (MIME/teto/fonte) e autorizada pela
		// capacidade do perfil antes de qualquer I/O ao provedor.
		if err := media.Validate(profile.Capabilities, media.FromMessages(session.Messages)); err != nil {
			return result, err
		}

		messageID := newID()
		chatReq := ChatRequest{
			Model:           profile.Model,
			SessionID:       session.ID,
			System:          h.effectiveSystemPrompt(session.AgentID),
			Messages:        messages,
			Tools:           toolDefs,
			Params:          profile.Params,
			MaxOutputTokens: profile.Capabilities.MaxOutputTokens,
			PromptCaching:   profile.Capabilities.PromptCaching,
			OutputSchema:    req.OutputSchema,
		}
		chatStart := h.cfg.Clock.Now()
		modelCtx, modelSpan := h.cfg.Tracer.StartModel(ctx, ModelAttrs{Provider: profile.Provider, Model: profile.Model})
		resp, effectiveModel, err := h.chat(modelCtx, profile, providerKey, chatReq, turnSink{
			ctx:       ctx,
			sessionID: session.ID,
			messageID: messageID,
			handler:   handler,
		})
		modelSpan.End(err)
		latency := h.cfg.Clock.Now().Sub(chatStart)
		if err != nil {
			handler.Error(ctx, ErrorEvent{Scope: ErrorModel, Message: "falha ao chamar o modelo", Retryable: true})
			result.Model = effectiveModel
			result.StopReason = StopError
			result.ToolCalls = executions
			result.Usage = session.Usage
			return result, fmt.Errorf("harness: chamada de modelo: %w", err)
		}

		assistant := resp.Message
		if assistant.ID == "" {
			assistant.ID = messageID
		}
		if assistant.Role == "" {
			assistant.Role = RoleAssistant
		}
		if assistant.CreatedAt.IsZero() {
			assistant.CreatedAt = h.cfg.Clock.Now()
		}
		session.Messages = append(session.Messages, assistant)
		messages = append(messages, assistant)

		usage := h.usageFor(chatReq, resp)
		session.Usage.InputTokens += usage.InputTokens
		session.Usage.OutputTokens += usage.OutputTokens
		session.Usage.CostMicros += usage.CostMicros
		session.Usage.CachedInputTokens += usage.CachedInputTokens
		session.Usage.CacheWriteTokens += usage.CacheWriteTokens
		if usage.Currency != "" {
			session.Usage.Currency = usage.Currency
		}
		session.Usage.Estimated = session.Usage.Estimated || usage.Estimated
		turnBudget.AddUsage(usage)
		h.recordModelCall(ctx, session, handler, profile, resp, usage, latency)
		handler.Usage(ctx, UsageEvent{
			Provider:          profile.Provider,
			Model:             effectiveModel,
			InputTokens:       usage.InputTokens,
			OutputTokens:      usage.OutputTokens,
			Estimated:         usage.Estimated,
			CostMicros:        usage.CostMicros,
			Currency:          usage.Currency,
			CachedInputTokens: usage.CachedInputTokens,
			CacheWriteTokens:  usage.CacheWriteTokens,
		})
		session.UpdatedAt = h.cfg.Clock.Now()
		result.Model = effectiveModel

		if len(resp.ToolCalls) == 0 {
			if outputValidator != nil {
				if err := outputValidator.Validate(json.RawMessage(outputText(assistant.Parts))); err != nil {
					return result, &OutputError{Code: "output/schema-invalido", Message: "saída não corresponde ao schema pedido", Err: err}
				}
			}
			result.Output = assistant.Parts
			return finish(StopCompleted)
		}

		if h.cfg.ParallelTools && len(resp.ToolCalls) > 1 {
			if plans, ok := h.planParallel(catalog, session.AgentID, resp.ToolCalls); ok {
				messages, executions = h.executeParallel(ctx, session, handler, plans, messages, executions)
				continue
			}
		}

		for _, call := range resp.ToolCalls {
			if call.ID == "" {
				call.ID = newID()
			}
			it, ok := findTool(catalog, callLookupName(call))
			if !ok {
				res := errorResult(call.ID, "tool desconhecida: "+call.Name)
				h.emitToolResult(ctx, session, handler, Tool{Name: call.Name, Namespace: call.Namespace}, call, res, 0)
				h.recordToolCall(ctx, session, handler, call, res, StatusError, 0)
				session.Messages = append(session.Messages, toolMessage(call, res, h.cfg.Clock.Now(), h.cfg.ToolResultMaxBytes))
				messages = append(messages, session.Messages[len(session.Messages)-1])
				executions = append(executions, ToolExecution{CallID: call.ID, Tool: callLookupName(call), Status: StatusError})
				continue
			}

			argsRedacted, _ := h.redactor.RedactJSON(call.Args)
			handler.ToolCall(ctx, ToolCallEvent{
				SessionID:    session.ID,
				CallID:       call.ID,
				Tool:         it.key,
				Namespace:    it.tool.Namespace,
				ArgsRedacted: argsRedacted,
				ArgsBytes:    len(call.Args),
			})

			if it.validator != nil {
				if err := it.validator.Validate(call.Args); err != nil {
					res := errorResult(call.ID, "argumentos inválidos: "+err.Error())
					h.emitToolResult(ctx, session, handler, it.tool, call, res, 0)
					h.recordToolCall(ctx, session, handler, call, res, StatusError, 0)
					session.Messages = append(session.Messages, toolMessage(call, res, h.cfg.Clock.Now(), h.cfg.ToolResultMaxBytes))
					messages = append(messages, session.Messages[len(session.Messages)-1])
					executions = append(executions, ToolExecution{CallID: call.ID, Tool: it.key, Status: StatusError})
					continue
				}
			}

			decision := h.authorize(session.AgentID, it.tool)
			if !decision.Allowed {
				res := deniedResult(call.ID, decision.Reason)
				h.emitToolResult(ctx, session, handler, it.tool, call, res, 0)
				h.recordToolCall(ctx, session, handler, call, res, StatusDenied, 0)
				session.Messages = append(session.Messages, toolMessage(call, res, h.cfg.Clock.Now(), h.cfg.ToolResultMaxBytes))
				messages = append(messages, session.Messages[len(session.Messages)-1])
				executions = append(executions, ToolExecution{CallID: call.ID, Tool: it.key, Status: StatusDenied})
				continue
			}

			if decision.RequiresConfirmation {
				reply, confirmErr := h.requestConfirmation(ctx, session, handler, call, it.tool)
				if errors.Is(confirmErr, errConfirmationPending) {
					h.recordToolCall(ctx, session, handler, call, ToolResult{CallID: call.ID}, StatusAwaitingConfirmation, 0)
					session.UpdatedAt = h.cfg.Clock.Now()
					result.State = SessionAwaitingConfirmation
					result.Pending = session.Pending
					result.StopReason = StopCompleted
					result.ToolCalls = executions
					result.Usage = session.Usage
					return result, nil
				}
				if confirmErr != nil {
					return result, confirmErr
				}
				if !reply.Approve {
					res := deniedResult(call.ID, "operação negada: "+reply.Reason)
					h.emitToolResult(ctx, session, handler, it.tool, call, res, 0)
					h.recordToolCall(ctx, session, handler, call, res, StatusDenied, 0)
					session.Messages = append(session.Messages, toolMessage(call, res, h.cfg.Clock.Now(), h.cfg.ToolResultMaxBytes))
					messages = append(messages, session.Messages[len(session.Messages)-1])
					executions = append(executions, ToolExecution{CallID: call.ID, Tool: it.key, Status: StatusDenied})
					continue
				}
			}

			callStart := h.cfg.Clock.Now()
			res, callErr := h.callTool(ctx, it, call, h.effectiveToolTimeout(session.AgentID, it.tool), func(update ProgressUpdate) {
				handler.Progress(ctx, ProgressEvent{
					CallID:   call.ID,
					Tool:     it.key,
					Message:  update.Message,
					Progress: update.Progress,
					Total:    update.Total,
				})
			})
			callLatency := h.cfg.Clock.Now().Sub(callStart)
			status := StatusOK
			if callErr != nil {
				// FR-011: falha de transporte vira resultado de erro para o modelo.
				handler.Error(ctx, ErrorEvent{Scope: ErrorMCP, Tool: it.key, Message: "falha ao executar tool", Retryable: true})
				res = errorResult(call.ID, "falha de comunicação com o serviço da tool")
				status = StatusError
			} else if res.IsError {
				status = StatusError
			}
			h.emitToolResult(ctx, session, handler, it.tool, call, res, callLatency.Milliseconds())
			h.recordToolCall(ctx, session, handler, call, res, status, callLatency)
			session.Messages = append(session.Messages, toolMessage(call, res, h.cfg.Clock.Now(), h.cfg.ToolResultMaxBytes))
			messages = append(messages, session.Messages[len(session.Messages)-1])
			executions = append(executions, ToolExecution{
				CallID:    call.ID,
				Tool:      it.key,
				Status:    status,
				LatencyMS: callLatency.Milliseconds(),
			})
		}
	}
}
