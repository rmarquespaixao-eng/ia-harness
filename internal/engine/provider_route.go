package engine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/rmarquespaixao-eng/ia-harness/internal/engine/media"
	"github.com/rmarquespaixao-eng/ia-harness/internal/engine/ratelimit"
)

// resolveModel resolve o alias (vazio ⇒ DefaultModel) para o perfil e a chave
// do provider injetado. Alias ou provider ausente falha explícito e nomeado
// (*ConfigError) — trocar de modelo é configuração, nunca código do host
// (FR-012/RN-1; CU-HAR-2 fluxo 2a).
func (h *Harness) resolveModel(alias string) (ModelProfile, string, error) {
	if alias == "" {
		alias = h.cfg.DefaultModel
	}
	profile, ok := h.cfg.Models[alias]
	if !ok {
		return ModelProfile{}, "", &ConfigError{
			Code:    "model/alias-desconhecido",
			Message: fmt.Sprintf("modelo %q não configurado", alias),
		}
	}
	if _, ok := h.cfg.Providers[profile.Provider]; !ok {
		return ModelProfile{}, "", &ConfigError{
			Code:    "model/provider-desconhecido",
			Message: fmt.Sprintf("provider %q do modelo %q não injetado", profile.Provider, alias),
		}
	}
	return profile, profile.Provider, nil
}

// aliasFor identifica o alias configurado correspondente a profile (mesmo
// provider+modelo). O modelo efetivo devolvido por chat é o alias — nunca a
// chave do provider (library-api.md invariante 4) — para TurnResult.Model e
// auditoria. A varredura é ordenada para ser determinística.
func (h *Harness) aliasFor(profile ModelProfile) string {
	aliases := make([]string, 0, len(h.cfg.Models))
	for alias := range h.cfg.Models {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		candidate := h.cfg.Models[alias]
		if candidate.Provider == profile.Provider && candidate.Model == profile.Model {
			return alias
		}
	}
	return profile.Model
}

// noopSink é um StreamSink sem efeito (chamadas internas sem host conectado).
type noopSink struct{}

func (noopSink) Text(string)                         {}
func (noopSink) Reasoning(string)                    {}
func (noopSink) ToolCallArgs(string, string, string) {}

// chat executa a chamada de modelo com fallback por chamada (R12/FR-013):
// tenta o perfil primário e, em erro, percorre profile.Fallbacks na ordem,
// resolvendo cada alias e parando no primeiro sucesso. Nenhuma tool é repetida
// aqui: o loop controla as tools e a conversa já executada é reaproveitada.
// Erro de configuração em um fallback (alias inválido) não derruba a cadeia —
// fica registrado na falha final, que agrega todas as tentativas e preserva o
// último erro em %w. Devolve o alias efetivo (nunca a chave do provider); em
// erro total devolve o alias primário. O mesmo sink é repassado a todas as
// tentativas: deltas já emitidos por uma tentativa que falhou permanecem
// entregues ao host (não há rollback de stream). O rate limit (feature 019) é
// aplicado por provider antes de cada tentativa.
func (h *Harness) chat(ctx context.Context, profile ModelProfile, providerKey string, req ChatRequest, sink StreamSink, handler Handler) (ChatResponse, string, error) {
	if sink == nil {
		sink = noopSink{}
	}
	primaryAlias := h.aliasFor(profile)
	failures := make([]error, 0, 1+len(profile.Fallbacks))
	mediaParts := media.FromMessages(req.Messages)

	attempt := func(alias string, attemptProfile ModelProfile, key string) (ChatResponse, bool) {
		provider, ok := h.cfg.Providers[key]
		if !ok {
			failures = append(failures, &ConfigError{
				Code:    "model/provider-desconhecido",
				Message: fmt.Sprintf("provider %q do modelo %q não injetado", key, alias),
			})
			return ChatResponse{}, false
		}
		if err := h.acquireRateLimit(ctx, handler, key); err != nil {
			failures = append(failures, err)
			return ChatResponse{}, false
		}
		attemptReq := req
		attemptReq.Model = attemptProfile.Model
		var (
			resp ChatResponse
			err  error
		)
		if streaming, ok := provider.(StreamingProvider); ok {
			resp, err = streaming.ChatStream(ctx, attemptReq, sink)
		} else {
			resp, err = provider.Chat(ctx, attemptReq, sink.Text)
		}
		if err != nil {
			failures = append(failures, err)
			return ChatResponse{}, false
		}
		return resp, true
	}

	if resp, ok := attempt(primaryAlias, profile, providerKey); ok {
		return resp, primaryAlias, nil
	}
	for _, fallbackAlias := range profile.Fallbacks {
		fallbackProfile, key, err := h.resolveModel(fallbackAlias)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if !media.Supports(fallbackProfile.Capabilities, mediaParts) {
			// FR-MM-011: nunca enviar mídia a um perfil sem a capacidade exigida.
			failures = append(failures, &ConfigError{
				Code:    "model/midia-nao-suportada",
				Message: fmt.Sprintf("fallback %q não declara a capacidade de mídia do turno", fallbackAlias),
			})
			continue
		}
		if resp, ok := attempt(fallbackAlias, fallbackProfile, key); ok {
			return resp, fallbackAlias, nil
		}
	}
	return ChatResponse{}, primaryAlias, fmt.Errorf("harness: todos os modelos falharam (primário %q): %w", primaryAlias, errors.Join(failures...))
}

// limiterFor devolve o bucket e o limite do provider: entrada específica vence
// o default (feature 019/FR-RL-002).
func (h *Harness) limiterFor(key string) (*ratelimit.Bucket, RateLimit) {
	if rl, ok := h.cfg.RateLimits[key]; ok {
		return h.limiters[key], rl
	}
	return h.defaultLimiter, h.cfg.DefaultRateLimit
}

// acquireRateLimit respeita o bucket do provider: espera pelo próximo token
// (cancelável) ou falha retryável quando a espera excede MaxWait
// (feature 019/FR-RL-003/004/008).
func (h *Harness) acquireRateLimit(ctx context.Context, handler Handler, providerKey string) error {
	bucket, rl := h.limiterFor(providerKey)
	if bucket == nil || !bucket.Limited() {
		return nil
	}
	h.limiterMu.Lock()
	wait := bucket.Reserve(h.cfg.Clock.Now())
	h.limiterMu.Unlock()
	if wait <= 0 {
		return nil
	}
	if rl.MaxWait > 0 && wait > rl.MaxWait {
		return &ProviderError{
			Code:    "ratelimit/espera-excedida",
			Message: fmt.Sprintf("provider %q: espera de %s excede o teto de %s", providerKey, wait, rl.MaxWait),
		}
	}
	h.emitRateLimit(ctx, handler, providerKey, wait)
	if err := h.cfg.Waiter.Wait(ctx, wait); err != nil {
		return err
	}
	return nil
}

// emitRateLimit publica o evento quando o Handler implementa RateLimitHandler.
func (h *Harness) emitRateLimit(ctx context.Context, handler Handler, providerKey string, wait time.Duration) {
	if handler == nil {
		return
	}
	sink, ok := handler.(RateLimitHandler)
	if !ok {
		return
	}
	sink.RateLimited(ctx, RateLimitEvent{Provider: providerKey, WaitMS: wait.Milliseconds(), Reason: "requests_per_minute"})
}
