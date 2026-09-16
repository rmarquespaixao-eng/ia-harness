package engine

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"time"

	gen "rmarquespaixao/ia-harness/contracts/gen"
	"rmarquespaixao/ia-harness/internal/engine/telemetry"
	"rmarquespaixao/ia-harness/internal/platform/trace"
)

// usageFor decide o Usage de uma chamada de modelo (FR-025/ADR 0004): uso
// reportado pelo provedor tem precedência; sem reporte, estima os tokens pelo
// volume de bytes do request (mensagens+tools) e do texto da resposta × premissa
// de Pricing, com o rótulo estimated=true. Premissa zerada não produz custo.
func (h *Harness) usageFor(req ChatRequest, resp ChatResponse) Usage {
	if resp.Usage.InputTokens > 0 || resp.Usage.OutputTokens > 0 {
		return telemetry.UsageReportedCached(h.cfg.Pricing,
			resp.Usage.InputTokens, resp.Usage.OutputTokens,
			resp.Usage.CachedInputTokens, resp.Usage.CacheWriteTokens)
	}
	if h.cfg.Pricing == (Pricing{}) {
		return Usage{Estimated: true}
	}
	return telemetry.UsageEstimated(h.cfg.Pricing, requestBytes(req), messageTextBytes(resp.Message))
}

// requestPayload é o recorte do request medido pela estimativa: apenas as
// mensagens e as tools enviadas ao modelo (bytes originais, antes da redação).
type requestPayload struct {
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
}

// requestBytes mede o JSON de mensagens+tools do request; falha de marshal
// (impossível com os tipos canônicos) degrada para volume zero.
func requestBytes(req ChatRequest) int {
	raw, err := json.Marshal(requestPayload{Messages: req.Messages, Tools: req.Tools})
	if err != nil {
		return 0
	}
	return len(raw)
}

// messageTextBytes soma os bytes das partes de texto de uma mensagem.
func messageTextBytes(msg Message) int {
	total := 0
	for _, part := range msg.Parts {
		if part.Kind == PartText {
			total += len(part.Text)
		}
	}
	return total
}

// recordModelCall registra a chamada de modelo na trilha de auditoria (US5):
// um AuditEvent kind=model_call com provedor/modelo, tokens/custo/estimated,
// latência, status ok e o trace_id do contexto. Nunca carrega payload da
// mensagem. Falha do sink vira ErrorEvent e não derruba o turno (FR-023/FR-024).
func (h *Harness) recordModelCall(ctx context.Context, session *Session, handler Handler, profile ModelProfile, resp ChatResponse, usage Usage, latency time.Duration) {
	handler = handlerOrNop(handler)

	event := AuditEvent{
		Id:           newID(),
		Kind:         gen.AuditEventKindModelCall,
		TraceId:      traceID(ctx),
		UserId:       session.UserID,
		SessionId:    session.ID,
		AgentId:      session.AgentID,
		Provider:     optString(profile.Provider),
		Model:        optString(effectiveModel(profile, resp)),
		Status:       gen.AuditEventStatusOk,
		InputTokens:  intPtr(usage.InputTokens),
		OutputTokens: intPtr(usage.OutputTokens),
		Estimated:    usage.Estimated,
		CostMicros:   intPtr(usage.CostMicros),
		Currency:     optString(usage.Currency),
		// Prompt caching (feature 006).
		CachedInputTokens: intPtr(usage.CachedInputTokens),
		CacheWriteTokens:  intPtr(usage.CacheWriteTokens),
		LatencyMs:         latencyMillis(latency),
		OccurredAt:        h.cfg.Clock.Now(),
	}

	if err := h.cfg.Audit.Emit(ctx, event); err != nil {
		h.reportAuditFailure(ctx, err)
		handler.Error(ctx, ErrorEvent{Scope: ErrorModel, Message: "falha ao registrar auditoria", Retryable: false})
	}
}

// recordToolCall registra a execução/negativa de tool na trilha (US5): um
// AuditEvent kind=tool_call com tool, call_id, status, latência e payloads
// redigidos/truncados. Status denied sai sem payload (FR-024). Falha do sink
// vira ErrorEvent e não derruba o turno.
func (h *Harness) recordToolCall(ctx context.Context, session *Session, handler Handler, call ToolCall, res ToolResult, status string, latency time.Duration) {
	handler = handlerOrNop(handler)

	event := AuditEvent{
		Id:         newID(),
		Kind:       gen.AuditEventKindToolCall,
		TraceId:    traceID(ctx),
		UserId:     session.UserID,
		SessionId:  session.ID,
		AgentId:    session.AgentID,
		Tool:       optString(callLookupName(call)),
		ToolCallId: optString(call.ID),
		Status:     gen.AuditEventStatus(status),
		Estimated:  false,
		LatencyMs:  latencyMillis(latency),
		OccurredAt: h.cfg.Clock.Now(),
	}

	if status != StatusDenied {
		event.ArgsRedacted = redactOptional(h.redactor, call.Args)
		event.ResultRedacted = optString(h.resultSummary(res))
	}

	if err := h.cfg.Audit.Emit(ctx, event); err != nil {
		h.reportAuditFailure(ctx, err)
		handler.Error(ctx, ErrorEvent{
			Scope:     ErrorTool,
			Tool:      callLookupName(call),
			Message:   "falha ao registrar auditoria",
			Retryable: false,
		})
	}
}

// handlerOrNop devolve o handler informado ou um handler sem efeito (retomada
// sem host conectado).
func handlerOrNop(handler Handler) Handler {
	if handler == nil {
		return NopHandler{}
	}
	return handler
}

// traceID extrai o trace_id do contexto; ausente devolve string vazia.
func traceID(ctx context.Context) string {
	id, _ := trace.IDFromContext(ctx)
	return id
}

// effectiveModel prefere o modelo devolvido pelo adaptador (modelo efetivo) e
// recai no modelo configurado do perfil quando ausente.
func effectiveModel(profile ModelProfile, resp ChatResponse) string {
	if resp.Model != "" {
		return resp.Model
	}
	return profile.Model
}

// latencyMillis converte a latência para milissegundos inteiros, nunca negativa
// (o contrato exige latency_ms >= 0).
func latencyMillis(d time.Duration) int {
	ms := d.Milliseconds()
	if ms < 0 {
		return 0
	}
	if ms > int64(math.MaxInt) {
		return math.MaxInt
	}
	return int(ms)
}

// intPtr devolve um ponteiro para o inteiro da trilha, zerando negativos e
// saturando em MaxInt (o contrato exige inteiros >= 0).
func intPtr(v int64) *int {
	if v < 0 {
		v = 0
	}
	if v > int64(math.MaxInt) {
		v = int64(math.MaxInt)
	}
	n := int(v)
	return &n
}

// optString devolve o ponteiro do valor ou nil quando vazio (o schema exige
// currency com exatamente 3 caracteres, então string vazia não é omitida).
func optString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// redactOptional aplica a redação/truncamento e devolve nil para resultado vazio.
func redactOptional(redactor *telemetry.Redactor, args json.RawMessage) *string {
	redacted, _ := redactor.RedactJSON(args)
	if redacted == "" {
		return nil
	}
	return &redacted
}

// reportAuditFailure registra a causa da falha do sink no logger do host,
// estruturada e com o trace_id do contexto (FR-023; R13).
func (h *Harness) reportAuditFailure(ctx context.Context, err error) {
	if h.cfg.Logger == nil {
		return
	}
	h.cfg.Logger.ErrorContext(ctx, "falha ao registrar auditoria",
		slog.String("trace_id", traceID(ctx)),
		slog.String("error", err.Error()),
	)
}
