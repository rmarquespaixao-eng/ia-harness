package log_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auditlog "rmarquespaixao/ia-harness/adapters/audit/log"
	gen "rmarquespaixao/ia-harness/contracts/gen"
	"rmarquespaixao/ia-harness/harness"
)

// instante fixo evita dependência do relógio real nos testes.
var instante = time.Date(2026, time.September, 15, 10, 30, 0, 0, time.UTC)

// registro espelha os campos do resumo Info para decodificar a linha JSON.
type registro struct {
	Level        string `json:"level"`
	Msg          string `json:"msg"`
	Kind         string `json:"kind"`
	Status       string `json:"status"`
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	Tool         string `json:"tool"`
	LatencyMs    int64  `json:"latency_ms"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	Estimated    bool   `json:"estimated"`
	CostMicros   int64  `json:"cost_micros"`
	Currency     string `json:"currency"`
	TraceID      string `json:"trace_id"`
}

// payload espelha o registro Debug com os campos redigidos.
type payload struct {
	Level          string `json:"level"`
	Msg            string `json:"msg"`
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	ArgsRedacted   string `json:"args_redacted"`
	ResultRedacted string `json:"result_redacted"`
}

// eventoPreenchido devolve um AuditEvent com todos os campos do contrato.
func eventoPreenchido() harness.AuditEvent {
	provider := "openai"
	model := "gpt-4o-mini"
	tool := "financeiro.criar_transacao"
	inputTokens := 321
	outputTokens := 45
	costMicros := 678
	currency := "BRL"
	args := `{"amount_cents":"[REDACTED]"}`
	result := `{"id":"tx-1"}`

	return harness.AuditEvent{
		Id:             "evt-1",
		Kind:           gen.AuditEventKindToolCall,
		TraceId:        "trace-do-evento",
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
		LatencyMs:      42,
		ArgsRedacted:   &args,
		ResultRedacted: &result,
		OccurredAt:     instante,
	}
}

// decodifica a primeira linha do buffer como um registro de resumo.
func decodificarResumo(t *testing.T, saida string) registro {
	t.Helper()
	linhas := strings.Split(strings.TrimSpace(saida), "\n")
	require.NotEmpty(t, linhas)

	var r registro
	require.NoError(t, json.Unmarshal([]byte(linhas[0]), &r))
	return r
}

// camposDaLinha decodifica a primeira linha do buffer como mapa de campos,
// permitindo checar presença/ausência de chaves no JSON.
func camposDaLinha(t *testing.T, saida string) map[string]any {
	t.Helper()
	linhas := strings.Split(strings.TrimSpace(saida), "\n")
	require.NotEmpty(t, linhas)

	var campos map[string]any
	require.NoError(t, json.Unmarshal([]byte(linhas[0]), &campos))
	return campos
}

func TestSink_Emit_RegistraResumoCompletoEmInfo(t *testing.T) {
	// Arrange
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	sink := auditlog.New(logger)
	ctx := harness.WithTraceID(context.Background(), "trace-9f3a")
	ev := eventoPreenchido()

	// Act
	err := sink.Emit(ctx, ev)
	require.NoError(t, err)

	// Assert
	r := decodificarResumo(t, buf.String())
	assert.Equal(t, "INFO", r.Level)
	assert.Equal(t, string(gen.AuditEventKindToolCall), r.Kind)
	assert.Equal(t, string(gen.AuditEventStatusOk), r.Status)
	assert.Equal(t, "openai", r.Provider)
	assert.Equal(t, "gpt-4o-mini", r.Model)
	assert.Equal(t, "financeiro.criar_transacao", r.Tool)
	assert.Equal(t, int64(42), r.LatencyMs)
	assert.Equal(t, int64(321), r.InputTokens)
	assert.Equal(t, int64(45), r.OutputTokens)
	assert.True(t, r.Estimated)
	assert.Equal(t, int64(678), r.CostMicros)
	assert.Equal(t, "BRL", r.Currency)
	assert.Equal(t, "trace-9f3a", r.TraceID)

	campos := camposDaLinha(t, buf.String())
	assert.NotContains(t, campos, "args_redacted")
	assert.NotContains(t, campos, "result_redacted")
	assert.NotContains(t, buf.String(), "[REDACTED]")
}

func TestSink_Emit_SemTraceNoContextoMantemCampoVazio(t *testing.T) {
	// Arrange
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	sink := auditlog.New(logger)
	ev := eventoPreenchido()

	// Act
	err := sink.Emit(context.Background(), ev)

	// Assert
	require.NoError(t, err)
	r := decodificarResumo(t, buf.String())
	assert.Equal(t, "", r.TraceID)
}

func TestSink_Emit_CamposOpcionaisAusentesNaoPanicam(t *testing.T) {
	// Arrange
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	sink := auditlog.New(logger)
	ev := harness.AuditEvent{
		Id:         "evt-2",
		Kind:       gen.AuditEventKindModelCall,
		Status:     gen.AuditEventStatusError,
		LatencyMs:  0,
		OccurredAt: instante,
	}

	// Act
	err := sink.Emit(context.Background(), ev)

	// Assert
	require.NoError(t, err)
	r := decodificarResumo(t, buf.String())
	assert.Equal(t, string(gen.AuditEventKindModelCall), r.Kind)
	assert.Equal(t, string(gen.AuditEventStatusError), r.Status)
	assert.Equal(t, "", r.Provider)
	assert.Equal(t, int64(0), r.InputTokens)
	assert.Equal(t, int64(0), r.CostMicros)
	assert.False(t, r.Estimated)
}

func TestSink_Emit_PayloadRedigidoSoApareceEmDebug(t *testing.T) {
	// Arrange
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sink := auditlog.New(logger)
	ev := eventoPreenchido()

	// Act
	err := sink.Emit(context.Background(), ev)

	// Assert
	require.NoError(t, err)
	linhas := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, linhas, 2)

	assert.Contains(t, linhas[0], `"level":"INFO"`)
	assert.NotContains(t, linhas[0], "args_redacted")
	assert.NotContains(t, linhas[0], "[REDACTED]")

	var p payload
	require.NoError(t, json.Unmarshal([]byte(linhas[1]), &p))
	assert.Equal(t, "DEBUG", p.Level)
	assert.Equal(t, "evt-1", p.ID)
	assert.Equal(t, string(gen.AuditEventKindToolCall), p.Kind)
	assert.Equal(t, `{"amount_cents":"[REDACTED]"}`, p.ArgsRedacted)
	assert.Equal(t, `{"id":"tx-1"}`, p.ResultRedacted)
}

func TestSink_Emit_NuncaDevolveErro(t *testing.T) {
	// Arrange
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	sink := auditlog.New(logger)
	ctxCancelado, cancelar := context.WithCancel(context.Background())
	cancelar()

	// Act
	err := sink.Emit(ctxCancelado, eventoPreenchido())

	// Assert
	assert.NoError(t, err)
}
