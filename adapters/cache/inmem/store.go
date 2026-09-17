// Package inmem é um SemanticCache em memória para dev/teste (feature 020):
// busca por similaridade de cosseno, TTL e eviction FIFO, isolado por usuário.
package inmem

import (
	"context"
	"sync"
	"time"

	core "github.com/rmarquespaixao-eng/ia-harness/internal/core"
	"github.com/rmarquespaixao-eng/ia-harness/internal/engine/semcache"
	"github.com/rmarquespaixao-eng/ia-harness/internal/platform/clock"
)

// entry é uma resposta cacheada com o vetor do contexto.
type entry struct {
	userID    string
	model     string
	key       string
	vector    []float32
	response  core.ChatResponse
	createdAt time.Time
}

// Store é um SemanticCache em memória, seguro para uso concorrente.
type Store struct {
	mu      sync.Mutex
	cfg     core.SemanticCacheConfig
	clock   core.Clock
	entries []entry
}

// New cria o store aplicando os defaults do cache. clock nil usa o relógio do
// sistema.
func New(cfg core.SemanticCacheConfig, clk core.Clock) *Store {
	if clk == nil {
		clk = clock.System{}
	}
	return &Store{cfg: semcache.Defaults(cfg), clock: clk}
}

// Lookup devolve a melhor entrada do usuário acima do limiar (FR-SC-003/006/008).
func (s *Store) Lookup(ctx context.Context, q core.CacheQuery) (*core.CacheEntry, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if !semcache.Valid(q.Vector) {
		return nil, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()

	var (
		best      = -1
		bestScore = s.cfg.MinScore
	)
	for i := range s.entries {
		candidate := s.entries[i]
		if candidate.userID != q.UserID || candidate.model != q.Model {
			continue
		}
		score := semcache.Cosine(q.Vector, candidate.vector)
		if score >= bestScore {
			best = i
			bestScore = score
		}
	}
	if best < 0 {
		return nil, false, nil
	}
	found := s.entries[best]
	return &core.CacheEntry{Response: cloneResponse(found.response), CreatedAt: found.createdAt, Score: bestScore}, true, nil
}

// Store guarda a resposta com o vetor do contexto (FR-SC-006).
func (s *Store) Store(ctx context.Context, q core.CacheQuery, e core.CacheEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !semcache.Valid(q.Vector) || q.Key == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	for i := range s.entries {
		if s.entries[i].userID == q.UserID && s.entries[i].model == q.Model && s.entries[i].key == q.Key {
			s.entries[i].vector = append([]float32(nil), q.Vector...)
			s.entries[i].response = cloneResponse(e.Response)
			s.entries[i].createdAt = e.CreatedAt
			return nil
		}
	}
	if len(s.entries) >= s.cfg.MaxEntries {
		s.entries = s.entries[1:]
	}
	s.entries = append(s.entries, entry{
		userID:    q.UserID,
		model:     q.Model,
		key:       q.Key,
		vector:    append([]float32(nil), q.Vector...),
		response:  cloneResponse(e.Response),
		createdAt: e.CreatedAt,
	})
	return nil
}

// prune descarta entradas expiradas pelo TTL.
func (s *Store) prune() {
	if s.cfg.TTL <= 0 {
		return
	}
	now := s.clock.Now()
	kept := s.entries[:0]
	for _, e := range s.entries {
		if now.Sub(e.createdAt) < s.cfg.TTL {
			kept = append(kept, e)
		}
	}
	s.entries = kept
}

// cloneResponse copia a resposta e as partes da mensagem (evita aliasing entre
// a entrada armazenada e o turno servido).
func cloneResponse(in core.ChatResponse) core.ChatResponse {
	out := in
	out.Message.Parts = append([]core.Part(nil), in.Message.Parts...)
	out.ToolCalls = append([]core.ToolCall(nil), in.ToolCalls...)
	return out
}
