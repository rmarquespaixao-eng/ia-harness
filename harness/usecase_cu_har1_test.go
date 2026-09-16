package harness_test

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/harness"
	"github.com/rmarquespaixao-eng/ia-harness/internal/testutil"
)

// TestCU_HAR1_CriteriosDeAceite executa os quatro cenários Gherkin do caso de
// uso CU-HAR-1 (executar turno de agente com tools MCP) como subtests.
func TestCU_HAR1_CriteriosDeAceite(t *testing.T) {
	t.Run("Resposta fundamentada em tool", func(t *testing.T) {
		// Dado um provider simulado e um servidor MCP em memória com a tool "saldo".
		handler := &turnHandler{}
		env := newTurnEnv(t, []testutil.ProviderStep{
			{
				Text:      "Consultando seu saldo.",
				ToolCalls: []harness.ToolCall{toolCall("call-saldo", "consultar_saldo", `{"q":"saldo"}`)},
			},
			{Text: "Seu saldo é R$ 1.234,56."},
		})

		// Quando o usuário pergunta "qual o meu saldo?".
		result, err := env.h.Run(context.Background(), pergunta("qual o meu saldo?"), handler)

		// Então o harness valida os argumentos e chama a tool.
		require.NoError(t, err)
		require.Len(t, env.tools.Calls, 1)
		assert.Equal(t, "consultar_saldo", env.tools.Calls[0].Name)
		assert.JSONEq(t, `{"q":"saldo"}`, string(env.tools.Calls[0].Args))

		// E o texto final é entregue em fragmentos (TextDelta).
		assert.GreaterOrEqual(t, len(handler.deltas), 2, "streaming incremental")
		var textoFinal string
		for _, delta := range handler.deltas {
			textoFinal += delta.Text
		}
		assert.Contains(t, textoFinal, "Seu saldo é R$ 1.234,56.")

		// E o TurnResult tem StopReason "completed".
		assert.Equal(t, harness.StopCompleted, result.StopReason)
	})

	t.Run("Argumento inválido não executa", func(t *testing.T) {
		// Dado que o modelo pede a tool com um argumento fora do schema.
		handler := &turnHandler{}
		env := newTurnEnv(t, []testutil.ProviderStep{
			{ToolCalls: []harness.ToolCall{toolCall("call-invalida", "consultar_saldo", `{"sem_q":"x"}`)}},
			{Text: "Não consegui consultar."},
		})

		// Quando o harness valida a chamada.
		result, err := env.h.Run(context.Background(), pergunta("qual o meu saldo?"), handler)

		// Então um resultado de erro é devolvido ao modelo.
		require.NoError(t, err)
		require.Len(t, handler.toolResults, 1)
		assert.True(t, handler.toolResults[0].IsError)
		assert.Equal(t, harness.StatusError, handler.toolResults[0].Status)

		// E o servidor MCP não recebe a chamada.
		assert.Empty(t, env.tools.Calls)
		assert.Equal(t, harness.StopCompleted, result.StopReason, "o turno continua após o erro")
	})

	t.Run("Tool longa emite progresso", func(t *testing.T) {
		// Dado uma tool que publica notificações de progresso.
		handler := &turnHandler{}
		env := newTurnEnv(t, []testutil.ProviderStep{
			{ToolCalls: []harness.ToolCall{toolCall("call-lenta", "executar_lenta", `{"q":"tudo"}`)}},
			{Text: "Concluído."},
		})
		env.tools.Progress = map[string][]harness.ProgressUpdate{
			"executar_lenta": {{Message: "processando", Progress: 1, Total: 3}},
		}

		// Quando o harness a executa.
		_, err := env.h.Run(context.Background(), pergunta("rode a tarefa"), handler)

		// Então os eventos de progresso chegam antes do resultado final.
		require.NoError(t, err)
		require.Len(t, handler.progress, 1)
		require.Len(t, handler.toolResults, 1)
		indiceProgresso := slices.Index(handler.order, "progress:executar_lenta")
		indiceResultado := slices.Index(handler.order, "tool_result:executar_lenta")
		require.NotEqual(t, -1, indiceProgresso)
		require.NotEqual(t, -1, indiceResultado)
		assert.Less(t, indiceProgresso, indiceResultado)
	})

	t.Run("Cancelamento", func(t *testing.T) {
		// Dado um turno em andamento.
		ctx, cancelar := context.WithCancel(context.Background())
		defer cancelar()
		eventosNoCancelamento := -1
		var handler *turnHandler
		handler = &turnHandler{onToolResult: func(harness.ToolResultEvent) {
			cancelar()
			eventosNoCancelamento = len(handler.order)
		}}
		env := newTurnEnv(t, []testutil.ProviderStep{
			{ToolCalls: []harness.ToolCall{toolCall("call-1", "consultar_saldo", `{"q":"saldo"}`)}},
			{Text: "não deve ser chamado"},
		})

		// Quando o host cancela o contexto.
		result, err := env.h.Run(ctx, pergunta("qual o saldo?"), handler)

		// Então nenhum evento novo é emitido.
		require.NotEqual(t, -1, eventosNoCancelamento)
		assert.Len(t, handler.order, eventosNoCancelamento, "nenhum evento novo após o cancelamento")

		// E o TurnResult tem StopReason "cancelled", com a sessão persistida.
		assert.Equal(t, harness.StopCancelled, result.StopReason)
		assert.NoError(t, err, "turno cancelado encerra sem erro (CU-HAR-1 6a)")
		sessao, errCarga := env.h.Session(context.Background(), result.SessionID)
		require.NoError(t, errCarga, "sessão persistida no ponto consistente")
		assert.Equal(t, harness.SessionActive, sessao.State)
	})
}
