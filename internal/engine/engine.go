package engine

import (
	"context"
	"fmt"
	"sync"

	"github.com/rmarquespaixao-eng/ia-harness/internal/engine/agents"
	"github.com/rmarquespaixao-eng/ia-harness/internal/engine/ratelimit"
	"github.com/rmarquespaixao-eng/ia-harness/internal/engine/telemetry"
)

// Harness é o núcleo embutível do agente: compõe as portas injetadas pelo host
// (DI — ADR 0006) e executa turnos. Create por New; seguro para múltiplas
// sessões concorrentes (uma execução de turno por sessão — D-10).
type Harness struct {
	cfg      Config
	redactor *telemetry.Redactor

	// limiters implementam o token bucket por provider (feature 019); o mutex
	// protege o estado dos buckets compartilhados entre turnos concorrentes.
	limiters       map[string]*ratelimit.Bucket
	defaultLimiter *ratelimit.Bucket
	limiterMu      sync.Mutex
}

// New valida a configuração (falha rápida, erro nomeado) e monta o harness.
// Não abre rede nem resolve credenciais: adaptadores chegam prontos por DI.
// Providers vazio, adapter nulo e portas obrigatórias ausentes são erro;
// Tools vazio é válido (agente sem tools).
func New(cfg Config) (*Harness, error) {
	applyDefaults(&cfg)
	if err := validate(&cfg); err != nil {
		return nil, err
	}
	h := &Harness{
		cfg:      cfg,
		redactor: telemetry.NewRedactor(cfg.Redaction),
		limiters: make(map[string]*ratelimit.Bucket, len(cfg.RateLimits)),
	}
	for key, rl := range cfg.RateLimits {
		h.limiters[key] = ratelimit.New(rl.RequestsPerMinute, rl.Burst)
	}
	h.defaultLimiter = ratelimit.New(cfg.DefaultRateLimit.RequestsPerMinute, cfg.DefaultRateLimit.Burst)
	if len(cfg.Agents) > 0 {
		// A fonte sintética de delegação entra no catálogo depois do middleware
		// (feature 022); o motor é o próprio AgentRunner.
		h.cfg.Tools = append(h.cfg.Tools, agents.NewSource(h, cfg.Agents, cfg.MaxAgentDepth))
	}
	return h, nil
}

// Run executa um turno do agente de ponta a ponta e bloqueia até concluir
// (resposta final, pausa por confirmação ou erro). Eventos são entregues
// incrementalmente pelo Handler durante a execução.
func (h *Harness) Run(ctx context.Context, req RunRequest, handler Handler) (TurnResult, error) {
	if req.UserID == "" {
		return TurnResult{}, &ConfigError{Code: "run/user-obrigatorio", Message: "UserID é obrigatório"}
	}
	session, err := h.loadOrCreateSession(ctx, req)
	if err != nil {
		return TurnResult{}, err
	}
	if session.State == SessionAwaitingConfirmation {
		return TurnResult{}, &ConfigError{
			Code:    "run/confirmacao-pendente",
			Message: "sessão aguardando confirmação; use ResolveConfirmation",
		}
	}
	// Retomada durável (feature 021): checkpoint "running" continua o turno do
	// histórico persistido; a entrada nova não é re-anexada.
	resumed := h.cfg.Durable && session.Checkpoint != nil && session.Checkpoint.Status == CheckpointRunning
	if resumed {
		req.Input = nil
	}
	result, runErr := h.runTurnTraced(ctx, session, req, handler, nil)
	result.Resumed = resumed
	if runErr != nil {
		return result, runErr
	}
	// O save sobrevive ao cancelamento do turno: a sessão precisa ficar no ponto
	// consistente mesmo quando Run retorna StopCancelled (FR-010/CU-HAR-1 6a).
	if h.cfg.Durable {
		h.finishCheckpoint(session, result)
	}
	if err := h.cfg.Sessions.Save(context.WithoutCancel(ctx), session); err != nil {
		return result, fmt.Errorf("harness: salvar sessão: %w", err)
	}
	return result, nil
}

// runTurnTraced envolve runTurn num span de agente (feature 011).
func (h *Harness) runTurnTraced(ctx context.Context, session *Session, req RunRequest, handler Handler, resume *resumeState) (TurnResult, error) {
	ctx, span := h.cfg.Tracer.StartTurn(ctx, TurnAttrs{
		SessionID: session.ID,
		UserID:    session.UserID,
		AgentID:   session.AgentID,
		Model:     session.Model,
	})
	result, err := h.runTurn(ctx, session, req, handler, resume)
	span.End(err)
	return result, err
}

// ResolveConfirmation retoma uma sessão em awaiting_confirmation aplicando a
// decisão do host e seguindo o turno de onde pausou.
func (h *Harness) ResolveConfirmation(ctx context.Context, sessionID, callID string, d Decision, handler Handler) (TurnResult, error) {
	if sessionID == "" || callID == "" {
		return TurnResult{}, &ConfigError{Code: "confirmacao/parametros", Message: "sessionID e callID são obrigatórios"}
	}
	session, err := h.cfg.Sessions.Load(ctx, sessionID)
	if err != nil {
		return TurnResult{}, fmt.Errorf("harness: carregar sessão %q: %w", sessionID, err)
	}
	if session.State != SessionAwaitingConfirmation || session.Pending == nil || session.Pending.CallID != callID {
		return TurnResult{}, &ConfigError{
			Code:    "confirmacao/pendencia-inexistente",
			Message: "não há confirmação pendente com esse call_id",
		}
	}
	call, ok := findPendingCall(session, callID)
	if !ok {
		return TurnResult{}, &ConfigError{
			Code:    "confirmacao/chamada-nao-encontrada",
			Message: "chamada pendente não encontrada no histórico da sessão",
		}
	}
	session.Pending = nil
	session.State = SessionActive
	if h.cfg.Durable && session.Checkpoint != nil {
		session.Checkpoint.Status = CheckpointRunning
		session.Checkpoint.PendingCall = nil
	}
	req := RunRequest{SessionID: sessionID, UserID: session.UserID, AgentID: session.AgentID}
	result, runErr := h.runTurnTraced(ctx, session, req, handler, &resumeState{call: call, decision: d})
	if runErr != nil {
		return result, runErr
	}
	if err := h.cfg.Sessions.Save(context.WithoutCancel(ctx), session); err != nil {
		return result, fmt.Errorf("harness: salvar sessão: %w", err)
	}
	return result, nil
}

// Session carrega o estado atual da sessão pelo SessionStore injetado.
func (h *Harness) Session(ctx context.Context, sessionID string) (*Session, error) {
	if sessionID == "" {
		return nil, &ConfigError{Code: "session/id-vazio", Message: "sessionID vazio"}
	}
	session, err := h.cfg.Sessions.Load(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("harness: carregar sessão %q: %w", sessionID, err)
	}
	return session, nil
}

// Close libera recursos dos adaptadores injetados. Idempotente: o primeiro erro
// é retornado e os demais adaptadores são fechados mesmo assim.
func (h *Harness) Close() error {
	var firstErr error
	closeAll := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for _, p := range h.cfg.Providers {
		if p == nil {
			continue
		}
		closeAll(p.Close())
	}
	for _, t := range h.cfg.Tools {
		if t == nil {
			continue
		}
		closeAll(t.Close())
	}
	return firstErr
}

// loadOrCreateSession carrega a sessão existente (exigindo o mesmo dono) ou cria
// uma nova com o modelo efetivo.
func (h *Harness) loadOrCreateSession(ctx context.Context, req RunRequest) (*Session, error) {
	if req.SessionID != "" {
		session, err := h.cfg.Sessions.Load(ctx, req.SessionID)
		if err != nil {
			return nil, fmt.Errorf("harness: carregar sessão %q: %w", req.SessionID, err)
		}
		if session.UserID != req.UserID {
			return nil, &ConfigError{Code: "run/sessao-de-outro-usuario", Message: "sessão pertence a outro usuário (isolamento — FR-022)"}
		}
		if req.AgentID != "" {
			session.AgentID = req.AgentID
		}
		if req.Model != "" {
			session.Model = req.Model
		}
		if req.ParentSessionID != "" {
			session.ParentSessionID = req.ParentSessionID
		}
		return session, nil
	}

	model := req.Model
	if model == "" {
		model = h.cfg.DefaultModel
	}
	if model == "" {
		return nil, &ConfigError{Code: "run/modelo-ausente", Message: "informe Model no RunRequest ou DefaultModel na Config"}
	}
	if _, ok := h.cfg.Models[model]; !ok {
		return nil, &ConfigError{Code: "run/modelo-desconhecido", Message: fmt.Sprintf("modelo %q não configurado", model)}
	}
	now := h.cfg.Clock.Now()
	return &Session{
		ID:              newID(),
		UserID:          req.UserID,
		AgentID:         req.AgentID,
		Model:           model,
		State:           SessionActive,
		CreatedAt:       now,
		UpdatedAt:       now,
		ParentSessionID: req.ParentSessionID,
	}, nil
}

// findPendingCall localiza a tool call pendente no histórico da sessão.
func findPendingCall(session *Session, callID string) (ToolCall, bool) {
	for i := len(session.Messages) - 1; i >= 0; i-- {
		message := session.Messages[i]
		for _, part := range message.Parts {
			if part.Kind == PartToolCall && part.Call != nil && part.Call.ID == callID {
				return *part.Call, true
			}
		}
	}
	return ToolCall{}, false
}

// effectivePolicy devolve a política do agente, aplicando a sobreposição
// declarada em AgentSpec.Policy (feature 022) quando presente.
func (h *Harness) effectivePolicy(agentID string) PolicyConfig {
	spec, ok := h.cfg.Agents[agentID]
	if !ok || spec.Policy == nil {
		return h.cfg.Policy
	}
	merged := h.cfg.Policy
	agents := make(map[string]AgentPolicy, len(merged.Agents)+1)
	for key, value := range merged.Agents {
		agents[key] = value
	}
	agents[agentID] = *spec.Policy
	merged.Agents = agents
	return merged
}

// RunSubAgent executa o sub-turno de um agente nomeado (feature 022) em sessão
// própria, herdando o usuário/contexto do pai. Implementa core.AgentRunner.
func (h *Harness) RunSubAgent(ctx context.Context, req SubAgentRequest) (SubAgentResult, error) {
	spec, ok := h.cfg.Agents[req.AgentID]
	if !ok {
		return SubAgentResult{}, &ConfigError{Code: "agents/agente-desconhecido", Message: fmt.Sprintf("agente %q não configurado", req.AgentID)}
	}
	cc := agents.FromCallContext(ctx)
	handler := handlerOrNop(cc.Handler)
	h.emitSubAgent(ctx, handler, req.ParentSessionID, "", req.AgentID, cc.Depth, "running")

	model := spec.Model
	if model == "" {
		model = h.cfg.DefaultModel
	}
	result, err := h.Run(ctx, RunRequest{
		UserID:          req.UserID,
		AgentID:         req.AgentID,
		Model:           model,
		Input:           req.Input,
		MaxIterations:   spec.MaxIterations,
		Budget:          req.Budget,
		ParentSessionID: req.ParentSessionID,
	}, handler)

	status := "completed"
	if err != nil {
		status = "error"
	}
	h.emitSubAgent(ctx, handler, req.ParentSessionID, result.SessionID, req.AgentID, cc.Depth, status)

	return SubAgentResult{
		SessionID:  result.SessionID,
		Output:     result.Output,
		State:      result.State,
		StopReason: result.StopReason,
	}, err
}

// emitSubAgent publica o evento de delegação quando o Handler implementa a
// interface opcional SubAgentHandler (feature 022).
func (h *Harness) emitSubAgent(ctx context.Context, handler Handler, parent, child, agent string, depth int, status string) {
	if handler == nil {
		return
	}
	sink, ok := handler.(SubAgentHandler)
	if !ok {
		return
	}
	sink.SubAgent(ctx, SubAgentEvent{
		ParentSessionID: parent,
		ChildSessionID:  child,
		Agent:           agent,
		Depth:           depth,
		Status:          status,
	})
}
