package engine

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memoryStoreFake registra as leituras e devolve fatos/erro roteirizados.
type memoryStoreFake struct {
	Facts    []Fact
	Err      error
	ListUser string
	Lists    int
}

func (m *memoryStoreFake) Put(context.Context, Fact) error { return nil }

func (m *memoryStoreFake) List(_ context.Context, userID string) ([]Fact, error) {
	m.Lists++
	m.ListUser = userID
	return m.Facts, m.Err
}

func (m *memoryStoreFake) Delete(context.Context, string, string) error { return nil }

var _ MemoryStore = (*memoryStoreFake)(nil)

// retrieverFake registra consulta/usuário/k e devolve itens/erro roteirizados.
type retrieverFake struct {
	Items   []RetrievedItem
	Err     error
	Queries []string
	UserIDs []string
	Ks      []int
}

func (r *retrieverFake) Retrieve(_ context.Context, userID, query string, k int) ([]RetrievedItem, error) {
	r.Queries = append(r.Queries, query)
	r.UserIDs = append(r.UserIDs, userID)
	r.Ks = append(r.Ks, k)
	return r.Items, r.Err
}

var _ Retriever = (*retrieverFake)(nil)

// memoryHarness monta o harness com as portas de memória definidas pelo caso.
func memoryHarness(t *testing.T, mutate func(*Config)) *Harness {
	t.Helper()
	cfg := minimalConfig(t)
	mutate(&cfg)
	h, err := New(cfg)
	require.NoError(t, err)
	return h
}

// textoDaMensagem concatena as partes de texto de uma mensagem.
func textoDaMensagem(m Message) string {
	var b strings.Builder
	for _, part := range m.Parts {
		if part.Kind == PartText {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}

func TestMemoryMessagesSemPortasNaoInjetaNemLe(t *testing.T) {
	// Arrange
	h := memoryHarness(t, func(*Config) {})

	// Act
	messages := h.memoryMessages(context.Background(), &Session{ID: "s1", UserID: "u1"}, []Part{{Kind: PartText, Text: "oi"}})

	// Assert
	assert.Nil(t, messages, "sem porta injetada não há leitura nem injeção (FR-029)")
}

func TestMemoryMessagesInjetaFatosComProvenienciaNoUsuarioDaSessao(t *testing.T) {
	// Arrange
	store := &memoryStoreFake{Facts: []Fact{
		{ID: "f1", UserID: "u1", Kind: FactKindPreference, Text: "Prefere respostas curtas", Source: "user"},
		{ID: "f2", UserID: "u1", Kind: FactKindFact, Text: "Mora em São Paulo", Source: "session:s0"},
	}}
	h := memoryHarness(t, func(c *Config) { c.Memory = store })

	// Act
	messages := h.memoryMessages(context.Background(), &Session{ID: "s1", UserID: "u1"}, nil)

	// Assert
	require.Len(t, messages, 1)
	assert.Equal(t, RoleSystem, messages[0].Role, "fatos entram como mensagem de sistema")
	text := textoDaMensagem(messages[0])
	assert.Contains(t, text, "Prefere respostas curtas")
	assert.Contains(t, text, "Mora em São Paulo")
	assert.Contains(t, text, "fonte: user")
	assert.Contains(t, text, "fonte: session:s0")
	assert.Equal(t, "u1", store.ListUser, "memória é lida sempre escopada pelo usuário da sessão")
	assert.Equal(t, 1, store.Lists)
}

func TestMemoryMessagesRecuperaComCitacaoEUsaUltimoTextoComoConsulta(t *testing.T) {
	// Arrange
	retriever := &retrieverFake{Items: []RetrievedItem{
		{ID: "doc-1", Text: "O prazo é de 30 dias", Source: "faq.md", Score: 0.93},
	}}
	h := memoryHarness(t, func(c *Config) { c.Retriever = retriever })
	input := []Part{
		{Kind: PartText, Text: "primeiro"},
		{Kind: PartText, Text: "  qual é o prazo?  "},
	}

	// Act
	messages := h.memoryMessages(context.Background(), &Session{ID: "s1", UserID: "u1"}, input)

	// Assert
	require.Len(t, messages, 1)
	assert.Equal(t, RoleSystem, messages[0].Role)
	text := textoDaMensagem(messages[0])
	assert.Contains(t, text, "[doc-1]", "item recuperado é citado pelo id")
	assert.Contains(t, text, "faq.md")
	assert.Contains(t, text, "O prazo é de 30 dias")
	assert.Equal(t, []string{"qual é o prazo?"}, retriever.Queries)
	assert.Equal(t, []string{"u1"}, retriever.UserIDs)
	assert.Equal(t, []int{retrievalK}, retriever.Ks)
	assert.Equal(t, 5, retrievalK)
}

func TestMemoryMessagesFatosAntesDosTrechosRecuperados(t *testing.T) {
	// Arrange
	store := &memoryStoreFake{Facts: []Fact{{ID: "f1", UserID: "u1", Text: "fato", Source: "user"}}}
	retriever := &retrieverFake{Items: []RetrievedItem{{ID: "doc-1", Text: "trecho", Source: "faq.md"}}}
	h := memoryHarness(t, func(c *Config) {
		c.Memory = store
		c.Retriever = retriever
	})

	// Act
	messages := h.memoryMessages(context.Background(), &Session{ID: "s1", UserID: "u1"}, []Part{{Kind: PartText, Text: "consulta"}})

	// Assert
	require.Len(t, messages, 2)
	assert.Contains(t, textoDaMensagem(messages[0]), "fato")
	assert.Contains(t, textoDaMensagem(messages[1]), "trecho")
}

func TestMemoryMessagesSemConteudoNaoInjeta(t *testing.T) {
	// Arrange
	store := &memoryStoreFake{}
	retriever := &retrieverFake{}
	h := memoryHarness(t, func(c *Config) {
		c.Memory = store
		c.Retriever = retriever
	})

	// Act
	messages := h.memoryMessages(context.Background(), &Session{ID: "s1", UserID: "u1"}, []Part{{Kind: PartText, Text: "oi"}})

	// Assert
	assert.Nil(t, messages, "store vazio e retriever sem itens não geram mensagem")
}

func TestMemoryMessagesSemTextoNoInputNaoConsultaRetriever(t *testing.T) {
	// Arrange
	retriever := &retrieverFake{Items: []RetrievedItem{{ID: "doc-1", Text: "trecho", Source: "faq.md"}}}
	h := memoryHarness(t, func(c *Config) { c.Retriever = retriever })
	input := []Part{
		{Kind: PartToolResult, Result: &ToolResult{CallID: "c1"}},
		{Kind: PartText, Text: "   "},
	}

	// Act
	messages := h.memoryMessages(context.Background(), &Session{ID: "s1", UserID: "u1"}, input)

	// Assert
	assert.Nil(t, messages)
	assert.Empty(t, retriever.Queries, "sem texto do usuário não há consulta semântica")
}

func TestMemoryMessagesRedigeETruncaPeloRedactor(t *testing.T) {
	// Arrange
	textoLongo := strings.Repeat("x", 200)
	store := &memoryStoreFake{Facts: []Fact{{ID: "f1", UserID: "u1", Text: textoLongo, Source: "user"}}}
	h := memoryHarness(t, func(c *Config) {
		c.Memory = store
		c.Redaction.MaxFieldBytes = 64
	})

	// Act
	messages := h.memoryMessages(context.Background(), &Session{ID: "s1", UserID: "u1"}, nil)

	// Assert
	require.Len(t, messages, 1)
	text := textoDaMensagem(messages[0])
	assert.Contains(t, text, "…", "conteúdo acima do limite é truncado pelo Redactor")
	assert.NotContains(t, text, textoLongo)
	assert.Contains(t, text, "fonte: user", "proveniência continua visível")
}

func TestMemoryMessagesDegradaComErroDePortaERegistraCausa(t *testing.T) {
	// Arrange
	sentinela := errors.New("store fora do ar")
	var buf bytes.Buffer
	store := &memoryStoreFake{Err: sentinela}
	retriever := &retrieverFake{Items: []RetrievedItem{{ID: "doc-1", Text: "trecho", Source: "faq.md"}}}
	h := memoryHarness(t, func(c *Config) {
		c.Memory = store
		c.Retriever = retriever
		c.Logger = slog.New(slog.NewTextHandler(&buf, nil))
	})

	// Act
	messages := h.memoryMessages(context.Background(), &Session{ID: "s1", UserID: "u1"}, []Part{{Kind: PartText, Text: "consulta"}})

	// Assert
	require.Len(t, messages, 1, "a falha dos fatos não impede os trechos recuperados")
	assert.Contains(t, textoDaMensagem(messages[0]), "trecho")
	logged := buf.String()
	assert.Contains(t, logged, "harness: memória")
	assert.Contains(t, logged, "store fora do ar", "a causa é preservada no log")
}

func TestMemoryMessagesRetrieverIndisponivelSegueSemContexto(t *testing.T) {
	// Arrange
	var buf bytes.Buffer
	retriever := &retrieverFake{Err: errors.New("índice indisponível")}
	h := memoryHarness(t, func(c *Config) {
		c.Retriever = retriever
		c.Logger = slog.New(slog.NewTextHandler(&buf, nil))
	})

	// Act
	messages := h.memoryMessages(context.Background(), &Session{ID: "s1", UserID: "u1"}, []Part{{Kind: PartText, Text: "consulta"}})

	// Assert
	assert.Nil(t, messages, "turno segue sem contexto recuperado (CU-HAR-6 3a)")
	assert.Contains(t, buf.String(), "índice indisponível")
}
