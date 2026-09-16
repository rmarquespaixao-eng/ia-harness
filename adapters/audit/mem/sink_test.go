package memory_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auditmemory "rmarquespaixao/ia-harness/adapters/audit/mem"
	gen "rmarquespaixao/ia-harness/contracts/gen"
	"rmarquespaixao/ia-harness/harness"
)

// instante fixo evita dependência do relógio real nos testes.
var instante = time.Date(2026, time.September, 15, 10, 30, 0, 0, time.UTC)

// evento devolve um AuditEvent preenchido e identificável por id.
func evento(id string) harness.AuditEvent {
	provider := "openai"
	model := "gpt-4o-mini"
	tool := "financeiro.criar_transacao"
	inputTokens := 100
	outputTokens := 20
	costMicros := 550
	currency := "BRL"
	args := `{"amount_cents":"[REDACTED]"}`
	result := `{"id":"tx-1"}`

	return harness.AuditEvent{
		Id:             id,
		Kind:           gen.AuditEventKindToolCall,
		TraceId:        "trace-" + id,
		UserId:         "user-1",
		SessionId:      "sessao-1",
		AgentId:        "agent-1",
		Provider:       &provider,
		Model:          &model,
		Tool:           &tool,
		Status:         gen.AuditEventStatusOk,
		InputTokens:    &inputTokens,
		OutputTokens:   &outputTokens,
		Estimated:      true,
		CostMicros:     &costMicros,
		Currency:       &currency,
		LatencyMs:      7,
		ArgsRedacted:   &args,
		ResultRedacted: &result,
		OccurredAt:     instante,
	}
}

func TestSink_Emit_PreservaOrdemDeEmissao(t *testing.T) {
	// Arrange
	ctx := context.Background()
	sink := auditmemory.New()

	// Act
	require.NoError(t, sink.Emit(ctx, evento("evt-1")))
	require.NoError(t, sink.Emit(ctx, evento("evt-2")))
	require.NoError(t, sink.Emit(ctx, evento("evt-3")))
	eventos := sink.Snapshot()

	// Assert
	require.Len(t, eventos, 3)
	assert.Equal(t, "evt-1", eventos[0].Id)
	assert.Equal(t, "evt-2", eventos[1].Id)
	assert.Equal(t, "evt-3", eventos[2].Id)
}

func TestSink_Snapshot_MutacaoNoRetornoNaoAlteraArmazenado(t *testing.T) {
	// Arrange
	ctx := context.Background()
	sink := auditmemory.New()
	original := evento("evt-1")
	require.NoError(t, sink.Emit(ctx, original))
	esperado := evento("evt-1")

	// Act — muta o evento devolvido, inclusive os campos apontados
	obtidos := sink.Snapshot()
	require.Len(t, obtidos, 1)
	obtidos[0].Id = "alterado"
	obtidos[0].Estimated = false
	*obtidos[0].Tool = "alterado"
	*obtidos[0].InputTokens = 999
	*obtidos[0].CostMicros = 999
	*obtidos[0].Currency = "USD"
	*obtidos[0].ArgsRedacted = "alterado"
	_ = append(obtidos, evento("evt-2"))
	novos := sink.Snapshot()

	// Assert
	require.Len(t, novos, 1)
	assert.Equal(t, esperado, novos[0])
}

func TestSink_Emit_MutacaoNaOrigemNaoAlteraArmazenado(t *testing.T) {
	// Arrange
	ctx := context.Background()
	sink := auditmemory.New()
	original := evento("evt-1")
	esperado := evento("evt-1")
	require.NoError(t, sink.Emit(ctx, original))

	// Act — muta o evento entregue após o Emit
	original.Id = "alterado"
	original.Estimated = false
	*original.Tool = "alterado"
	*original.InputTokens = 999
	*original.ArgsRedacted = "alterado"
	obtidos := sink.Snapshot()

	// Assert
	require.Len(t, obtidos, 1)
	assert.Equal(t, esperado, obtidos[0])
}

func TestSink_Emit_ConcorrenciaNaoPerdeEventos(t *testing.T) {
	// Arrange
	ctx := context.Background()
	sink := auditmemory.New()
	const goroutines = 8
	const porGoroutine = 50
	var wg sync.WaitGroup

	// Act
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < porGoroutine; i++ {
				assert.NoError(t, sink.Emit(ctx, evento("evt-concorrente")))
			}
		}(g)
	}
	wg.Wait()
	eventos := sink.Snapshot()

	// Assert
	assert.Len(t, eventos, goroutines*porGoroutine)
}

func TestSink_Snapshot_VazioDevolveListaVazia(t *testing.T) {
	// Arrange
	sink := auditmemory.New()

	// Act
	eventos := sink.Snapshot()

	// Assert
	assert.Empty(t, eventos)
}
