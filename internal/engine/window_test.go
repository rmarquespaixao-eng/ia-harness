package engine

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// windowNow é o instante determinístico dos fixtures da janela (sem relógio real).
var windowNow = time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC)

// windowStubProvider é o provider inerte do harness de teste da janela.
type windowStubProvider struct{}

func (windowStubProvider) Chat(context.Context, ChatRequest, func(string)) (ChatResponse, error) {
	return ChatResponse{}, nil
}

func (windowStubProvider) Close() error { return nil }

// windowStubCredentials é o resolvedor de credencial inerte do teste da janela.
type windowStubCredentials struct{}

func (windowStubCredentials) Resolve(context.Context, string) (string, error) { return "", nil }

// windowStubSessions é o SessionStore inerte do harness de teste da janela.
type windowStubSessions struct{}

func (windowStubSessions) Load(context.Context, string) (*Session, error) { return nil, nil }
func (windowStubSessions) Save(context.Context, *Session) error           { return nil }

// windowTestHarness monta um Harness mínimo com a ContextPolicy e a premissa de
// bytes/token do cenário (stubs locais — sem depender de outros testes).
func windowTestHarness(t *testing.T, policy ContextPolicy, bytesPerToken float64) *Harness {
	t.Helper()
	h, err := New(Config{
		Providers:   map[string]Provider{"stub": windowStubProvider{}},
		Models:      map[string]ModelProfile{"default": {Provider: "stub", Model: "stub-1"}},
		Credentials: windowStubCredentials{},
		Sessions:    windowStubSessions{},
		Logger:      slog.Default(),
		Context:     policy,
		Pricing:     Pricing{BytesPerToken: bytesPerToken},
	})
	require.NoError(t, err)
	return h
}

// windowMessage cria uma mensagem textual com id e papel dados.
func windowMessage(id string, role Role, text string) Message {
	return Message{
		ID:        id,
		Role:      role,
		Parts:     []Part{{Kind: PartText, Text: text}},
		CreatedAt: windowNow,
	}
}

// windowSession devolve um histórico maior que qualquer orçamento dos testes:
// sistema + três mensagens antigas grandes.
func windowSession() *Session {
	return &Session{
		Messages: []Message{
			windowMessage("sys", RoleSystem, strings.Repeat("s", 20)),
			windowMessage("old-1", RoleUser, strings.Repeat("a", 400)),
			windowMessage("old-2", RoleAssistant, strings.Repeat("b", 400)),
			windowMessage("old-3", RoleUser, strings.Repeat("c", 200)),
		},
	}
}

// windowIDs extrai os ids na ordem, para asserções legíveis.
func windowIDs(messages []Message) []string {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	return ids
}

// windowFakeSummarizer registra o corte recebido e devolve o resumo roteirizado.
type windowFakeSummarizer struct {
	summary string
	err     error
	calls   int
	removed []Message
	budget  int
}

func (f *windowFakeSummarizer) Summarize(_ context.Context, messages []Message, maxTokens int) (string, error) {
	f.calls++
	f.removed = append([]Message(nil), messages...)
	f.budget = maxTokens
	return f.summary, f.err
}

func TestWindow_TruncateOldestPreservaSistemaEAtual(t *testing.T) {
	// Arrange — orçamento de 60 tokens (240 bytes); o total é 1060 bytes.
	h := windowTestHarness(t, ContextPolicy{MaxTokens: 60, Strategy: StrategyTruncateOldest}, 4)
	session := windowSession()
	input := []Part{{Kind: PartText, Text: strings.Repeat("d", 40)}}

	// Act
	messages := h.buildMessages(context.Background(), session, input)

	// Assert — sistema e entrada atual ficam; as antigas saem do mais velho.
	require.Len(t, messages, 2)
	assert.Equal(t, "sys", messages[0].ID)
	assert.Equal(t, RoleSystem, messages[0].Role)

	assert.Equal(t, RoleUser, messages[1].Role)
	assert.Equal(t, strings.Repeat("d", 40), messages[1].Parts[0].Text)
	assert.Equal(t, 15, h.estimateWindowTokens(messages), "restou sistema (20 bytes) + atual (40 bytes)")
}

func TestWindow_TruncateMantemEntradaMesmoAcimaDoOrcamento(t *testing.T) {
	// Arrange — orçamento de 1 token, menor que a própria entrada atual.
	h := windowTestHarness(t, ContextPolicy{MaxTokens: 1, Strategy: StrategyTruncateOldest}, 4)
	session := windowSession()

	// Act
	messages := h.buildMessages(context.Background(), session, []Part{{Kind: PartText, Text: "pergunta atual"}})

	// Assert — a entrada nunca é removida, mesmo estourando o orçamento.
	require.Len(t, messages, 2)
	assert.Equal(t, "sys", messages[0].ID)
	assert.Equal(t, RoleUser, messages[1].Role)
	assert.Equal(t, "pergunta atual", messages[1].Parts[0].Text)
}

func TestWindow_TruncatePreservaMensagemMaisRecenteSemEntrada(t *testing.T) {
	// Arrange — retomada sem entrada: a última do histórico também é protegida.
	h := windowTestHarness(t, ContextPolicy{MaxTokens: 30, Strategy: StrategyTruncateOldest}, 4)
	session := &Session{Messages: []Message{
		windowMessage("a", RoleUser, strings.Repeat("a", 400)),
		windowMessage("b", RoleAssistant, strings.Repeat("b", 400)),
		windowMessage("c", RoleTool, strings.Repeat("c", 8)),
	}}

	// Act
	messages := h.buildMessages(context.Background(), session, nil)

	// Assert — antigas saem; a mais recente permanece.
	require.Len(t, messages, 1)
	assert.Equal(t, "c", messages[0].ID)
}

func TestWindow_SemOrcamentoNaoTrunca(t *testing.T) {
	// Arrange — MaxTokens=0 desliga a janela (comportamento anterior).
	h := windowTestHarness(t, ContextPolicy{}, 4)
	session := windowSession()

	// Act
	messages := h.buildMessages(context.Background(), session, []Part{{Kind: PartText, Text: "oi"}})

	// Assert — histórico íntegro + entrada.
	require.Len(t, messages, len(session.Messages)+1)
	assert.Equal(t, session.Messages[0], messages[0])
	assert.Equal(t, "oi", messages[len(messages)-1].Parts[0].Text)
}

func TestWindow_SummarizeChamaUmaVezEInjetaResumoComProveniencia(t *testing.T) {
	// Arrange
	summarizer := &windowFakeSummarizer{summary: "o usuário pediu X"}
	h := windowTestHarness(t, ContextPolicy{MaxTokens: 60, Strategy: StrategySummarize, Summarizer: summarizer}, 4)
	session := windowSession()
	input := []Part{{Kind: PartText, Text: strings.Repeat("d", 40)}}

	// Act
	messages := h.buildMessages(context.Background(), session, input)

	// Assert — exatamente 1 chamada, com as removidas e o orçamento do turno.
	assert.Equal(t, 1, summarizer.calls, "summarizer roda no máximo 1×/turno")
	assert.Equal(t, 60, summarizer.budget)
	require.Len(t, summarizer.removed, 3)
	assert.Equal(t, []string{"old-1", "old-2", "old-3"}, windowIDs(summarizer.removed))

	// O resumo entra como sistema com proveniência, após o sistema preservado.
	require.Len(t, messages, 3)
	assert.Equal(t, "sys", messages[0].ID)
	assert.Equal(t, RoleSystem, messages[1].Role)
	assert.True(t, strings.HasPrefix(messages[1].Parts[0].Text, "resumo de histórico: "))
	assert.Contains(t, messages[1].Parts[0].Text, "o usuário pediu X")
	assert.Equal(t, RoleUser, messages[2].Role)
	assert.Equal(t, strings.Repeat("d", 40), messages[2].Parts[0].Text)
}

func TestWindow_SummarizeFalhaCaiParaTruncamento(t *testing.T) {
	// Arrange — summarizer devolve erro.
	summarizer := &windowFakeSummarizer{err: errors.New("sem quota")}
	h := windowTestHarness(t, ContextPolicy{MaxTokens: 60, Strategy: StrategySummarize, Summarizer: summarizer}, 4)

	// Act
	messages := h.buildMessages(context.Background(), windowSession(), []Part{{Kind: PartText, Text: strings.Repeat("d", 40)}})

	// Assert — truncamento íntegro e sem resumo injetado.
	assert.Equal(t, 1, summarizer.calls)
	require.Len(t, messages, 2)
	assert.Equal(t, "sys", messages[0].ID)
	for _, message := range messages {
		assert.NotContains(t, message.Parts[0].Text, "resumo de histórico")
	}
}

func TestWindow_SummarizeSemSummarizerTrunca(t *testing.T) {
	// Arrange — estratégia summarize sem a porta injetada.
	h := windowTestHarness(t, ContextPolicy{MaxTokens: 60, Strategy: StrategySummarize}, 4)

	// Act
	messages := h.buildMessages(context.Background(), windowSession(), []Part{{Kind: PartText, Text: strings.Repeat("d", 40)}})

	// Assert
	require.Len(t, messages, 2)
	assert.Equal(t, "sys", messages[0].ID)
	assert.Equal(t, RoleUser, messages[1].Role)
}

func TestWindow_EstimativaContaTextoArgsEJSON(t *testing.T) {
	// Arrange — 40 (texto) + 11 (args) + 20 (texto) + 7 (JSON) = 78 bytes.
	h := windowTestHarness(t, ContextPolicy{}, 4)
	message := Message{ID: "m", Role: RoleAssistant, Parts: []Part{
		{Kind: PartText, Text: strings.Repeat("x", 40)},
		{Kind: PartToolCall, Call: &ToolCall{ID: "call", Name: "buscar", Args: []byte(`{"q":"abc"}`)}},
		{Kind: PartToolResult, Result: &ToolResult{CallID: "call", Content: []ResultContent{
			{Kind: ResultText, Text: strings.Repeat("y", 20)},
			{Kind: ResultJSON, JSON: []byte(`{"r":1}`)},
		}}},
	}}

	// Act + Assert
	assert.Equal(t, 20, h.estimateWindowTokens([]Message{message}))
}

func TestContextBudget_DerivaDaCapacidadeDoModelo(t *testing.T) {
	// Arrange — política global de 500 e margem de 100.
	h := windowTestHarness(t, ContextPolicy{MaxTokens: 500, SafetyMargin: 100}, 4)

	// Act + Assert — a capacidade do modelo vence e reserva saída + margem.
	budget, fromModel := h.contextBudget(ModelProfile{Capabilities: Capabilities{MaxContextTokens: 1000, MaxOutputTokens: 200}})
	assert.True(t, fromModel, "orçamento veio do modelo")
	assert.Equal(t, 700, budget)

	// Sem capacidade, cai para a política global (legado).
	fallback, fromModel := h.contextBudget(ModelProfile{})
	assert.False(t, fromModel)
	assert.Equal(t, 500, fallback)

	// Sem capacidade e sem política global, a janela fica desligada.
	off := windowTestHarness(t, ContextPolicy{}, 4)
	zero, _ := off.contextBudget(ModelProfile{})
	assert.Zero(t, zero)
}

func TestWindow_PorModeloCompactaNoGatilhoDe80(t *testing.T) {
	// Arrange — orçamento 100; gatilho 0,8 dispara com ~201 tokens.
	h := windowTestHarness(t, ContextPolicy{}, 4)
	messages := []Message{
		windowMessage("a", RoleUser, strings.Repeat("a", 400)),
		windowMessage("b", RoleAssistant, strings.Repeat("b", 400)),
		windowMessage("now", RoleUser, "oi"),
	}

	// Act
	kept, info := h.applyWindow(context.Background(), &Session{}, messages, 0, 100, 0.8, false, "")

	// Assert — compactou até caber, preservando o mais recente.
	require.NotNil(t, info)
	assert.Equal(t, []string{"now"}, windowIDs(kept))
	assert.False(t, info.Summarized)
	assert.Greater(t, info.TokensBefore, info.TokensAfter)
	assert.LessOrEqual(t, info.TokensAfter, 100)
}

func TestWindow_RemoveBlocoDeToolInteiroSemOrfao(t *testing.T) {
	// Arrange — bloco assistant(tool_call) + tool_result grande entre antigas.
	h := windowTestHarness(t, ContextPolicy{}, 4)
	call := Message{ID: "call", Role: RoleAssistant, Parts: []Part{
		{Kind: PartToolCall, Call: &ToolCall{ID: "c1", Name: "t", Args: []byte("{}")}},
	}}
	result := Message{ID: "res", Role: RoleTool, Parts: []Part{
		{Kind: PartToolResult, Result: &ToolResult{CallID: "c1", Content: []ResultContent{
			{Kind: ResultText, Text: strings.Repeat("z", 400)},
		}}},
	}}
	messages := []Message{
		windowMessage("old", RoleUser, strings.Repeat("a", 400)),
		call, result,
		windowMessage("now", RoleUser, "oi"),
	}

	// Act — orçamento apertado força remover o bloco inteiro.
	kept, info := h.applyWindow(context.Background(), &Session{}, messages, 0, 30, 1, false, "")

	// Assert — chamada e resultado saíram juntos; nenhum resultado órfão.
	require.NotNil(t, info)
	assert.Equal(t, []string{"now"}, windowIDs(kept))
	assert.Equal(t, 3, info.MessagesRemoved)
	for i, message := range kept {
		if messageHasToolResult(message) {
			require.Greater(t, i, 0, "resultado de tool não pode liderar o contexto")
			assert.True(t, messageHasToolCall(kept[i-1]), "resultado precisa da chamada anterior")
		}
	}
}

func TestWindow_EstimativasContamSystemETools(t *testing.T) {
	// Arrange
	h := windowTestHarness(t, ContextPolicy{}, 4)

	// Act + Assert — vazio é zero; conteúdo vira estimativa positiva.
	assert.Zero(t, h.estimateTextTokens(""))
	assert.Zero(t, h.estimateToolsTokens(nil))
	assert.Positive(t, h.estimateTextTokens(strings.Repeat("y", 40)))
	assert.Positive(t, h.estimateToolsTokens([]Tool{
		{Name: "t", Description: "d", InputSchema: []byte(strings.Repeat("x", 40)), Namespace: "n"},
	}))
}

// modelAwareSummarizer implementa Summarizer e ModelSummarizer; registra o modelo recebido.
type modelAwareSummarizer struct {
	model   string
	summary string
}

func (m *modelAwareSummarizer) Summarize(context.Context, []Message, int) (string, error) {
	return "fallback", nil
}

func (m *modelAwareSummarizer) SummarizeForModel(_ context.Context, model string, _ []Message, _ int) (string, error) {
	m.model = model
	return m.summary, nil
}

func TestWindow_ModelSummarizerRecebeOAlisDoTurno(t *testing.T) {
	// Arrange
	summarizer := &modelAwareSummarizer{summary: "resumo"}
	h := windowTestHarness(t, ContextPolicy{Strategy: StrategySummarize, Summarizer: summarizer}, 4)
	messages := []Message{
		windowMessage("a", RoleUser, strings.Repeat("a", 400)),
		windowMessage("b", RoleAssistant, strings.Repeat("b", 400)),
		windowMessage("now", RoleUser, "oi"),
	}

	// Act
	kept, info := h.applyWindow(context.Background(), &Session{}, messages, 0, 100, 0.8, true, "go-luna")

	// Assert — o resumo usa o alias do modelo do perfil.
	require.NotNil(t, info)
	assert.True(t, info.Summarized)
	assert.Equal(t, "go-luna", summarizer.model)
	require.Len(t, kept, 2)
	assert.Equal(t, RoleSystem, kept[0].Role, "resumo injetado como sistema")
	assert.Equal(t, "now", kept[1].ID, "mensagem recente preservada")
}
