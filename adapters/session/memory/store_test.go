package memory_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmarquespaixao/ia-harness/adapters/session/memory"
	"rmarquespaixao/ia-harness/harness"
)

// instante fixo evita dependência do relógio real nos testes.
var instante = time.Date(2026, time.September, 15, 10, 30, 0, 0, time.UTC)

// sessaoCompleta devolve uma sessão nova, com todas as variantes preenchidas.
func sessaoCompleta(id string) *harness.Session {
	return &harness.Session{
		ID:      id,
		UserID:  "user-1",
		AgentID: "agent-1",
		Model:   "default",
		State:   harness.SessionAwaitingConfirmation,
		Messages: []harness.Message{
			{
				ID:   "msg-1",
				Role: harness.RoleSystem,
				Parts: []harness.Part{
					{Kind: harness.PartText, Text: "prompt de sistema"},
				},
				CreatedAt: instante,
			},
			{
				ID:   "msg-2",
				Role: harness.RoleAssistant,
				Parts: []harness.Part{
					{Kind: harness.PartText, Text: "vou chamar a tool"},
					{
						Kind: harness.PartToolCall,
						Call: &harness.ToolCall{
							ID:        "call-1",
							Name:      "criar_transacao",
							Namespace: "financeiro",
							Args:      json.RawMessage(`{"amount_cents":1500,"description":"café"}`),
						},
					},
				},
				CreatedAt: instante.Add(time.Minute),
			},
			{
				ID:   "msg-3",
				Role: harness.RoleTool,
				Parts: []harness.Part{
					{
						Kind: harness.PartToolResult,
						Result: &harness.ToolResult{
							CallID: "call-1",
							Content: []harness.ResultContent{
								{Kind: harness.ResultText, Text: "ok"},
								{Kind: harness.ResultJSON, JSON: json.RawMessage(`{"id":"tx-1"}`)},
							},
							IsError:   false,
							Denied:    false,
							Truncated: false,
						},
					},
				},
				CreatedAt: instante.Add(2 * time.Minute),
			},
		},
		Pending: &harness.PendingConfirmation{
			CallID:       "call-1",
			ToolName:     "criar_transacao",
			ArgsRedacted: `{"amount_cents":"[REDACTED]"}`,
			Reason:       "tool destrutiva",
			RequestedAt:  instante,
		},
		Usage: harness.UsageTotals{
			InputTokens:  123,
			OutputTokens: 45,
			Estimated:    true,
			CostMicros:   678,
			Currency:     "BRL",
		},
		CreatedAt: instante,
		UpdatedAt: instante.Add(3 * time.Minute),
	}
}

func TestStore_SaveELoad_RoundtripFielDeTodosOsCampos(t *testing.T) {
	// Arrange
	ctx := context.Background()
	store := memory.New()
	original := sessaoCompleta("sessao-1")

	// Act
	require.NoError(t, store.Save(ctx, original))
	obtida, err := store.Load(ctx, "sessao-1")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, obtida)
	assert.Equal(t, original, obtida)
}

func TestStore_Load_MutacaoNoRetornoNaoAlteraArmazenado(t *testing.T) {
	// Arrange
	ctx := context.Background()
	store := memory.New()
	require.NoError(t, store.Save(ctx, sessaoCompleta("sessao-1")))
	obtida, err := store.Load(ctx, "sessao-1")
	require.NoError(t, err)

	// Act — muta todas as estruturas aninhadas da cópia devolvida
	obtida.UserID = "intruso"
	obtida.Usage.InputTokens = 999
	obtida.Pending.Reason = "alterado"
	obtida.Messages[0].Parts[0].Text = "alterado"
	obtida.Messages[1].Parts[1].Call.Name = "alterado"
	obtida.Messages[1].Parts[1].Call.Args[0] = '['
	obtida.Messages[2].Parts[0].Result.Content[1].JSON[0] = '['
	obtida.Messages = append(obtida.Messages, harness.Message{ID: "msg-4", Role: harness.RoleUser})
	nova, err := store.Load(ctx, "sessao-1")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, sessaoCompleta("sessao-1"), nova)
}

func TestStore_Save_MutacaoNaOrigemNaoAlteraArmazenado(t *testing.T) {
	// Arrange
	ctx := context.Background()
	store := memory.New()
	original := sessaoCompleta("sessao-1")
	require.NoError(t, store.Save(ctx, original))

	// Act — muta a sessão entregue após o Save
	original.UserID = "intruso"
	original.Pending.Reason = "alterado"
	original.Messages[0].Parts[0].Text = "alterado"
	original.Messages[1].Parts[1].Call.Args[0] = '['
	original.Messages[2].Parts[0].Result.Content[1].JSON[0] = '['
	original.Messages = original.Messages[:1]
	obtida, err := store.Load(ctx, "sessao-1")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, sessaoCompleta("sessao-1"), obtida)
}

func TestStore_Load_SessoesIsoladasPorID(t *testing.T) {
	// Arrange
	ctx := context.Background()
	store := memory.New()
	primeira := sessaoCompleta("sessao-1")
	segunda := sessaoCompleta("sessao-2")
	primeira.Messages[0].Parts[0].Text = "primeira"
	segunda.Messages[0].Parts[0].Text = "segunda"
	segunda.UserID = "user-2"
	require.NoError(t, store.Save(ctx, primeira))
	require.NoError(t, store.Save(ctx, segunda))

	// Act
	obtidaPrimeira, errPrimeira := store.Load(ctx, "sessao-1")
	obtidaSegunda, errSegunda := store.Load(ctx, "sessao-2")

	// Assert
	require.NoError(t, errPrimeira)
	require.NoError(t, errSegunda)
	assert.Equal(t, "user-1", obtidaPrimeira.UserID)
	assert.Equal(t, "primeira", obtidaPrimeira.Messages[0].Parts[0].Text)
	assert.Equal(t, "user-2", obtidaSegunda.UserID)
	assert.Equal(t, "segunda", obtidaSegunda.Messages[0].Parts[0].Text)
}

func TestStore_Load_SessaoAusenteDevolveErrNotFound(t *testing.T) {
	// Arrange
	ctx := context.Background()
	store := memory.New()

	// Act
	obtida, err := store.Load(ctx, "sessao-inexistente")

	// Assert
	require.Error(t, err)
	assert.Nil(t, obtida)
	assert.ErrorIs(t, err, memory.ErrNotFound)
	assert.Contains(t, err.Error(), `"sessao-inexistente"`)
}

func TestStore_Load_ContextoCancelado(t *testing.T) {
	// Arrange
	store := memory.New()
	ctx, cancelar := context.WithCancel(context.Background())
	cancelar()

	// Act
	obtida, err := store.Load(ctx, "sessao-1")

	// Assert
	assert.Nil(t, obtida)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestStore_Save_ContextoCancelado(t *testing.T) {
	// Arrange
	store := memory.New()
	ctx, cancelar := context.WithCancel(context.Background())
	cancelar()

	// Act
	err := store.Save(ctx, sessaoCompleta("sessao-1"))

	// Assert
	assert.ErrorIs(t, err, context.Canceled)
	_, errCarga := store.Load(context.Background(), "sessao-1")
	assert.ErrorIs(t, errCarga, memory.ErrNotFound)
}

func TestStore_Save_SessaoNulaDevolveErrNilSession(t *testing.T) {
	// Arrange
	store := memory.New()

	// Act
	err := store.Save(context.Background(), nil)

	// Assert
	assert.ErrorIs(t, err, memory.ErrNilSession)
}
