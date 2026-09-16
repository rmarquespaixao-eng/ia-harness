package harness_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmarquespaixao/ia-harness/adapters/memory/inmem"
	"rmarquespaixao/ia-harness/harness"
	"rmarquespaixao/ia-harness/internal/testutil"
)

// errSessionNotFound marca sessão ausente no store fake.
var errSessionNotFound = errors.New("sessão não encontrada")

// fixedClock devolve sempre o mesmo instante (inmem.New determinístico).
type fixedClock struct{ now time.Time }

// Now devolve o instante fixo.
func (c fixedClock) Now() time.Time { return c.now }

// memSessions é um harness.SessionStore em memória para os testes de uso.
type memSessions struct {
	sessions map[string]*harness.Session
}

// newMemSessions cria o store de sessões vazio.
func newMemSessions() *memSessions {
	return &memSessions{sessions: map[string]*harness.Session{}}
}

// Load devolve a sessão salva ou erro quando ausente.
func (s *memSessions) Load(_ context.Context, sessionID string) (*harness.Session, error) {
	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, errSessionNotFound
	}
	return session, nil
}

// Save guarda a sessão pelo ID.
func (s *memSessions) Save(_ context.Context, session *harness.Session) error {
	s.sessions[session.ID] = session
	return nil
}

var _ harness.SessionStore = (*memSessions)(nil)

// fakeEmbedder devolve vetores 2-D fixos por texto (determinístico).
type fakeEmbedder struct{ vectors map[string][]float32 }

// Embed implementa harness.Embedder sem rede.
func (e *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		if vector, ok := e.vectors[text]; ok {
			out[i] = vector
			continue
		}
		out[i] = []float32{0, 1}
	}
	return out, nil
}

var _ harness.Embedder = (*fakeEmbedder)(nil)

// memoryConfig monta uma Config válida e mínima para os cenários do CU-HAR-6.
func memoryConfig(t *testing.T, provider harness.Provider, clk harness.Clock) harness.Config {
	t.Helper()
	return harness.Config{
		Providers: map[string]harness.Provider{"scripted": provider},
		Models:    map[string]harness.ModelProfile{"default": {Provider: "scripted", Model: "modelo-teste"}},
		Credentials: &testutil.FakeCredentialProvider{
			Values: map[string]string{},
		},
		Sessions: newMemSessions(),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Clock:    clk,
	}
}

// runMemoryTurn executa um turno de uma nova sessão para o usuário.
func runMemoryTurn(t *testing.T, cfg harness.Config, userID, input string) harness.TurnResult {
	t.Helper()
	h, err := harness.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })

	result, err := h.Run(context.Background(), harness.RunRequest{
		UserID: userID,
		Model:  "default",
		Input:  []harness.Part{{Kind: harness.PartText, Text: input}},
	}, nil)
	require.NoError(t, err)
	return result
}

// systemTexts devolve o texto das mensagens de sistema enviadas ao modelo.
func systemTexts(messages []harness.Message) []string {
	var out []string
	for _, message := range messages {
		if message.Role != harness.RoleSystem {
			continue
		}
		var b strings.Builder
		for _, part := range message.Parts {
			if part.Kind == harness.PartText {
				b.WriteString(part.Text)
			}
		}
		out = append(out, b.String())
	}
	return out
}

// systemText concatena as mensagens de sistema do request em um texto único.
func systemText(messages []harness.Message) string {
	return strings.Join(systemTexts(messages), "\n")
}

func TestCU_HAR6_FatoGravadoInfluenciaNovaSessao(t *testing.T) {
	// Dado um fato gravado para o usuário
	clk := fixedClock{now: time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)}
	store := inmem.New(clk)
	require.NoError(t, store.Put(context.Background(), harness.Fact{
		UserID: "u1",
		Kind:   harness.FactKindPreference,
		Text:   "Prefere respostas curtas",
		Source: "user",
	}))
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "ok"}}}
	cfg := memoryConfig(t, provider, clk)
	cfg.Memory = store

	// Quando uma nova sessão é criada
	result := runMemoryTurn(t, cfg, "u1", "olá")

	// Então o fato é injetado no contexto com sua proveniência
	require.NotEmpty(t, result.SessionID)
	require.Len(t, provider.Requests, 1)
	contexto := systemText(provider.Requests[0].Messages)
	assert.Contains(t, contexto, "Prefere respostas curtas")
	assert.Contains(t, contexto, "fonte: user")
}

func TestCU_HAR6_RecuperacaoSemanticaCitaFonte(t *testing.T) {
	// Dado conteúdo indexado pelo host para o usuário
	clk := fixedClock{now: time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)}
	embedder := &fakeEmbedder{vectors: map[string][]float32{
		"O prazo de devolução é de 30 dias": {1, 0},
		"qual é o prazo de devolução?":      {1, 0},
	}}
	retriever := inmem.NewRetriever(embedder, map[string][]harness.RetrievedItem{
		"u1": {{ID: "doc-1", Text: "O prazo de devolução é de 30 dias", Source: "faq.md"}},
	})
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "30 dias"}}}
	cfg := memoryConfig(t, provider, clk)
	cfg.Retriever = retriever

	// Quando a pergunta é semanticamente relacionada
	runMemoryTurn(t, cfg, "u1", "qual é o prazo de devolução?")

	// Então os itens relevantes entram no contexto citando a fonte
	require.Len(t, provider.Requests, 1)
	contexto := systemText(provider.Requests[0].Messages)
	assert.Contains(t, contexto, "[doc-1]")
	assert.Contains(t, contexto, "faq.md")
	assert.Contains(t, contexto, "O prazo de devolução é de 30 dias")
}

func TestCU_HAR6_EsquecerRemoveDoContexto(t *testing.T) {
	// Dado um fato gravado
	clk := fixedClock{now: time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)}
	store := inmem.New(clk)
	require.NoError(t, store.Put(context.Background(), harness.Fact{
		ID: "f1", UserID: "u1", Text: "segredo antigo", Source: "user",
	}))

	// Quando o usuário pede para esquecer
	require.NoError(t, store.Delete(context.Background(), "u1", "f1"))

	// Então o fato deixa de ser recuperado
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "ok"}}}
	cfg := memoryConfig(t, provider, clk)
	cfg.Memory = store
	runMemoryTurn(t, cfg, "u1", "olá")

	require.Len(t, provider.Requests, 1)
	assert.Empty(t, systemTexts(provider.Requests[0].Messages), "sem fatos, nenhuma mensagem de memória")
	assert.NotContains(t, systemText(provider.Requests[0].Messages), "segredo antigo")
}

func TestCU_HAR6_IsolamentoEntreUsuarios(t *testing.T) {
	// Dado um fato gravado para u1
	clk := fixedClock{now: time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)}
	store := inmem.New(clk)
	require.NoError(t, store.Put(context.Background(), harness.Fact{
		UserID: "u1", Text: "segredo de u1", Source: "user",
	}))
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "ok"}, {Text: "ok"}}}
	cfg := memoryConfig(t, provider, clk)
	cfg.Memory = store

	// Quando u1 conversa, o fato aparece
	runMemoryTurn(t, cfg, "u1", "olá")
	// E quando u2 conversa, o fato de u1 não aparece
	runMemoryTurn(t, cfg, "u2", "olá")

	// Então o contexto é sempre escopado pelo usuário da sessão
	require.Len(t, provider.Requests, 2)
	assert.Contains(t, systemText(provider.Requests[0].Messages), "segredo de u1")
	assert.NotContains(t, systemText(provider.Requests[1].Messages), "segredo de u1")
	assert.Empty(t, systemTexts(provider.Requests[1].Messages))
}
