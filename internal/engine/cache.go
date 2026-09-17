package engine

import (
	"context"

	"github.com/rmarquespaixao-eng/ia-harness/internal/engine/semcache"
	"github.com/rmarquespaixao-eng/ia-harness/internal/engine/telemetry"
)

// cacheAttempt é a consulta preparada do cache semântico (feature 020): a
// chave/vetor do contexto e o volume do request (para estimar a economia).
type cacheAttempt struct {
	sessionID    string
	query        CacheQuery
	requestBytes int
}

// prepareCache avalia a elegibilidade e gera o vetor do contexto. Devolve nil
// quando o cache está desligado, o turno não é elegível ou o embedding falha
// (falha vira miss, nunca derruba o turno — FR-SC-007).
func (h *Harness) prepareCache(ctx context.Context, session *Session, req ChatRequest, tools []Tool) *cacheAttempt {
	if !semcache.Eligible(req, tools, h.cfg.Cache) || h.cfg.CacheStore == nil || h.cfg.Embedder == nil {
		return nil
	}
	vectors, err := h.cfg.Embedder.Embed(ctx, []string{semcache.CanonicalText(req)})
	if err != nil || len(vectors) == 0 || !semcache.Valid(vectors[0]) {
		h.cfg.Logger.Warn("cache semântico: embedding indisponível; seguindo sem cache",
			"session_id", session.ID, "error", errText(err))
		return nil
	}
	return &cacheAttempt{
		sessionID: session.ID,
		query: CacheQuery{
			UserID: session.UserID,
			Model:  req.Model,
			Key:    semcache.Key(req, req.Model),
			Vector: vectors[0],
		},
		requestBytes: requestBytes(req),
	}
}

// lookupCache procura a resposta no store; hit devolve a resposta e o metadado.
// Miss e falha são silenciosos (com evento/WARN) e o provedor é chamado.
func (h *Harness) lookupCache(ctx context.Context, handler Handler, attempt *cacheAttempt) (*ChatResponse, *CacheInfo, bool) {
	entry, ok, err := h.cfg.CacheStore.Lookup(ctx, attempt.query)
	if err != nil {
		h.cfg.Logger.Warn("cache semântico: lookup falhou; seguindo sem cache",
			"session_id", attempt.sessionID, "error", err.Error())
		return nil, nil, false
	}
	if !ok {
		h.emitCache(ctx, handler, CacheEvent{SessionID: attempt.sessionID, Model: attempt.query.Model, Hit: false, Key: attempt.query.Key})
		return nil, nil, false
	}
	info := &CacheInfo{Key: attempt.query.Key, Score: entry.Score, Hit: true}
	h.emitCache(ctx, handler, CacheEvent{
		SessionID:   attempt.sessionID,
		Model:       attempt.query.Model,
		Hit:         true,
		Score:       entry.Score,
		Key:         attempt.query.Key,
		SavedMicros: h.estimatedSavedMicros(attempt, entry.Response),
	})
	return &entry.Response, info, true
}

// saveCache armazena a resposta de um miss elegível (sem tool calls).
func (h *Harness) saveCache(ctx context.Context, handler Handler, session *Session, attempt *cacheAttempt, resp ChatResponse) {
	if len(resp.ToolCalls) > 0 {
		return
	}
	entry := CacheEntry{Response: resp, CreatedAt: h.cfg.Clock.Now()}
	if err := h.cfg.CacheStore.Store(ctx, attempt.query, entry); err != nil {
		h.cfg.Logger.Warn("cache semântico: store falhou",
			"session_id", session.ID, "error", err.Error())
	}
}

// estimatedSavedMicros estima o custo evitado por um hit (premissa do host).
func (h *Harness) estimatedSavedMicros(attempt *cacheAttempt, resp ChatResponse) int64 {
	if h.cfg.Pricing == (Pricing{}) {
		return 0
	}
	return telemetry.UsageEstimated(h.cfg.Pricing, attempt.requestBytes, messageTextBytes(resp.Message)).CostMicros
}

// emitCache publica o evento quando o Handler implementa CacheHandler.
func (h *Harness) emitCache(ctx context.Context, handler Handler, ev CacheEvent) {
	if handler == nil {
		return
	}
	sink, ok := handler.(CacheHandler)
	if !ok {
		return
	}
	sink.Cache(ctx, ev)
}

// errText devolve a mensagem do erro ou vazio (evita logar "nil").
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
