package inmem

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	core "github.com/rmarquespaixao-eng/ia-harness/internal/core"
)

// fakeClock é um relógio determinístico controlável pelo teste.
type fakeClock struct{ now time.Time }

// Now devolve o instante fixado, sem tocar no relógio do sistema.
func (c *fakeClock) Now() time.Time { return c.now }

// fakeResponse monta uma resposta com uma parte de texto.
func fakeResponse(text string) core.ChatResponse {
	return core.ChatResponse{Message: core.Message{
		Role:  core.RoleAssistant,
		Parts: []core.Part{{Kind: core.PartText, Text: text}},
	}}
}

// newTestStore cria o store com um clock fake ancorado em um instante fixo.
func newTestStore(cfg core.SemanticCacheConfig) (*Store, *fakeClock) {
	clk := &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	return New(cfg, clk), clk
}

// TestLookup_Hit cobre o caminho feliz: a mesma consulta do mesmo usuário/modelo
// encontra a entrada e devolve o Score preenchido.
func TestLookup_Hit(t *testing.T) {
	// Arrange
	store, clk := newTestStore(core.SemanticCacheConfig{Enabled: true, MinScore: 0.9, MaxEntries: 10})
	ctx := context.Background()
	query := core.CacheQuery{UserID: "u1", Model: "m1", Key: "k1", Vector: []float32{1, 0, 0}}
	require.NoError(t, store.Store(ctx, query, core.CacheEntry{Response: fakeResponse("saldo: R$ 100"), CreatedAt: clk.now}))

	// Act
	found, ok, err := store.Lookup(ctx, query)

	// Assert
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, found)
	assert.InDelta(t, 1.0, found.Score, 1e-9)
	assert.Equal(t, "saldo: R$ 100", found.Response.Message.Parts[0].Text)
	assert.Equal(t, clk.now, found.CreatedAt)
}

// TestLookup_MissAbaixoDoLimiar garante que similaridade abaixo do MinScore não
// devolve hit.
func TestLookup_MissAbaixoDoLimiar(t *testing.T) {
	// Arrange
	store, clk := newTestStore(core.SemanticCacheConfig{Enabled: true, MinScore: 0.9, MaxEntries: 10})
	ctx := context.Background()
	stored := core.CacheQuery{UserID: "u1", Model: "m1", Key: "k1", Vector: []float32{1, 0}}
	require.NoError(t, store.Store(ctx, stored, core.CacheEntry{Response: fakeResponse("x"), CreatedAt: clk.now}))

	// Act
	found, ok, err := store.Lookup(ctx, core.CacheQuery{UserID: "u1", Model: "m1", Key: "k2", Vector: []float32{0, 1}})

	// Assert
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Nil(t, found)
}

// TestLookup_IsolamentoPorUsuario garante que a entrada de um usuário não vaza
// para outro, mesmo com vetor idêntico.
func TestLookup_IsolamentoPorUsuario(t *testing.T) {
	// Arrange
	store, clk := newTestStore(core.SemanticCacheConfig{Enabled: true, MinScore: 0.9, MaxEntries: 10})
	ctx := context.Background()
	vector := []float32{1, 0, 0}
	require.NoError(t, store.Store(ctx, core.CacheQuery{UserID: "A", Model: "m1", Key: "k1", Vector: vector},
		core.CacheEntry{Response: fakeResponse("do A"), CreatedAt: clk.now}))

	// Act
	found, ok, err := store.Lookup(ctx, core.CacheQuery{UserID: "B", Model: "m1", Key: "k1", Vector: vector})

	// Assert
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Nil(t, found)
}

// TestLookup_IsolamentoPorModelo garante que modelos distintos não compartilham
// a mesma entrada.
func TestLookup_IsolamentoPorModelo(t *testing.T) {
	// Arrange
	store, clk := newTestStore(core.SemanticCacheConfig{Enabled: true, MinScore: 0.9, MaxEntries: 10})
	ctx := context.Background()
	vector := []float32{1, 0, 0}
	require.NoError(t, store.Store(ctx, core.CacheQuery{UserID: "u1", Model: "m1", Key: "k1", Vector: vector},
		core.CacheEntry{Response: fakeResponse("do m1"), CreatedAt: clk.now}))

	// Act
	found, ok, err := store.Lookup(ctx, core.CacheQuery{UserID: "u1", Model: "m2", Key: "k1", Vector: vector})

	// Assert
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Nil(t, found)
}

// TestLookup_TTL cobre a expiração por TTL com clock fake e a ausência de
// expiração quando o TTL é zero.
func TestLookup_TTL(t *testing.T) {
	ctx := context.Background()
	vector := []float32{1, 0, 0}

	t.Run("expira após o TTL", func(t *testing.T) {
		// Arrange
		store, clk := newTestStore(core.SemanticCacheConfig{Enabled: true, MinScore: 0.9, TTL: time.Minute, MaxEntries: 10})
		query := core.CacheQuery{UserID: "u1", Model: "m1", Key: "k1", Vector: vector}
		require.NoError(t, store.Store(ctx, query, core.CacheEntry{Response: fakeResponse("x"), CreatedAt: clk.now}))

		// Act: avança 2min, além do TTL de 1min
		clk.now = clk.now.Add(2 * time.Minute)
		found, ok, err := store.Lookup(ctx, query)

		// Assert
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Nil(t, found)
	})

	t.Run("sem TTL não expira", func(t *testing.T) {
		// Arrange
		store, clk := newTestStore(core.SemanticCacheConfig{Enabled: true, MinScore: 0.9, TTL: 0, MaxEntries: 10})
		query := core.CacheQuery{UserID: "u1", Model: "m1", Key: "k1", Vector: vector}
		require.NoError(t, store.Store(ctx, query, core.CacheEntry{Response: fakeResponse("x"), CreatedAt: clk.now}))

		// Act: avança 2min mesmo sem TTL
		clk.now = clk.now.Add(2 * time.Minute)
		found, ok, err := store.Lookup(ctx, query)

		// Assert
		require.NoError(t, err)
		require.True(t, ok)
		assert.NotNil(t, found)
	})
}

// TestStore_Eviction garante a eviction FIFO: ao estourar MaxEntries, a entrada
// mais antiga é descartada.
func TestStore_Eviction(t *testing.T) {
	// Arrange: três chaves com vetores ortogonais entre si (só o exato casa)
	store, clk := newTestStore(core.SemanticCacheConfig{Enabled: true, MinScore: 0.9, MaxEntries: 2})
	ctx := context.Background()
	queries := []core.CacheQuery{
		{UserID: "u1", Model: "m1", Key: "k1", Vector: []float32{1, 0, 0}},
		{UserID: "u1", Model: "m1", Key: "k2", Vector: []float32{0, 1, 0}},
		{UserID: "u1", Model: "m1", Key: "k3", Vector: []float32{0, 0, 1}},
	}
	for _, q := range queries {
		require.NoError(t, store.Store(ctx, q, core.CacheEntry{Response: fakeResponse(q.Key), CreatedAt: clk.now}))
	}

	// Act
	_, foundK1, errK1 := store.Lookup(ctx, queries[0])
	_, foundK2, errK2 := store.Lookup(ctx, queries[1])
	_, foundK3, errK3 := store.Lookup(ctx, queries[2])

	// Assert: a mais antiga (k1) saiu; k2 e k3 permanecem
	require.NoError(t, errK1)
	require.NoError(t, errK2)
	require.NoError(t, errK3)
	assert.False(t, foundK1)
	assert.True(t, foundK2)
	assert.True(t, foundK3)
	assert.Len(t, store.entries, 2)
}

// TestStore_IgnoraEntradasInvalidas garante que vetor inválido ou Key vazia não
// são armazenados e, portanto, nunca geram hit.
func TestStore_IgnoraEntradasInvalidas(t *testing.T) {
	ctx := context.Background()

	t.Run("vetor inválido é ignorado", func(t *testing.T) {
		// Arrange
		store, clk := newTestStore(core.SemanticCacheConfig{Enabled: true, MinScore: 0.9, MaxEntries: 10})

		// Act
		err := store.Store(ctx, core.CacheQuery{UserID: "u1", Model: "m1", Key: "k1", Vector: nil},
			core.CacheEntry{Response: fakeResponse("x"), CreatedAt: clk.now})
		found, ok, lookupErr := store.Lookup(ctx, core.CacheQuery{UserID: "u1", Model: "m1", Key: "k1", Vector: []float32{1, 0}})

		// Assert
		require.NoError(t, err)
		require.NoError(t, lookupErr)
		assert.False(t, ok)
		assert.Nil(t, found)
		assert.Empty(t, store.entries)
	})

	t.Run("key vazia é ignorada", func(t *testing.T) {
		// Arrange
		store, clk := newTestStore(core.SemanticCacheConfig{Enabled: true, MinScore: 0.9, MaxEntries: 10})

		// Act
		err := store.Store(ctx, core.CacheQuery{UserID: "u1", Model: "m1", Key: "", Vector: []float32{1, 0}},
			core.CacheEntry{Response: fakeResponse("x"), CreatedAt: clk.now})

		// Assert
		require.NoError(t, err)
		assert.Empty(t, store.entries)
	})
}
