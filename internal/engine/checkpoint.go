package engine

import (
	"context"
	"fmt"
)

// beginCheckpoint abre o checkpoint durável do turno (feature 021) quando
// Config.Durable está ligado. Retomada de um checkpoint running não é
// reaberta.
func (h *Harness) beginCheckpoint(ctx context.Context, session *Session, handler Handler) error {
	if !h.cfg.Durable {
		return nil
	}
	if session.Checkpoint != nil && session.Checkpoint.Status == CheckpointRunning {
		return nil
	}
	session.Checkpoint = &TurnCheckpoint{
		TurnID: newID(),
		Status: CheckpointRunning,
	}
	return h.saveCheckpoint(ctx, session, handler)
}

// saveCheckpoint persiste o snapshot com o checkpoint atual e emite o evento.
// Falha do SessionStore aborta o turno: com durabilidade ligada o checkpoint
// não é best-effort (FR-DU-001).
func (h *Harness) saveCheckpoint(ctx context.Context, session *Session, handler Handler) error {
	if !h.cfg.Durable || session.Checkpoint == nil {
		return nil
	}
	session.Checkpoint.UpdatedAt = h.cfg.Clock.Now()
	if err := h.cfg.Sessions.Save(context.WithoutCancel(ctx), session); err != nil {
		return fmt.Errorf("harness: salvar checkpoint: %w", err)
	}
	h.emitCheckpoint(ctx, handler, session)
	return nil
}

// checkpointStep avança o passo durável e limpa o write-ahead pendente.
func (h *Harness) checkpointStep(ctx context.Context, session *Session, handler Handler) error {
	if !h.cfg.Durable || session.Checkpoint == nil {
		return nil
	}
	session.Checkpoint.Step++
	session.Checkpoint.PendingCall = nil
	return h.saveCheckpoint(ctx, session, handler)
}

// checkpointToolStart grava o write-ahead antes de executar a tool.
func (h *Harness) checkpointToolStart(ctx context.Context, session *Session, handler Handler, it installedTool, call ToolCall) error {
	if !h.cfg.Durable || session.Checkpoint == nil {
		return nil
	}
	argsRedacted, _ := h.redactor.RedactJSON(call.Args)
	session.Checkpoint.PendingCall = &PendingCall{
		CallID:       call.ID,
		Tool:         it.key,
		ArgsRedacted: argsRedacted,
		Idempotent:   it.tool.Idempotent,
	}
	return h.saveCheckpoint(ctx, session, handler)
}

// finishCheckpoint encerra o checkpoint conforme o desfecho do turno: pausa por
// confirmação vira awaiting_confirmation; cancelamento mantém running (o turno
// é retomável); os demais marcam completed.
func (h *Harness) finishCheckpoint(session *Session, result TurnResult) {
	if !h.cfg.Durable || session.Checkpoint == nil {
		return
	}
	session.Checkpoint.PendingCall = nil
	switch {
	case session.Pending != nil:
		session.Checkpoint.Status = CheckpointAwaitingConfirm
	case result.StopReason == StopCancelled:
		// mantém running para permitir a retomada
	default:
		session.Checkpoint.Status = CheckpointCompleted
	}
	session.Checkpoint.UpdatedAt = h.cfg.Clock.Now()
}

// reconcileCheckpoint resolve um write-ahead interrompido na retomada
// (FR-DU-004/005): se já há resultado para o call_id, apenas limpa; se não,
// reexecuta apenas tool idempotente — caso contrário injeta resultado ambíguo
// que impede a repetição do efeito.
func (h *Harness) reconcileCheckpoint(ctx context.Context, session *Session, handler Handler, catalog []installedTool) error {
	if !h.cfg.Durable || session.Checkpoint == nil || session.Checkpoint.PendingCall == nil {
		return nil
	}
	pending := session.Checkpoint.PendingCall
	if hasToolResult(session, pending.CallID) {
		session.Checkpoint.PendingCall = nil
		session.Checkpoint.Step++
		return h.saveCheckpoint(ctx, session, handler)
	}

	res := errorResult(pending.CallID, "resultado ambíguo após interrupção: operação não reexecutada")
	if it, ok := findTool(catalog, pending.Tool); ok && pending.Idempotent {
		if call, found := findToolCall(session, pending.CallID); found {
			callRes, callErr := h.callTool(ctx, it, call, h.effectiveToolTimeout(session.AgentID, it.tool), nil)
			if callErr != nil {
				callRes = errorResult(pending.CallID, "falha de comunicação com o serviço da tool")
			}
			res = callRes
			status := StatusOK
			if res.IsError || callErr != nil {
				status = StatusError
			}
			h.emitToolResult(ctx, session, handler, it.tool, call, res, 0)
			h.recordToolCall(ctx, session, handler, call, res, status, 0)
		}
	}
	session.Messages = append(session.Messages, toolMessage(ToolCall{ID: pending.CallID}, res, h.cfg.Clock.Now(), h.cfg.ToolResultMaxBytes))
	session.Checkpoint.PendingCall = nil
	session.Checkpoint.Step++
	return h.saveCheckpoint(ctx, session, handler)
}

// emitCheckpoint publica o evento quando o Handler implementa a interface
// opcional CheckpointHandler (feature 021).
func (h *Harness) emitCheckpoint(ctx context.Context, handler Handler, session *Session) {
	if handler == nil || session.Checkpoint == nil {
		return
	}
	sink, ok := handler.(CheckpointHandler)
	if !ok {
		return
	}
	sink.Checkpoint(ctx, CheckpointEvent{
		SessionID: session.ID,
		TurnID:    session.Checkpoint.TurnID,
		Status:    string(session.Checkpoint.Status),
		Step:      session.Checkpoint.Step,
	})
}

// hasToolResult informa se há resultado de tool para o call_id no histórico.
func hasToolResult(session *Session, callID string) bool {
	return findToolResult(session, callID) != nil
}

// findToolResult devolve o resultado de um call_id no histórico (nil se ausente).
func findToolResult(session *Session, callID string) *ToolResult {
	for i := len(session.Messages) - 1; i >= 0; i-- {
		for _, part := range session.Messages[i].Parts {
			if part.Kind == PartToolResult && part.Result != nil && part.Result.CallID == callID {
				return part.Result
			}
		}
	}
	return nil
}

// findToolCall localiza a chamada pelo id no histórico.
func findToolCall(session *Session, callID string) (ToolCall, bool) {
	for i := len(session.Messages) - 1; i >= 0; i-- {
		for _, part := range session.Messages[i].Parts {
			if part.Kind == PartToolCall && part.Call != nil && part.Call.ID == callID {
				return *part.Call, true
			}
		}
	}
	return ToolCall{}, false
}
