package harness_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auditlog "rmarquespaixao/ia-harness/adapters/audit/log"
	"rmarquespaixao/ia-harness/adapters/session/memory"
	"rmarquespaixao/ia-harness/contracts"
	gen "rmarquespaixao/ia-harness/contracts/gen"
	"rmarquespaixao/ia-harness/harness"
	"rmarquespaixao/ia-harness/internal/platform/schema"
	"rmarquespaixao/ia-harness/internal/testutil"
)

// requireAuditSchema valida o marshal do evento real contra o schema normativo
// contracts/audit/audit_event.json (D-11/US5).
func requireAuditSchema(t *testing.T, event harness.AuditEvent) {
	t.Helper()

	raw, err := contracts.Schema("audit/audit_event.json")
	require.NoError(t, err)
	validator, err := schema.Compile(raw)
	require.NoError(t, err)

	encoded, err := json.Marshal(event)
	require.NoError(t, err)
	require.NoError(t, validator.Validate(encoded), "evento não conforma com o schema: %s", encoded)
}

// teeSink encaminha cada evento para dois sinks (contrato em memória + log).
type teeSink struct {
	mem harness.AuditSink
	log harness.AuditSink
}

func (s *teeSink) Emit(ctx context.Context, ev harness.AuditEvent) error {
	if err := s.mem.Emit(ctx, ev); err != nil {
		return err
	}
	return s.log.Emit(ctx, ev)
}

var _ harness.AuditSink = (*teeSink)(nil)

// Cobre o Gherkin do CU-HAR-5 (use-cases.md): evento por chamada conforme o
// schema de auditoria, redação de iscas no evento e no log, custo estimado
// quando o provider não reporta uso.
func TestCU_HAR5_Cenario1_EventoPorChamadaConformeSchema(t *testing.T) {
	// Dado um turno com chamada de modelo e execução de tool
	sink := &testutil.FakeAuditSink{}
	source := &testutil.MemToolSource{
		Tools: []harness.Tool{{Name: "buscar", Namespace: "demo", Description: "busca"}},
		Results: map[string]harness.ToolResult{
			"buscar": {Content: []harness.ResultContent{{Kind: harness.ResultText, Text: "resultado"}}},
		},
	}
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{
		{ToolCalls: []harness.ToolCall{{ID: "call-1", Name: "buscar", Namespace: "demo", Args: json.RawMessage(`{"q":"x"}`)}}},
		{Text: "fim", Usage: harness.Usage{InputTokens: 40, OutputTokens: 8}},
	}}
	h := newAuditHarness(t, provider, []harness.ToolSource{source}, sink, memory.New(), nil)

	// Quando o turno termina
	result, err := h.Run(
		harness.WithTraceID(context.Background(), "trace-cu-har5"),
		harness.RunRequest{UserID: "user-1", AgentID: "agent-1", Input: []harness.Part{{Kind: harness.PartText, Text: "busque x"}}},
		&recordingHandler{},
	)

	// Então existe um AuditEvent para cada chamada
	require.NoError(t, err)
	assert.Equal(t, harness.StopCompleted, result.StopReason)
	require.Len(t, sink.Events, 3)
	esperados := []gen.AuditEventKind{
		gen.AuditEventKindModelCall,
		gen.AuditEventKindToolCall,
		gen.AuditEventKindModelCall,
	}

	// E todos validam contra o schema de auditoria
	for i, event := range sink.Events {
		assert.Equal(t, string(esperados[i]), string(event.Kind))
		assert.Equal(t, "trace-cu-har5", event.TraceId, "trace_id propagado a todos os eventos")
		requireAuditSchema(t, event)
	}
}

func TestCU_HAR5_Cenario2_RedacaoDeIscaNoEventoENoLog(t *testing.T) {
	// Dado argumentos contendo "api_key" e "authorization"
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mem := &testutil.FakeAuditSink{}
	sink := &teeSink{mem: mem, log: auditlog.New(logger)}

	const (
		iscaArgsKey      = "isca-api-key-123"
		iscaArgsAuth     = "Bearer isca-auth-456"
		iscaArgsDocument = "aXNjYS1kb2N1bWVudG8="
		iscaResult       = "isca-result-789"
	)
	source := &testutil.MemToolSource{
		Tools: []harness.Tool{{Name: "enviar", Namespace: "demo"}},
		Results: map[string]harness.ToolResult{
			"enviar": {Content: []harness.ResultContent{{
				Kind: harness.ResultJSON,
				JSON: json.RawMessage(`{"api_key":"` + iscaResult + `","total":1}`),
			}}},
		},
	}
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{
		{ToolCalls: []harness.ToolCall{{
			ID:        "call-1",
			Name:      "enviar",
			Namespace: "demo",
			Args: json.RawMessage(`{"api_key":"` + iscaArgsKey + `","authorization":"` + iscaArgsAuth +
				`","document_base64":"` + iscaArgsDocument + `","q":"x"}`),
		}}},
		{Text: "fim"},
	}}
	h := newAuditHarness(t, provider, []harness.ToolSource{source}, sink, memory.New(), func(c *harness.Config) {
		c.Redaction = harness.RedactionConfig{SensitiveKeys: []string{"api_key", "authorization"}}
	})

	// Quando o evento é emitido
	_, err := h.Run(context.Background(), harness.RunRequest{UserID: "user-1"}, &recordingHandler{})

	// Então os valores aparecem como "[REDACTED]"
	require.NoError(t, err)
	require.Len(t, mem.Events, 3)
	event := mem.Events[1]
	require.NotNil(t, event.ArgsRedacted)
	assert.Contains(t, *event.ArgsRedacted, "[REDACTED]")
	require.NotNil(t, event.ResultRedacted)
	assert.Contains(t, *event.ResultRedacted, "[REDACTED]")
	assert.Contains(t, *event.ArgsRedacted, `"q"`, "campo não sensível deve ser preservado")

	// E nenhuma isca vaza no evento nem no log (payload em Debug).
	evento := eventJSON(t, event)
	log := buf.String()
	assert.Contains(t, log, "audit_payload", "payload redigido deve ser registrado em Debug")
	for _, isca := range []string{iscaArgsKey, iscaArgsAuth, iscaArgsDocument, iscaResult} {
		assert.NotContains(t, evento, isca, "isca não pode vazar no evento de auditoria")
		assert.NotContains(t, log, isca, "isca não pode vazar no log")
	}
}

func TestCU_HAR5_Cenario3_CustoEstimadoSemUsoReportado(t *testing.T) {
	// Dado um provider que não reporta uso
	sink := &testutil.FakeAuditSink{}
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{
		Text:       "sem usage nesta resposta",
		StopReason: harness.StopCompleted,
	}}}
	h := newAuditHarness(t, provider, nil, sink, memory.New(), func(c *harness.Config) {
		c.Pricing = harness.Pricing{
			Currency:                    "USD",
			InputPriceMicrosPerMillion:  3_000_000,
			OutputPriceMicrosPerMillion: 15_000_000,
			BytesPerToken:               4,
		}
	})

	// Quando o evento de modelo é emitido
	_, err := h.Run(context.Background(), harness.RunRequest{UserID: "user-1"}, &recordingHandler{})

	// Então "estimated" é true e o custo usa a premissa
	require.NoError(t, err)
	require.Len(t, sink.Events, 1)
	event := sink.Events[0]
	assert.True(t, event.Estimated)
	require.NotNil(t, event.InputTokens)
	require.NotNil(t, event.OutputTokens)
	assert.Positive(t, *event.InputTokens)
	assert.Positive(t, *event.OutputTokens)
	require.NotNil(t, event.CostMicros)
	esperado := (int64(*event.InputTokens)*3_000_000 + int64(*event.OutputTokens)*15_000_000) / 1_000_000
	assert.Equal(t, int(esperado), *event.CostMicros)
	require.NotNil(t, event.Currency)
	assert.Equal(t, "USD", *event.Currency)
	requireAuditSchema(t, event)
}
