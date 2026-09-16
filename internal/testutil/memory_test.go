package testutil_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/harness"
	"github.com/rmarquespaixao-eng/ia-harness/internal/testutil"
)

func TestFakeMemoryStore_IsolaPorUsuario(t *testing.T) {
	// Arrange
	store := &testutil.FakeMemoryStore{}
	require.NoError(t, store.Put(context.Background(), harness.Fact{ID: "f1", UserID: "u1", Kind: harness.FactKindFact, Text: "gosta de go"}))
	require.NoError(t, store.Put(context.Background(), harness.Fact{ID: "f2", UserID: "u2", Kind: harness.FactKindPreference, Text: "sem lactose"}))

	// Act
	fatosU1, errU1 := store.List(context.Background(), "u1")
	fatosU2, errU2 := store.List(context.Background(), "u2")

	// Assert
	require.NoError(t, errU1)
	require.NoError(t, errU2)
	require.Len(t, fatosU1, 1)
	assert.Equal(t, "f1", fatosU1[0].ID)
	require.Len(t, fatosU2, 1)
	assert.Equal(t, "f2", fatosU2[0].ID)
}

func TestFakeMemoryStore_DeleteIsolaUsuarioEIgnoraInexistente(t *testing.T) {
	// Arrange
	store := &testutil.FakeMemoryStore{}
	require.NoError(t, store.Put(context.Background(), harness.Fact{ID: "f1", UserID: "u1"}))
	require.NoError(t, store.Put(context.Background(), harness.Fact{ID: "f2", UserID: "u1"}))
	require.NoError(t, store.Put(context.Background(), harness.Fact{ID: "f3", UserID: "u2"}))

	// Act
	errDelete := store.Delete(context.Background(), "u1", "f1")
	errInexistente := store.Delete(context.Background(), "u1", "inexistente")
	fatosU1, errU1 := store.List(context.Background(), "u1")
	fatosU2, errU2 := store.List(context.Background(), "u2")

	// Assert
	require.NoError(t, errDelete)
	require.NoError(t, errInexistente)
	require.NoError(t, errU1)
	require.NoError(t, errU2)
	require.Len(t, fatosU1, 1)
	assert.Equal(t, "f2", fatosU1[0].ID)
	require.Len(t, fatosU2, 1)
	assert.Equal(t, "f3", fatosU2[0].ID)
}

func TestFakeMemoryStore_ListDevolveCopia(t *testing.T) {
	// Arrange
	store := &testutil.FakeMemoryStore{}
	require.NoError(t, store.Put(context.Background(), harness.Fact{ID: "f1", UserID: "u1", Text: "original"}))

	// Act
	fatos, err := store.List(context.Background(), "u1")
	require.NoError(t, err)
	fatos[0].Text = "mutado"
	denovo, err := store.List(context.Background(), "u1")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "original", denovo[0].Text)
}

func TestFakeRetriever_RegistraQueriesEDevolveCopia(t *testing.T) {
	// Arrange
	retriever := &testutil.FakeRetriever{Items: []harness.RetrievedItem{{ID: "r1", Text: "trecho", Source: "doc", Score: 0.9}}}

	// Act
	itens, err := retriever.Retrieve(context.Background(), "u1", "primeira", 5)
	require.NoError(t, err)
	itens[0].Text = "mutado"
	denovo, errDenovo := retriever.Retrieve(context.Background(), "u1", "segunda", 5)

	// Assert
	require.NoError(t, errDenovo)
	assert.Equal(t, "trecho", denovo[0].Text)
	assert.Equal(t, []string{"primeira", "segunda"}, retriever.Queries)
}

func TestFakeSummarizer_DevolveResumoEContaChamadas(t *testing.T) {
	// Arrange
	summarizer := &testutil.FakeSummarizer{Summary: "resumo"}

	// Act
	texto, err := summarizer.Summarize(context.Background(), []harness.Message{{Role: harness.RoleUser}}, 100)
	_, errSegundo := summarizer.Summarize(context.Background(), nil, 100)

	// Assert
	require.NoError(t, err)
	require.NoError(t, errSegundo)
	assert.Equal(t, "resumo", texto)
	assert.Equal(t, 2, summarizer.Calls)
}
