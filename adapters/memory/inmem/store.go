// Package inmem entrega implementações em memória das portas de memória do
// harness (MemoryStore/Retriever) para desenvolvimento e testes (R6). O núcleo
// não cria armazenamento próprio (FR-029); a persistência real é do host
// (ex.: pgvector), que injeta a porta correspondente.
package inmem

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/google/uuid"

	"rmarquespaixao/ia-harness/harness"
	"rmarquespaixao/ia-harness/internal/platform/clock"
)

// ErrInvalidFact marca a gravação de um fato sem user_id ou sem texto.
var ErrInvalidFact = errors.New("inmem: fato inválido: user_id e text são obrigatórios")

// Store é um harness.MemoryStore em memória, isolado por usuário e seguro para
// uso concorrente.
type Store struct {
	mu    sync.RWMutex
	clock harness.Clock
	facts map[string][]harness.Fact
}

// Store implementa harness.MemoryStore.
var _ harness.MemoryStore = (*Store)(nil)

// New cria o store com o relógio injetado (timestamps vêm sempre do Clock);
// clock nil usa o relógio do sistema.
func New(clk harness.Clock) *Store {
	if clk == nil {
		clk = clock.System{}
	}
	return &Store{clock: clk, facts: map[string][]harness.Fact{}}
}

// Put grava ou atualiza o fato do usuário. ID ausente é gerado com UUID; no
// update o CreatedAt armazenado é preservado e o UpdatedAt vem do Clock.
func (s *Store) Put(_ context.Context, f harness.Fact) error {
	if f.UserID == "" {
		return fmt.Errorf("%w: user_id vazio", ErrInvalidFact)
	}
	if f.Text == "" {
		return fmt.Errorf("%w: text vazio", ErrInvalidFact)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.clock.Now()
	if f.ID == "" {
		f.ID = uuid.NewString()
	}
	facts := s.facts[f.UserID]
	for i, existing := range facts {
		if existing.ID == f.ID {
			f.CreatedAt = existing.CreatedAt
			f.UpdatedAt = now
			facts[i] = f
			s.facts[f.UserID] = facts
			return nil
		}
	}
	if f.CreatedAt.IsZero() {
		f.CreatedAt = now
	}
	f.UpdatedAt = now
	s.facts[f.UserID] = append(facts, f)
	return nil
}

// List devolve uma cópia dos fatos do usuário, do mais recente (UpdatedAt
// desc) para o mais antigo; empate desempata por ID para ser determinístico.
func (s *Store) List(_ context.Context, userID string) ([]harness.Fact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	facts := s.facts[userID]
	out := make([]harness.Fact, len(facts))
	copy(out, facts)
	sort.Slice(out, func(i, j int) bool {
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// Delete remove o fato do usuário; ID inexistente não é erro (idempotente) e a
// busca nunca cruza o escopo de usuário (RN-1/FR-027).
func (s *Store) Delete(_ context.Context, userID, factID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	facts := s.facts[userID]
	for i, f := range facts {
		if f.ID == factID {
			s.facts[userID] = append(facts[:i], facts[i+1:]...)
			return nil
		}
	}
	return nil
}
