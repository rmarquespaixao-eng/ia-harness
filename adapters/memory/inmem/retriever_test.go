package inmem_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmarquespaixao/ia-harness/adapters/memory/inmem"
	"rmarquespaixao/ia-harness/harness"
)

// fakeEmbedder devolve vetores fixos por texto, registra as chamadas e pode
// falhar em uma chamada específica (1-based; 0 = nunca).
type fakeEmbedder struct {
	vectors  map[string][]float32
	fallback []float32
	failAt   int
	failErr  error
	calls    [][]string
}

// Embed implementa harness.Embedder de forma determinística.
func (e *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	e.calls = append(e.calls, append([]string(nil), texts...))
	if e.failAt > 0 && len(e.calls) == e.failAt {
		return nil, e.failErr
	}
	out := make([][]float32, len(texts))
	for i, text := range texts {
		switch {
		case e.vectors[text] != nil:
			out[i] = e.vectors[text]
		case e.fallback != nil:
			out[i] = e.fallback
		default:
			out[i] = []float32{0}
		}
	}
	return out, nil
}

var _ harness.Embedder = (*fakeEmbedder)(nil)

// vetoresFaq mapeia os textos usados nos testes para vetores 2-D.
func vetoresFaq() map[string][]float32 {
	return map[string][]float32{
		"gato":          {1, 0},
		"cachorro":      {0.8, 0.6},
		"peixe":         {0, 1},
		"segredo":       {1, 0},
		"quero um gato": {1, 0},
		"segredo?":      {1, 0},
	}
}

// docsFaq monta o índice de dois usuários.
func docsFaq() map[string][]harness.RetrievedItem {
	return map[string][]harness.RetrievedItem{
		"u1": {
			{ID: "d1", Text: "gato", Source: "faq:gatos"},
			{ID: "d2", Text: "cachorro", Source: "faq:caes"},
			{ID: "d3", Text: "peixe", Source: "faq:peixes"},
		},
		"u2": {{ID: "x1", Text: "segredo", Source: "u2:cofre"}},
	}
}

// ids extrai os IDs dos itens na ordem devolvida.
func ids(items []harness.RetrievedItem) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.ID
	}
	return out
}

func TestRetrieverPreComputaDocsEOrdenaPorScore(t *testing.T) {
	// Arrange
	emb := &fakeEmbedder{vectors: vetoresFaq()}
	r := inmem.NewRetriever(emb, docsFaq())
	require.Len(t, emb.calls, 1, "documentos são embedados uma vez, no construtor")
	assert.Equal(t, []string{"gato", "cachorro", "peixe", "segredo"}, emb.calls[0])

	// Act
	items, err := r.Retrieve(context.Background(), "u1", "quero um gato", 2)

	// Assert
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, "d1", items[0].ID)
	assert.InDelta(t, 1.0, items[0].Score, 1e-6)
	assert.Equal(t, "faq:gatos", items[0].Source)
	assert.Equal(t, "d2", items[1].ID)
	assert.InDelta(t, 0.8, items[1].Score, 1e-6)
	require.Len(t, emb.calls, 2, "apenas a consulta é embedada na Retrieve")
	assert.Equal(t, []string{"quero um gato"}, emb.calls[1])
}

func TestRetrieverIsolaPorUsuario(t *testing.T) {
	// Arrange
	emb := &fakeEmbedder{vectors: vetoresFaq()}
	r := inmem.NewRetriever(emb, docsFaq())

	// Act
	itensU1, errU1 := r.Retrieve(context.Background(), "u1", "segredo?", 5)
	itensU2, errU2 := r.Retrieve(context.Background(), "u2", "segredo?", 5)

	// Assert
	require.NoError(t, errU1)
	require.NoError(t, errU2)
	assert.ElementsMatch(t, []string{"d1", "d2", "d3"}, ids(itensU1), "u1 nunca enxerga o índice de u2")
	assert.Equal(t, []string{"x1"}, ids(itensU2))
}

func TestRetrieverEmpateDesempataPorID(t *testing.T) {
	// Arrange
	docs := map[string][]harness.RetrievedItem{"u1": {
		{ID: "c", Text: "t", Source: "s"},
		{ID: "a", Text: "t", Source: "s"},
		{ID: "b", Text: "t", Source: "s"},
	}}
	emb := &fakeEmbedder{vectors: map[string][]float32{"t": {1, 0}, "q": {1, 0}}}
	r := inmem.NewRetriever(emb, docs)

	// Act
	items, err := r.Retrieve(context.Background(), "u1", "q", 5)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, ids(items), "empate total é determinístico por ID")
}

func TestRetrieverSemTrabalhoNaoFazIO(t *testing.T) {
	// Arrange
	emb := &fakeEmbedder{vectors: vetoresFaq()}
	r := inmem.NewRetriever(emb, docsFaq())
	require.Len(t, emb.calls, 1)

	// Act
	itemsK, errK := r.Retrieve(context.Background(), "u1", "quero um gato", 0)
	itemsQueryVazia, errQueryVazia := r.Retrieve(context.Background(), "u1", "   ", 5)
	itemsUsuarioSemDocs, errUsuarioSemDocs := r.Retrieve(context.Background(), "ninguem", "quero um gato", 5)

	// Assert
	require.NoError(t, errK)
	require.NoError(t, errQueryVazia)
	require.NoError(t, errUsuarioSemDocs)
	assert.Nil(t, itemsK)
	assert.Nil(t, itemsQueryVazia)
	assert.Nil(t, itemsUsuarioSemDocs)
	assert.Len(t, emb.calls, 1, "nenhum desses casos embeda a consulta")
}

func TestRetrieverKMaiorQueIndiceDevolveTudo(t *testing.T) {
	// Arrange
	emb := &fakeEmbedder{vectors: vetoresFaq()}
	r := inmem.NewRetriever(emb, docsFaq())

	// Act
	items, err := r.Retrieve(context.Background(), "u1", "quero um gato", 99)

	// Assert
	require.NoError(t, err)
	assert.Len(t, items, 3)
}

func TestRetrieverReindexaAposFalhaNoConstrutor(t *testing.T) {
	// Arrange
	emb := &fakeEmbedder{vectors: vetoresFaq(), failAt: 1, failErr: errors.New("rede caiu")}
	r := inmem.NewRetriever(emb, docsFaq())
	require.Len(t, emb.calls, 1, "construtor tentou indexar")

	// Act
	items, err := r.Retrieve(context.Background(), "u1", "quero um gato", 1)

	// Assert
	require.NoError(t, err, "a falha do construtor não brica o retriever")
	require.Len(t, items, 1)
	assert.Equal(t, "d1", items[0].ID)
	assert.Len(t, emb.calls, 3, "retentou o índice e embedou a consulta")
}

func TestRetrieverErroAoEmbedarConsulta(t *testing.T) {
	// Arrange
	emb := &fakeEmbedder{vectors: vetoresFaq(), failAt: 2, failErr: errors.New("429 rate limit")}
	r := inmem.NewRetriever(emb, docsFaq())

	// Act
	_, err := r.Retrieve(context.Background(), "u1", "quero um gato", 1)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "embedding da consulta")
	assert.ErrorIs(t, err, emb.failErr, "causa preservada com %w")
}

func TestRetrieverSemEmbedder(t *testing.T) {
	// Arrange
	r := inmem.NewRetriever(nil, docsFaq())

	// Act
	_, err := r.Retrieve(context.Background(), "u1", "quero um gato", 1)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sem Embedder")
}

func TestRetrieverCopiaIndiceEDevolveCopia(t *testing.T) {
	// Arrange
	docs := docsFaq()
	emb := &fakeEmbedder{vectors: vetoresFaq()}
	r := inmem.NewRetriever(emb, docs)
	docs["u1"][0].Text = "mutado depois do New"
	docs["u3"] = []harness.RetrievedItem{{ID: "n1", Text: "gato", Source: "s"}}

	// Act
	items, err := r.Retrieve(context.Background(), "u1", "quero um gato", 5)
	require.NoError(t, err)
	items[0].Text = "mutado pelo chamador"
	itemsAgain, err := r.Retrieve(context.Background(), "u1", "quero um gato", 5)
	itensU3, errU3 := r.Retrieve(context.Background(), "u3", "quero um gato", 5)

	// Assert
	require.NoError(t, err)
	require.NoError(t, errU3)
	assert.Equal(t, "gato", itemsAgain[0].Text, "store devolve cópia a cada chamada")
	assert.Nil(t, itensU3, "usuário adicionado após o New não entra no índice")
}
