package inmem_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmarquespaixao/ia-harness/adapters/memory/inmem"
	"rmarquespaixao/ia-harness/harness"
)

// fakeClock devolve instantes controlados pelo teste.
type fakeClock struct{ now time.Time }

// Now devolve o instante corrente do fake.
func (c *fakeClock) Now() time.Time { return c.now }

// advance desloca o relógio para simular passagem de tempo.
func (c *fakeClock) advance(d time.Duration) { c.now = c.now.Add(d) }

// listFacts é um atalho de List para os testes.
func listFacts(t *testing.T, store *inmem.Store, userID string) []harness.Fact {
	t.Helper()
	facts, err := store.List(context.Background(), userID)
	require.NoError(t, err)
	return facts
}

func TestStorePutGeraIDETimestampsDoClock(t *testing.T) {
	// Arrange
	t0 := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	clk := &fakeClock{now: t0}
	store := inmem.New(clk)

	// Act
	err := store.Put(context.Background(), harness.Fact{
		UserID: "u1",
		Kind:   harness.FactKindPreference,
		Text:   "Prefere respostas curtas",
		Source: "user",
	})

	// Assert
	require.NoError(t, err)
	facts := listFacts(t, store, "u1")
	require.Len(t, facts, 1)
	_, err = uuid.Parse(facts[0].ID)
	require.NoError(t, err, "ID deve ser UUID gerado quando ausente")
	assert.Equal(t, t0, facts[0].CreatedAt)
	assert.Equal(t, t0, facts[0].UpdatedAt)
	assert.Equal(t, harness.FactKindPreference, facts[0].Kind)
}

func TestStorePutRejeitaFatoInvalido(t *testing.T) {
	tests := []struct {
		name string
		fact harness.Fact
	}{
		{"user_id vazio", harness.Fact{Text: "sem dono"}},
		{"text vazio", harness.Fact{UserID: "u1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			store := inmem.New(&fakeClock{now: time.Now()})

			// Act
			err := store.Put(context.Background(), tt.fact)

			// Assert
			assert.ErrorIs(t, err, inmem.ErrInvalidFact)
			assert.Empty(t, listFacts(t, store, tt.fact.UserID))
		})
	}
}

func TestStorePutAtualizaEPreservaCreatedAt(t *testing.T) {
	// Arrange
	t0 := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	clk := &fakeClock{now: t0}
	store := inmem.New(clk)
	require.NoError(t, store.Put(context.Background(), harness.Fact{
		ID: "f1", UserID: "u1", Text: "versão 1", Source: "user",
	}))

	// Act
	clk.advance(2 * time.Hour)
	require.NoError(t, store.Put(context.Background(), harness.Fact{
		ID: "f1", UserID: "u1", Text: "versão 2", Source: "user",
	}))

	// Assert
	facts := listFacts(t, store, "u1")
	require.Len(t, facts, 1, "mesmo ID atualiza no lugar")
	assert.Equal(t, "versão 2", facts[0].Text)
	assert.Equal(t, t0, facts[0].CreatedAt, "CreatedAt é preservado no update")
	assert.Equal(t, t0.Add(2*time.Hour), facts[0].UpdatedAt)
}

func TestStoreListOrdenaPorUpdatedAtDesc(t *testing.T) {
	// Arrange
	clk := &fakeClock{now: time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)}
	store := inmem.New(clk)
	for _, fact := range []harness.Fact{
		{ID: "f1", UserID: "u1", Text: "primeiro"},
		{ID: "f2", UserID: "u1", Text: "segundo"},
		{ID: "f3", UserID: "u1", Text: "terceiro"},
	} {
		require.NoError(t, store.Put(context.Background(), fact))
		clk.advance(time.Minute)
	}
	require.NoError(t, store.Put(context.Background(), harness.Fact{ID: "f1", UserID: "u1", Text: "primeiro atualizado"}))

	// Act
	facts := listFacts(t, store, "u1")

	// Assert
	require.Len(t, facts, 3)
	assert.Equal(t, []string{"f1", "f3", "f2"}, []string{facts[0].ID, facts[1].ID, facts[2].ID})
}

func TestStoreListDevolveCopia(t *testing.T) {
	// Arrange
	store := inmem.New(&fakeClock{now: time.Now()})
	require.NoError(t, store.Put(context.Background(), harness.Fact{ID: "f1", UserID: "u1", Text: "original"}))

	// Act
	facts := listFacts(t, store, "u1")
	require.Len(t, facts, 1)
	facts[0].Text = "mutado pelo chamador"
	_ = append(facts, harness.Fact{ID: "intruso", UserID: "u1", Text: "injetado"})

	// Assert
	reloaded := listFacts(t, store, "u1")
	require.Len(t, reloaded, 1, "mutação da cópia não afeta o store")
	assert.Equal(t, "original", reloaded[0].Text)
}

func TestStoreIsolamentoEntreUsuarios(t *testing.T) {
	// Arrange
	store := inmem.New(&fakeClock{now: time.Now()})
	require.NoError(t, store.Put(context.Background(), harness.Fact{ID: "mesmo-id", UserID: "u1", Text: "segredo de u1"}))
	require.NoError(t, store.Put(context.Background(), harness.Fact{ID: "mesmo-id", UserID: "u2", Text: "segredo de u2"}))

	// Act
	require.NoError(t, store.Delete(context.Background(), "u1", "mesmo-id"))
	factsU1 := listFacts(t, store, "u1")
	factsU2 := listFacts(t, store, "u2")

	// Assert
	assert.Empty(t, factsU1)
	require.Len(t, factsU2, 1)
	assert.Equal(t, "segredo de u2", factsU2[0].Text, "Delete de u1 não pode tocar u2")
}

func TestStoreDeleteIdempotenteENaoVazaExistencia(t *testing.T) {
	// Arrange
	store := inmem.New(&fakeClock{now: time.Now()})
	require.NoError(t, store.Put(context.Background(), harness.Fact{ID: "f1", UserID: "u1", Text: "lembrar"}))

	// Act
	errFirst := store.Delete(context.Background(), "u1", "f1")
	errAgain := store.Delete(context.Background(), "u1", "f1")
	errOtherUser := store.Delete(context.Background(), "u2", "f1")

	// Assert
	require.NoError(t, errFirst)
	require.NoError(t, errAgain, "delete repetido é idempotente")
	require.NoError(t, errOtherUser, "usuário dono de nada não recebe erro nem pista")
}

func TestStoreNewComClockNilUsaRelogioDoSistema(t *testing.T) {
	// Arrange
	store := inmem.New(nil)
	before := time.Now()

	// Act
	err := store.Put(context.Background(), harness.Fact{UserID: "u1", Text: "fato"})

	// Assert
	require.NoError(t, err)
	facts := listFacts(t, store, "u1")
	require.Len(t, facts, 1)
	assert.False(t, facts[0].CreatedAt.IsZero())
	assert.WithinDuration(t, before, facts[0].CreatedAt, time.Second)
}

func TestStorePutInvalidoNaoAlteraFatoExistente(t *testing.T) {
	// Arrange
	store := inmem.New(&fakeClock{now: time.Now()})
	require.NoError(t, store.Put(context.Background(), harness.Fact{ID: "f1", UserID: "u1", Text: "original"}))

	// Act
	err := store.Put(context.Background(), harness.Fact{ID: "f1", UserID: "u1"})

	// Assert
	assert.True(t, errors.Is(err, inmem.ErrInvalidFact))
	facts := listFacts(t, store, "u1")
	require.Len(t, facts, 1)
	assert.Equal(t, "original", facts[0].Text)
}
