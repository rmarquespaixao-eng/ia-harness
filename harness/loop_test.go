package harness_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/adapters/session/memory"
	"github.com/rmarquespaixao-eng/ia-harness/harness"
	"github.com/rmarquespaixao-eng/ia-harness/internal/testutil"
)

// Identificadores usados pelos fakes e pelo RunRequest dos testes.
const (
	turnProviderKey = "fake"
	turnModelAlias  = "default"
	turnUserID      = "user-1"
	turnAgentID     = "agent-1"
)

// turnToolSchema é o input schema simples compartilhado pelas tools do catálogo.
var turnToolSchema = json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}},"required":["q"]}`)

// turnEnv reúne o harness e os fakes injetados por DI; nenhum caso de teste toca rede.
type turnEnv struct {
	h        *harness.Harness
	provider *testutil.ScriptedProvider
	tools    *testutil.MemToolSource
	store    *memory.Store
	audit    *testutil.FakeAuditSink
	creds    *testutil.FakeCredentialProvider
	logs     *bytes.Buffer
}

// newTurnEnv monta o harness com provider roteirizado, catálogo em memória,
// adapters/session/memory, auditoria/credenciais fakes e log JSON em buffer.
func newTurnEnv(t *testing.T, script []testutil.ProviderStep) *turnEnv {
	t.Helper()

	resultado := func(texto string) harness.ToolResult {
		return harness.ToolResult{Content: []harness.ResultContent{{Kind: harness.ResultText, Text: texto}}}
	}
	env := &turnEnv{
		provider: &testutil.ScriptedProvider{Steps: script},
		tools: &testutil.MemToolSource{
			Tools: []harness.Tool{
				{Name: "consultar_saldo", Description: "consulta o saldo", InputSchema: turnToolSchema},
				{Name: "consultar_limite", Description: "consulta o limite", InputSchema: turnToolSchema},
				{Name: "executar_lenta", Description: "tool longa com progresso", InputSchema: turnToolSchema},
			},
			Results: map[string]harness.ToolResult{
				"consultar_saldo":  resultado("saldo: R$ 1.234,56"),
				"consultar_limite": resultado("limite: R$ 5.000,00"),
				"executar_lenta":   resultado("tarefa concluída"),
			},
		},
		store: memory.New(),
		audit: &testutil.FakeAuditSink{},
		creds: &testutil.FakeCredentialProvider{Values: map[string]string{"env:FAKE_API_KEY": "valor-de-teste"}},
		logs:  &bytes.Buffer{},
	}

	cfg := harness.Config{
		Providers: map[string]harness.Provider{turnProviderKey: env.provider},
		Models: map[string]harness.ModelProfile{
			turnModelAlias: {
				Provider: turnProviderKey,
				Model:    "fake-1",
				Capabilities: harness.Capabilities{
					ToolCalling:      true,
					Streaming:        true,
					MaxContextTokens: 8192,
					MaxOutputTokens:  1024,
				},
			},
		},
		Tools:        []harness.ToolSource{env.tools},
		Credentials:  env.creds,
		Sessions:     env.store,
		Logger:       slog.New(slog.NewJSONHandler(env.logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		Audit:        env.audit,
		DefaultModel: turnModelAlias,
		// CU-HAR-3: o default do núcleo é deny; os testes do loop concedem allow explícito.
		Policy: harness.PolicyConfig{Default: harness.PolicyAllow},
	}
	h, err := harness.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, h.Close()) })
	env.h = h
	return env
}

// turnHandler coleta os eventos do turno na ordem de emissão e permite
// interceptar um evento específico (ex.: cancelar o contexto).
type turnHandler struct {
	harness.NopHandler

	deltas      []harness.TextDelta
	toolCalls   []harness.ToolCallEvent
	toolResults []harness.ToolResultEvent
	progress    []harness.ProgressEvent
	usages      []harness.UsageEvent
	errs        []harness.ErrorEvent
	order       []string

	onToolResult func(harness.ToolResultEvent)
}

func (h *turnHandler) TextDelta(_ context.Context, ev harness.TextDelta) {
	h.order = append(h.order, "text_delta")
	h.deltas = append(h.deltas, ev)
}

func (h *turnHandler) ToolCall(_ context.Context, ev harness.ToolCallEvent) {
	h.order = append(h.order, "tool_call:"+ev.Tool)
	h.toolCalls = append(h.toolCalls, ev)
}

func (h *turnHandler) ToolResult(_ context.Context, ev harness.ToolResultEvent) {
	h.order = append(h.order, "tool_result:"+ev.Tool)
	h.toolResults = append(h.toolResults, ev)
	if h.onToolResult != nil {
		h.onToolResult(ev)
	}
}

func (h *turnHandler) Progress(_ context.Context, ev harness.ProgressEvent) {
	h.order = append(h.order, "progress:"+ev.Tool)
	h.progress = append(h.progress, ev)
}

func (h *turnHandler) Usage(_ context.Context, ev harness.UsageEvent) {
	h.order = append(h.order, "usage")
	h.usages = append(h.usages, ev)
}

func (h *turnHandler) Error(_ context.Context, ev harness.ErrorEvent) {
	h.order = append(h.order, "error:"+string(ev.Scope))
	h.errs = append(h.errs, ev)
}

// pergunta monta o RunRequest padrão dos testes do loop.
func pergunta(texto string) harness.RunRequest {
	return harness.RunRequest{
		UserID:  turnUserID,
		AgentID: turnAgentID,
		Model:   turnModelAlias,
		Input:   []harness.Part{{Kind: harness.PartText, Text: texto}},
	}
}

// toolCall monta a chamada de tool pedida pelo modelo no roteiro do provider.
func toolCall(id, nome, args string) harness.ToolCall {
	return harness.ToolCall{ID: id, Name: nome, Args: json.RawMessage(args)}
}

// toolResultParts extrai os resultados de tool persistidos no histórico.
func toolResultParts(sessao *harness.Session) []harness.ToolResult {
	var resultados []harness.ToolResult
	for _, mensagem := range sessao.Messages {
		for _, parte := range mensagem.Parts {
			if parte.Kind == harness.PartToolResult && parte.Result != nil {
				resultados = append(resultados, *parte.Result)
			}
		}
	}
	return resultados
}

// assistantTexts extrai os textos das mensagens do assistente no histórico.
func assistantTexts(sessao *harness.Session) []string {
	var textos []string
	for _, mensagem := range sessao.Messages {
		if mensagem.Role != harness.RoleAssistant {
			continue
		}
		for _, parte := range mensagem.Parts {
			if parte.Kind == harness.PartText {
				textos = append(textos, parte.Text)
			}
		}
	}
	return textos
}

// TestRun_ToolComSucesso_TurnoCompleto cobre o fluxo principal do CU-HAR-1:
// streaming incremental, tool validada/executada, uso acumulado e sessão salva.
func TestRun_ToolComSucesso_TurnoCompleto(t *testing.T) {
	// Arrange
	handler := &turnHandler{}
	env := newTurnEnv(t, []testutil.ProviderStep{
		{
			Text:      "Consultando o saldo.",
			ToolCalls: []harness.ToolCall{toolCall("call-1", "consultar_saldo", `{"q":"saldo"}`)},
			Usage:     harness.Usage{InputTokens: 11, OutputTokens: 3},
		},
		{Text: "Seu saldo é R$ 1.234,56.", Usage: harness.Usage{InputTokens: 7, OutputTokens: 4}},
	})

	// Act
	result, err := env.h.Run(context.Background(), pergunta("qual o meu saldo?"), handler)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, harness.StopCompleted, result.StopReason)
	assert.Equal(t, harness.SessionActive, result.State)

	// TextDelta incremental: cada resposta é entregue em fragmentos com o mesmo MessageID.
	require.Len(t, handler.deltas, 4, "duas respostas fragmentadas em duas partes")
	assert.Equal(t, "Consultando o saldo.", handler.deltas[0].Text+handler.deltas[1].Text)
	assert.Equal(t, handler.deltas[0].MessageID, handler.deltas[1].MessageID)
	assert.Equal(t, "Seu saldo é R$ 1.234,56.", handler.deltas[2].Text+handler.deltas[3].Text)
	assert.Equal(t, handler.deltas[2].MessageID, handler.deltas[3].MessageID)
	assert.NotEqual(t, handler.deltas[0].MessageID, handler.deltas[2].MessageID)
	for _, delta := range handler.deltas {
		assert.Equal(t, result.SessionID, delta.SessionID)
	}

	// ToolCallEvent com args redigidos (FR-024) e bytes originais.
	require.Len(t, handler.toolCalls, 1)
	assert.Equal(t, "call-1", handler.toolCalls[0].CallID)
	assert.Equal(t, "consultar_saldo", handler.toolCalls[0].Tool)
	assert.JSONEq(t, `{"q":"saldo"}`, handler.toolCalls[0].ArgsRedacted)
	assert.Equal(t, len(`{"q":"saldo"}`), handler.toolCalls[0].ArgsBytes)

	// Resultado ok e uso somado no TurnResult.
	require.Len(t, handler.toolResults, 1)
	assert.Equal(t, harness.StatusOK, handler.toolResults[0].Status)
	assert.False(t, handler.toolResults[0].IsError)
	assert.Contains(t, handler.toolResults[0].ResultSummary, "R$ 1.234,56")

	require.Len(t, handler.usages, 2)
	assert.Equal(t, int64(11), handler.usages[0].InputTokens)
	assert.Equal(t, int64(7), handler.usages[1].InputTokens)
	assert.Equal(t, int64(18), result.Usage.InputTokens)
	assert.Equal(t, int64(7), result.Usage.OutputTokens)

	// A tool executou exatamente uma vez com os argumentos validados.
	require.Len(t, env.tools.Calls, 1)
	assert.Equal(t, "consultar_saldo", env.tools.Calls[0].Name)
	assert.JSONEq(t, `{"q":"saldo"}`, string(env.tools.Calls[0].Args))
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, harness.StatusOK, result.ToolCalls[0].Status)

	// Sessão persistida com o histórico do turno.
	sessao, err := env.h.Session(context.Background(), result.SessionID)
	require.NoError(t, err)
	assert.Equal(t, harness.SessionActive, sessao.State)
	require.NotEmpty(t, sessao.Messages)
	resultados := toolResultParts(sessao)
	require.Len(t, resultados, 1)
	assert.Equal(t, "call-1", resultados[0].CallID)
	assert.False(t, resultados[0].IsError)
	assert.Contains(t, resultados[0].Content[0].Text, "R$ 1.234,56")
	assert.Contains(t, assistantTexts(sessao), "Seu saldo é R$ 1.234,56.")
	assert.Equal(t, int64(18), sessao.Usage.InputTokens)
}

// TestRun_ArgumentoInvalido_NaoExecutaTool cobre o desvio 4a do CU-HAR-1.
func TestRun_ArgumentoInvalido_NaoExecutaTool(t *testing.T) {
	// Arrange — a segunda etapa encerra o turno após o erro devolvido ao modelo.
	handler := &turnHandler{}
	env := newTurnEnv(t, []testutil.ProviderStep{
		{ToolCalls: []harness.ToolCall{toolCall("call-invalida", "consultar_saldo", `{"r":"fora do schema"}`)}},
		{Text: "Não consegui consultar."},
	})

	// Act
	result, err := env.h.Run(context.Background(), pergunta("qual o saldo?"), handler)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, harness.StopCompleted, result.StopReason)
	assert.Empty(t, env.tools.Calls, "argumento inválido nunca chega ao ToolSource")

	require.Len(t, handler.toolCalls, 1)
	assert.JSONEq(t, `{"r":"fora do schema"}`, handler.toolCalls[0].ArgsRedacted)

	require.Len(t, handler.toolResults, 1)
	assert.Equal(t, harness.StatusError, handler.toolResults[0].Status)
	assert.True(t, handler.toolResults[0].IsError)
	assert.Contains(t, handler.toolResults[0].ResultSummary, "argumentos inválidos")

	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, harness.StatusError, result.ToolCalls[0].Status)

	// O erro voltou ao modelo como mensagem de tool.
	sessao, err := env.h.Session(context.Background(), result.SessionID)
	require.NoError(t, err)
	resultados := toolResultParts(sessao)
	require.Len(t, resultados, 1)
	assert.True(t, resultados[0].IsError)
	assert.Contains(t, resultados[0].Content[0].Text, "argumentos inválidos")
}

// TestRun_ToolDesconhecida_ErroVoltaAoModelo cobre o desvio 4b do CU-HAR-1.
func TestRun_ToolDesconhecida_ErroVoltaAoModelo(t *testing.T) {
	// Arrange
	handler := &turnHandler{}
	env := newTurnEnv(t, []testutil.ProviderStep{
		{ToolCalls: []harness.ToolCall{toolCall("call-fantasma", "nao_existe", `{"q":"x"}`)}},
		{Text: "Tentei, mas a tool não existe."},
	})

	// Act
	result, err := env.h.Run(context.Background(), pergunta("consulte"), handler)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, harness.StopCompleted, result.StopReason)
	assert.Empty(t, env.tools.Calls, "tool desconhecida não executa nada")

	require.Len(t, handler.toolResults, 1)
	assert.Equal(t, harness.StatusError, handler.toolResults[0].Status)
	assert.True(t, handler.toolResults[0].IsError)
	assert.Contains(t, handler.toolResults[0].ResultSummary, "tool desconhecida")

	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, harness.StatusError, result.ToolCalls[0].Status)

	// O erro voltou ao modelo como mensagem de tool com IsError.
	sessao, err := env.h.Session(context.Background(), result.SessionID)
	require.NoError(t, err)
	resultados := toolResultParts(sessao)
	require.Len(t, resultados, 1)
	assert.True(t, resultados[0].IsError)
	assert.Contains(t, resultados[0].Content[0].Text, "tool desconhecida")
}

// TestRun_ToolLonga_ProgressoAntesDoResultado cobre o passo 5 do CU-HAR-1.
func TestRun_ToolLonga_ProgressoAntesDoResultado(t *testing.T) {
	// Arrange
	handler := &turnHandler{}
	env := newTurnEnv(t, []testutil.ProviderStep{
		{ToolCalls: []harness.ToolCall{toolCall("call-lenta", "executar_lenta", `{"q":"tudo"}`)}},
		{Text: "Pronto."},
	})
	env.tools.Progress = map[string][]harness.ProgressUpdate{
		"executar_lenta": {
			{Message: "metade", Progress: 1, Total: 2},
			{Message: "fim", Progress: 2, Total: 2},
		},
	}

	// Act
	result, err := env.h.Run(context.Background(), pergunta("rode a tarefa"), handler)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, harness.StopCompleted, result.StopReason)

	require.Len(t, handler.progress, 2)
	assert.Equal(t, "call-lenta", handler.progress[0].CallID)
	assert.Equal(t, "executar_lenta", handler.progress[0].Tool)
	assert.Equal(t, "metade", handler.progress[0].Message)
	assert.Equal(t, 1.0, handler.progress[0].Progress)
	assert.Equal(t, 2.0, handler.progress[0].Total)

	indiceProgresso := slices.Index(handler.order, "progress:executar_lenta")
	indiceResultado := slices.Index(handler.order, "tool_result:executar_lenta")
	require.NotEqual(t, -1, indiceProgresso)
	require.NotEqual(t, -1, indiceResultado)
	assert.Less(t, indiceProgresso, indiceResultado, "progresso chega antes do resultado final")

	ultimoProgresso := -1
	for i, evento := range handler.order {
		if evento == "progress:executar_lenta" {
			ultimoProgresso = i
		}
	}
	assert.Less(t, ultimoProgresso, indiceResultado)

	require.Len(t, handler.toolResults, 1)
	assert.Equal(t, harness.StatusOK, handler.toolResults[0].Status)
}

// TestRun_DuasToolCalls_ExecutamNaOrdem exige execução sequencial determinística
// (D-10): duas chamadas no mesmo turno rodam na ordem do roteiro.
func TestRun_DuasToolCalls_ExecutamNaOrdem(t *testing.T) {
	// Arrange
	handler := &turnHandler{}
	env := newTurnEnv(t, []testutil.ProviderStep{
		{
			Text: "Vou consultar saldo e limite.",
			ToolCalls: []harness.ToolCall{
				toolCall("call-saldo", "consultar_saldo", `{"q":"saldo"}`),
				toolCall("call-limite", "consultar_limite", `{"q":"limite"}`),
			},
		},
		{Text: "Saldo e limite consultados."},
	})

	// Act
	result, err := env.h.Run(context.Background(), pergunta("saldo e limite"), handler)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, harness.StopCompleted, result.StopReason)

	require.Len(t, env.tools.Calls, 2)
	assert.Equal(t, "consultar_saldo", env.tools.Calls[0].Name)
	assert.Equal(t, "consultar_limite", env.tools.Calls[1].Name)

	require.Len(t, handler.toolCalls, 2)
	assert.Equal(t, "call-saldo", handler.toolCalls[0].CallID)
	assert.Equal(t, "call-limite", handler.toolCalls[1].CallID)

	assert.Less(t, slices.Index(handler.order, "tool_call:consultar_saldo"), slices.Index(handler.order, "tool_result:consultar_saldo"))
	assert.Less(t, slices.Index(handler.order, "tool_result:consultar_saldo"), slices.Index(handler.order, "tool_call:consultar_limite"))
	assert.Less(t, slices.Index(handler.order, "tool_call:consultar_limite"), slices.Index(handler.order, "tool_result:consultar_limite"))

	require.Len(t, result.ToolCalls, 2)
	assert.Equal(t, harness.StatusOK, result.ToolCalls[0].Status)
	assert.Equal(t, harness.StatusOK, result.ToolCalls[1].Status)
}

// TestRun_Cancelamento_SessaoPersistida cobre o desvio 6a do CU-HAR-1: o host
// cancela o contexto no handler e o turno encerra com a sessão persistida.
func TestRun_Cancelamento_SessaoPersistida(t *testing.T) {
	// Arrange
	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()
	handler := &turnHandler{onToolResult: func(harness.ToolResultEvent) { cancelar() }}
	env := newTurnEnv(t, []testutil.ProviderStep{
		{ToolCalls: []harness.ToolCall{toolCall("call-1", "consultar_saldo", `{"q":"saldo"}`)}},
		{Text: "não deve ser chamado"},
	})

	// Act
	result, err := env.h.Run(ctx, pergunta("qual o saldo?"), handler)

	// Assert
	assert.Equal(t, harness.StopCancelled, result.StopReason)
	assert.NoError(t, err, "Run encerra o turno cancelado salvando a sessão (CU-HAR-1 6a)")
	assert.Equal(t, 1, env.provider.Calls, "o modelo não é chamado de novo após o cancelamento")

	sessao, errCarga := env.h.Session(context.Background(), result.SessionID)
	require.NoError(t, errCarga, "sessão persistida no ponto consistente")
	assert.Equal(t, harness.SessionActive, sessao.State)
	assert.NotEmpty(t, toolResultParts(sessao), "o ponto consistente inclui o resultado da tool")
}

// TestRun_MaxIterations_EncerraComStopReason cobre o desvio 5b do CU-HAR-1.
func TestRun_MaxIterations_EncerraComStopReason(t *testing.T) {
	// Arrange — roteiro com duas rodadas, mas o orçamento permite só uma.
	handler := &turnHandler{}
	env := newTurnEnv(t, []testutil.ProviderStep{
		{ToolCalls: []harness.ToolCall{toolCall("call-1", "consultar_saldo", `{"q":"saldo"}`)}},
		{Text: "segunda rodada"},
	})
	req := pergunta("saldo?")
	req.MaxIterations = 1

	// Act
	result, err := env.h.Run(context.Background(), req, handler)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, harness.StopMaxIterations, result.StopReason)
	assert.Equal(t, 1, env.provider.Calls, "a segunda rodada não acontece")
	require.Len(t, env.tools.Calls, 1, "a tool da primeira rodada executou antes do limite")
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, harness.StatusOK, result.ToolCalls[0].Status)
}

// TestRun_FalhaDeTool_ErroMCPESessaoSegue cobre o desvio 5a do CU-HAR-1:
// falha de transporte vira resultado de erro + ErrorEvent(mcp), sem derrubar.
func TestRun_FalhaDeTool_ErroMCPESessaoSegue(t *testing.T) {
	// Arrange
	handler := &turnHandler{}
	env := newTurnEnv(t, []testutil.ProviderStep{
		{ToolCalls: []harness.ToolCall{toolCall("call-erro", "consultar_saldo", `{"q":"saldo"}`)}},
		{Text: "Não consegui consultar agora."},
	})
	env.tools.Errors = map[string]error{"consultar_saldo": errors.New("transporte caiu")}

	// Act
	result, err := env.h.Run(context.Background(), pergunta("qual o saldo?"), handler)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, harness.StopCompleted, result.StopReason)
	assert.Equal(t, 2, env.provider.Calls, "o turno continua após a falha da tool")

	require.Len(t, handler.errs, 1)
	assert.Equal(t, harness.ErrorMCP, handler.errs[0].Scope)
	assert.NotEmpty(t, handler.errs[0].Message)

	require.Len(t, handler.toolResults, 1)
	assert.Equal(t, harness.StatusError, handler.toolResults[0].Status)
	assert.True(t, handler.toolResults[0].IsError)
	assert.Contains(t, handler.toolResults[0].ResultSummary, "falha de comunicação")

	sessao, err := env.h.Session(context.Background(), result.SessionID)
	require.NoError(t, err)
	resultados := toolResultParts(sessao)
	require.Len(t, resultados, 1)
	assert.True(t, resultados[0].IsError)
}
