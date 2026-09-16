package engine

import (
	"context"
	"errors"
	"fmt"

	"rmarquespaixao/ia-harness/internal/engine/policy"
)

// errConfirmationPending sinaliza que o turno pausou aguardando decisão humana.
var errConfirmationPending = errors.New("harness: confirmação pendente")

// authorize consulta a política do agente no motor puro de policy
// (default deny, deny_list, allowlist glob, read_only e confirmação).
func (h *Harness) authorize(agentID string, tool Tool) policy.Decision {
	return policy.Evaluate(h.cfg.Policy, agentID, tool)
}

// requestConfirmation pede a decisão humana e, em falha do host, persiste a
// pendência na sessão (estado awaiting_confirmation + PendingConfirmation com
// args redigidos e RequestedAt do Clock) para retomada via ResolveConfirmation.
// O turno pausa devolvendo errConfirmationPending.
func (h *Harness) requestConfirmation(ctx context.Context, session *Session, handler Handler, call ToolCall, tool Tool) (Decision, error) {
	argsRedacted, _ := h.redactor.RedactJSON(call.Args)
	reason := h.authorize(session.AgentID, tool).Reason
	if reason == "" {
		reason = "tool exige confirmação"
	}
	decision, err := handler.Confirmation(ctx, ConfirmationEvent{
		SessionID:    session.ID,
		CallID:       call.ID,
		Tool:         policy.ToolKey(tool),
		ArgsRedacted: argsRedacted,
		Reason:       reason,
	})
	if err != nil {
		session.State = SessionAwaitingConfirmation
		session.Pending = &PendingConfirmation{
			CallID:       call.ID,
			ToolName:     policy.ToolKey(tool),
			ArgsRedacted: argsRedacted,
			Reason:       reason,
			RequestedAt:  h.cfg.Clock.Now(),
		}
		if saveErr := h.cfg.Sessions.Save(ctx, session); saveErr != nil {
			return Decision{}, fmt.Errorf("harness: persistir confirmação pendente: %w", saveErr)
		}
		return Decision{}, fmt.Errorf("%w: %v", errConfirmationPending, err)
	}
	return decision, nil
}
