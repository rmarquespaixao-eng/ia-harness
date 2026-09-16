package engine

import (
	"context"
	"fmt"

	"github.com/rmarquespaixao-eng/ia-harness/internal/engine/telemetry"
)

// Harness é o núcleo embutível do agente: compõe as portas injetadas pelo host
// (DI — ADR 0006) e executa turnos. Create por New; seguro para múltiplas
// sessões concorrentes (uma execução de turno por sessão — D-10).
type Harness struct {
	cfg      Config
	redactor *telemetry.Redactor
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
	return &Harness{cfg: cfg, redactor: telemetry.NewRedactor(cfg.Redaction)}, nil
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
	result, runErr := h.runTurnTraced(ctx, session, req, handler, nil)
	if runErr != nil {
		return result, runErr
	}
	// O save sobrevive ao cancelamento do turno: a sessão precisa ficar no ponto
	// consistente mesmo quando Run retorna StopCancelled (FR-010/CU-HAR-1 6a).
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
		ID:        newID(),
		UserID:    req.UserID,
		AgentID:   req.AgentID,
		Model:     model,
		State:     SessionActive,
		CreatedAt: now,
		UpdatedAt: now,
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
