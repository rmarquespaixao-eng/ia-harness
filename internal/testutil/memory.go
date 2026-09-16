package testutil

import (
	"context"

	"github.com/rmarquespaixao-eng/ia-harness/harness"
)

// FakeMemoryStore guarda fatos em memória, isolados por user_id.
type FakeMemoryStore struct {
	Facts map[string][]harness.Fact
}

// Put grava o fato do usuário; mesmo ID é substituído no lugar.
func (m *FakeMemoryStore) Put(_ context.Context, f harness.Fact) error {
	if m.Facts == nil {
		m.Facts = map[string][]harness.Fact{}
	}
	facts := m.Facts[f.UserID]
	for i, existing := range facts {
		if existing.ID == f.ID {
			facts[i] = f
			m.Facts[f.UserID] = facts
			return nil
		}
	}
	m.Facts[f.UserID] = append(facts, f)
	return nil
}

// List devolve cópia dos fatos do usuário.
func (m *FakeMemoryStore) List(_ context.Context, userID string) ([]harness.Fact, error) {
	facts := m.Facts[userID]
	out := make([]harness.Fact, len(facts))
	copy(out, facts)
	return out, nil
}

// Delete remove o fato do usuário; ID inexistente não é erro.
func (m *FakeMemoryStore) Delete(_ context.Context, userID, factID string) error {
	facts := m.Facts[userID]
	for i, f := range facts {
		if f.ID == factID {
			m.Facts[userID] = append(facts[:i], facts[i+1:]...)
			return nil
		}
	}
	return nil
}

var _ harness.MemoryStore = (*FakeMemoryStore)(nil)

// FakeRetriever devolve itens fixos e registra as queries recebidas.
type FakeRetriever struct {
	Items   []harness.RetrievedItem
	Queries []string
}

// Retrieve registra a query e devolve cópia dos itens (k é ignorado).
func (r *FakeRetriever) Retrieve(_ context.Context, _, query string, _ int) ([]harness.RetrievedItem, error) {
	r.Queries = append(r.Queries, query)
	out := make([]harness.RetrievedItem, len(r.Items))
	copy(out, r.Items)
	return out, nil
}

var _ harness.Retriever = (*FakeRetriever)(nil)

// FakeSummarizer devolve sempre o mesmo resumo e conta as chamadas.
type FakeSummarizer struct {
	Summary string
	Calls   int
}

// Summarize devolve o resumo roteirizado.
func (s *FakeSummarizer) Summarize(_ context.Context, _ []harness.Message, _ int) (string, error) {
	s.Calls++
	return s.Summary, nil
}

var _ harness.Summarizer = (*FakeSummarizer)(nil)
