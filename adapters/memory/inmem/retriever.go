package inmem

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"rmarquespaixao/ia-harness/harness"
)

// Retriever é um harness.Retriever determinístico sobre documentos em memória,
// isolado por usuário (RN-1). Os embeddings dos documentos são pré-computados
// no construtor, numa única chamada ao Embedder; se essa indexação falhar, ela
// é retentada na primeira Retrieve (o construtor não devolve erro — assinatura
// do host — e nenhuma falha é engolida: ela reaparece na Retrieve).
type Retriever struct {
	embedder   harness.Embedder
	docsByUser map[string][]harness.RetrievedItem

	mu      sync.Mutex
	vectors map[string][][]float32
	ready   bool
}

// Retriever implementa harness.Retriever.
var _ harness.Retriever = (*Retriever)(nil)

// NewRetriever pré-computa os embeddings de todos os documentos (uma chamada
// por usuário, em ordem determinística) e monta o retriever. O mapa docsByUser
// é copiado: mutações do host depois do New não afetam o índice.
func NewRetriever(embedder harness.Embedder, docsByUser map[string][]harness.RetrievedItem) *Retriever {
	r := &Retriever{
		embedder:   embedder,
		docsByUser: copyDocs(docsByUser),
		vectors:    make(map[string][][]float32, len(docsByUser)),
	}
	// Pré-computação sem ctx do host: usa Background; a falha (rede, por
	// exemplo) é retentada na primeira Retrieve, que recebe o ctx do turno.
	_ = r.ensureIndex(context.Background())
	return r
}

// Retrieve devolve os k documentos do usuário mais similares à consulta (cosseno
// entre o embedding da query e os embeddings pré-computados), com Score
// preenchido; empate é desempatado por ID (determinístico). Sem documentos para
// o usuário, k <= 0 ou query vazia, devolve nil sem I/O.
func (r *Retriever) Retrieve(ctx context.Context, userID, query string, k int) ([]harness.RetrievedItem, error) {
	docs := r.docsByUser[userID]
	if k <= 0 || len(docs) == 0 || strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if r.embedder == nil {
		return nil, errors.New("inmem: retriever sem Embedder injetado")
	}
	if err := r.ensureIndex(ctx); err != nil {
		return nil, err
	}

	queryVectors, err := r.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("inmem: embedding da consulta: %w", err)
	}
	if len(queryVectors) == 0 {
		return nil, errors.New("inmem: embedding da consulta vazio")
	}

	r.mu.Lock()
	vectors := r.vectors[userID]
	r.mu.Unlock()

	scored := make([]harness.RetrievedItem, 0, len(docs))
	for i, doc := range docs {
		item := doc
		if i < len(vectors) {
			item.Score = cosine(queryVectors[0], vectors[i])
		}
		scored = append(scored, item)
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].ID < scored[j].ID
	})
	if k < len(scored) {
		scored = scored[:k]
	}
	return scored, nil
}

// ensureIndex pré-computa os embeddings dos documentos uma única vez. A falha
// não marca o índice como pronto, permitindo nova tentativa na próxima Retrieve.
func (r *Retriever) ensureIndex(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ready {
		return nil
	}
	if r.embedder == nil {
		return errors.New("inmem: retriever sem Embedder injetado")
	}

	users := make([]string, 0, len(r.docsByUser))
	for userID := range r.docsByUser {
		users = append(users, userID)
	}
	sort.Strings(users)

	var texts []string
	for _, userID := range users {
		for _, doc := range r.docsByUser[userID] {
			texts = append(texts, doc.Text)
		}
	}
	if len(texts) == 0 {
		r.ready = true
		return nil
	}

	vectors, err := r.embedder.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("inmem: indexar documentos: %w", err)
	}
	if len(vectors) != len(texts) {
		return fmt.Errorf("inmem: indexar documentos: %d embeddings para %d textos", len(vectors), len(texts))
	}
	pos := 0
	for _, userID := range users {
		docs := r.docsByUser[userID]
		userVectors := make([][]float32, len(docs))
		copy(userVectors, vectors[pos:pos+len(docs)])
		r.vectors[userID] = userVectors
		pos += len(docs)
	}
	r.ready = true
	return nil
}

// copyDocs devolve uma cópia rasa (itens são valores) do índice por usuário.
func copyDocs(docsByUser map[string][]harness.RetrievedItem) map[string][]harness.RetrievedItem {
	out := make(map[string][]harness.RetrievedItem, len(docsByUser))
	for userID, docs := range docsByUser {
		copied := make([]harness.RetrievedItem, len(docs))
		copy(copied, docs)
		out[userID] = copied
	}
	return out
}

// cosine devolve a similaridade de cosseno entre dois vetores; vetores vazios,
// de tamanhos diferentes ou de norma zero valem 0 (sem similaridade).
func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		av, bv := float64(a[i]), float64(b[i])
		dot += av * bv
		normA += av * av
		normB += bv * bv
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
