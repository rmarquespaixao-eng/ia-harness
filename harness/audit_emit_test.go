package harness_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmarquespaixao/ia-harness/adapters/session/memory"
	"rmarquespaixao/ia-harness/harness"
	"rmarquespaixao/ia-harness/internal/testutil"
)

// discardLogger devolve um logger que descarta a saída (os testes de auditoria
// observam eventos e buffers próprios, não o log padrão).
func discardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

// recordingHandler captura os eventos do turno para asserção.
type recordingHandler struct {
	harness.NopHandler
	usage   []harness.UsageEvent
	errs    []harness.ErrorEvent
	results []harness.ToolResultEvent
}

func (r *recordingHandler) Usage(_ context.Context, ev harness.UsageEvent) {
	r.usage = append(r.usage, ev)
}

func (r *recordingHandler) Error(_ context.Context, ev harness.ErrorEvent) {
	r.errs = append(r.errs, ev)
}

func (r *recordingHandler) ToolResult(_ context.Context, ev harness.ToolResultEvent) {
	r.results = append(r.results, ev)
}

// newAuditHarness monta o harness com o sink/preço de auditoria e aplica os
// ajustes de config pedidos pelo teste.
func newAuditHarness(t *testing.T, provider harness.Provider, tools []harness.ToolSource, sink harness.AuditSink, store harness.SessionStore, mutate func(*harness.Config)) *harness.Harness {
	t.Helper()

	cfg := harness.Config{
		Providers: map[string]harness.Provider{"stub": provider},
		Models: map[string]harness.ModelProfile{
			"default": {Provider: "stub", Model: "stub-1", Capabilities: harness.Capabilities{ToolCalling: true, Streaming: true}},
		},
		DefaultModel: "default",
		Policy:       harness.PolicyConfig{Default: harness.PolicyAllow},
		Tools:        tools,
		Credentials:  &testutil.FakeCredentialProvider{Values: map[string]string{}},
		Sessions:     store,
		Logger:       discardLogger(),
		Audit:        sink,
		Pricing: harness.Pricing{
			Currency:                    "BRL",
			InputPriceMicrosPerMillion:  1_000_000,
			OutputPriceMicrosPerMillion: 2_000_000,
			BytesPerToken:               4,
		},
	}
	if mutate != nil {
		mutate(&cfg)
	}

	h, err := harness.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, h.Close()) })
	return h
}

// eventJSON serializa o evento para asserções de ausência de segredo.
func eventJSON(t *testing.T, event harness.AuditEvent) string {
	t.Helper()
	raw, err := json.Marshal(event)
	require.NoError(t, err)
	return string(raw)
}

func TestAudit_ModelCallComUsoReportado(t *testing.T) {
	// Arrange — o provider reporta tokens; a premissa calcula o custo.
	sink := &testutil.FakeAuditSink{}
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{
		Text:       "pronto",
		Usage:      harness.Usage{InputTokens: 1000, OutputTokens: 500},
		StopReason: harness.StopCompleted,
	}}}
	h := newAuditHarness(t, provider, nil, sink, memory.New(), nil)
	handler := &recordingHandler{}

	// Act — trace_id injetado pelo host via WithTraceID.
	result, err := h.Run(
		harness.WithTraceID(context.Background(), "trace-abc123"),
		harness.RunRequest{UserID: "user-1", AgentID: "agent-1", Input: []harness.Part{{Kind: harness.PartText, Text: "oi"}}},
		handler,
	)

	// Assert — um evento model_call reportado (estimated=false) com custo.
	require.NoError(t, err)
	assert.Equal(t, harness.StopCompleted, result.StopReason)
	require.Len(t, sink.Events, 1)

	event := sink.Events[0]
	assert.Equal(t, "model_call", string(event.Kind))
	assert.Equal(t, "ok", string(event.Status))
	assert.Equal(t, "trace-abc123", event.TraceId)
	assert.Equal(t, "user-1", event.UserId)
	assert.Equal(t, "agent-1", event.AgentId)
	assert.NotEmpty(t, event.SessionId)
	require.NotNil(t, event.Provider)
	assert.Equal(t, "stub", *event.Provider)
	require.NotNil(t, event.Model)
	assert.Equal(t, "stub-1", *event.Model)
	assert.False(t, event.Estimated)
	require.NotNil(t, event.InputTokens)
	assert.Equal(t, 1000, *event.InputTokens)
	require.NotNil(t, event.OutputTokens)
	assert.Equal(t, 500, *event.OutputTokens)
	require.NotNil(t, event.CostMicros)
	assert.Equal(t, 2000, *event.CostMicros, "custo = (1000×1 + 500×2) micros")
	require.NotNil(t, event.Currency)
	assert.Equal(t, "BRL", *event.Currency)
	assert.GreaterOrEqual(t, event.LatencyMs, 0)
	assert.WithinDuration(t, time.Now(), event.OccurredAt, time.Minute)

	// O UsageEvent do host espelha o mesmo Usage acumulado na sessão.
	require.Len(t, handler.usage, 1)
	assert.Equal(t, int64(1000), handler.usage[0].InputTokens)
	assert.Equal(t, int64(500), handler.usage[0].OutputTokens)
	assert.False(t, handler.usage[0].Estimated)
	assert.Equal(t, int64(2000), handler.usage[0].CostMicros)
	assert.Equal(t, int64(1000), result.Usage.InputTokens)
	assert.Equal(t, int64(500), result.Usage.OutputTokens)
	assert.Equal(t, int64(2000), result.Usage.CostMicros)
}

func TestAudit_ModelCallSemUsoEstimaPelaPremissa(t *testing.T) {
	// Arrange — o provider não reporta uso e não há trace_id no contexto.
	sink := &testutil.FakeAuditSink{}
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{
		Text:       "resposta estimada",
		StopReason: harness.StopCompleted,
	}}}
	h := newAuditHarness(t, provider, nil, sink, memory.New(), nil)

	// Act
	result, err := h.Run(context.Background(), harness.RunRequest{UserID: "user-1"}, &recordingHandler{})

	// Assert — estimated=true com tokens/custo da premissa e trace_id vazio.
	require.NoError(t, err)
	require.Len(t, sink.Events, 1)

	event := sink.Events[0]
	assert.True(t, event.Estimated)
	assert.Empty(t, event.TraceId)
	require.NotNil(t, event.InputTokens)
	require.NotNil(t, event.OutputTokens)
	assert.Positive(t, *event.InputTokens)
	assert.Positive(t, *event.OutputTokens)
	esperado := (int64(*event.InputTokens)*1_000_000 + int64(*event.OutputTokens)*2_000_000) / 1_000_000
	require.NotNil(t, event.CostMicros)
	assert.Equal(t, int(esperado), *event.CostMicros)
	assert.True(t, result.Usage.Estimated)
	assert.Equal(t, int64(*event.InputTokens), result.Usage.InputTokens, "acúmulo usa o usage estimado")
}

func TestAudit_ToolCallOKComPayloadRedigido(t *testing.T) {
	// Arrange — tool com args e resultado JSON que passam pela trilha.
	sink := &testutil.FakeAuditSink{}
	source := &testutil.MemToolSource{
		Tools: []harness.Tool{{Name: "buscar", Namespace: "demo", Description: "busca"}},
		Results: map[string]harness.ToolResult{
			"buscar": {Content: []harness.ResultContent{{Kind: harness.ResultText, Text: "resultado"}}},
		},
	}
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{
		{ToolCalls: []harness.ToolCall{{ID: "call-1", Name: "buscar", Namespace: "demo", Args: json.RawMessage(`{"q":"x"}`)}}},
		{Text: "fim", Usage: harness.Usage{InputTokens: 5, OutputTokens: 3}},
	}}
	h := newAuditHarness(t, provider, []harness.ToolSource{source}, sink, memory.New(), nil)

	// Act
	result, err := h.Run(context.Background(), harness.RunRequest{UserID: "user-1"}, &recordingHandler{})

	// Assert — model, tool ok e model, na ordem do turno.
	require.NoError(t, err)
	assert.Equal(t, harness.StopCompleted, result.StopReason)
	require.Len(t, sink.Events, 3)
	assert.Equal(t, "model_call", string(sink.Events[0].Kind))

	event := sink.Events[1]
	assert.Equal(t, "tool_call", string(event.Kind))
	assert.Equal(t, "ok", string(event.Status))
	require.NotNil(t, event.Tool)
	assert.Equal(t, "demo.buscar", *event.Tool)
	require.NotNil(t, event.ToolCallId)
	assert.Equal(t, "call-1", *event.ToolCallId)
	assert.False(t, event.Estimated)
	require.NotNil(t, event.ArgsRedacted)
	assert.JSONEq(t, `{"q":"x"}`, *event.ArgsRedacted)
	require.NotNil(t, event.ResultRedacted)
	assert.Equal(t, "resultado", *event.ResultRedacted)
	assert.Nil(t, event.CostMicros, "evento de tool não carrega custo")
	assert.Nil(t, event.Provider, "evento de tool não carrega provider")
}

func TestAudit_ToolCallErroRegistraStatusEErro(t *testing.T) {
	// Arrange — a fonte da tool falha; o loop transforma em resultado de erro.
	sink := &testutil.FakeAuditSink{}
	source := &testutil.MemToolSource{
		Tools:  []harness.Tool{{Name: "buscar", Namespace: "demo"}},
		Errors: map[string]error{"buscar": errors.New("transporte caiu")},
	}
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{
		{ToolCalls: []harness.ToolCall{{ID: "call-2", Name: "buscar", Namespace: "demo", Args: json.RawMessage(`{"q":"x"}`)}}},
		{Text: "fim"},
	}}
	h := newAuditHarness(t, provider, []harness.ToolSource{source}, sink, memory.New(), nil)

	// Act
	_, err := h.Run(context.Background(), harness.RunRequest{UserID: "user-1"}, &recordingHandler{})

	// Assert
	require.NoError(t, err)
	require.Len(t, sink.Events, 3)
	event := sink.Events[1]
	assert.Equal(t, "tool_call", string(event.Kind))
	assert.Equal(t, "error", string(event.Status))
	require.NotNil(t, event.ArgsRedacted)
	assert.JSONEq(t, `{"q":"x"}`, *event.ArgsRedacted)
	require.NotNil(t, event.ResultRedacted)
	assert.Contains(t, *event.ResultRedacted, "falha de comunicação")
}

func TestAudit_ToolCallDeniedSemPayload(t *testing.T) {
	// Arrange — sessão pausada aguardando confirmação com args-isca; o host nega.
	sink := &testutil.FakeAuditSink{}
	store := memory.New()
	source := &testutil.MemToolSource{Tools: []harness.Tool{{Name: "transferir", Namespace: "demo"}}}
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "ok"}}}
	h := newAuditHarness(t, provider, []harness.ToolSource{source}, sink, store, nil)

	agora := time.Now()
	session := &harness.Session{
		ID:      "sess-1",
		UserID:  "user-1",
		AgentID: "agent-1",
		Model:   "default",
		State:   harness.SessionAwaitingConfirmation,
		Messages: []harness.Message{{
			ID:   "msg-1",
			Role: harness.RoleAssistant,
			Parts: []harness.Part{{
				Kind: harness.PartToolCall,
				Call: &harness.ToolCall{
					ID:        "call-9",
					Name:      "transferir",
					Namespace: "demo",
					Args:      json.RawMessage(`{"api_key":"isca-secreta","amount_cents":100}`),
				},
			}},
			CreatedAt: agora,
		}},
		Pending:   &harness.PendingConfirmation{CallID: "call-9", ToolName: "demo.transferir", Reason: "destrutiva"},
		CreatedAt: agora,
		UpdatedAt: agora,
	}
	require.NoError(t, store.Save(context.Background(), session))

	// Act
	result, err := h.ResolveConfirmation(context.Background(), "sess-1", "call-9", harness.Decision{Approve: false, Reason: "não autorizo"}, &recordingHandler{})

	// Assert — denied sem payload e sem vazar a isca.
	require.NoError(t, err)
	assert.Equal(t, harness.StopCompleted, result.StopReason)
	require.Len(t, sink.Events, 2)
	event := sink.Events[0]
	assert.Equal(t, "tool_call", string(event.Kind))
	assert.Equal(t, "denied", string(event.Status))
	require.NotNil(t, event.ToolCallId)
	assert.Equal(t, "call-9", *event.ToolCallId)
	assert.Nil(t, event.ArgsRedacted, "denied não carrega args")
	assert.Nil(t, event.ResultRedacted, "denied não carrega resultado")
	assert.NotContains(t, eventJSON(t, event), "isca-secreta")
}

func TestAudit_FalhaDoSinkViraErrorEventSemDerrubarOTurno(t *testing.T) {
	// Arrange — sink sempre falha; turno tem modelo e execução de tool.
	sink := &testutil.FakeAuditSink{Err: errors.New("sink indisponível")}
	source := &testutil.MemToolSource{
		Tools: []harness.Tool{{Name: "buscar", Namespace: "demo"}},
		Results: map[string]harness.ToolResult{
			"buscar": {Content: []harness.ResultContent{{Kind: harness.ResultText, Text: "ok"}}},
		},
	}
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{
		{ToolCalls: []harness.ToolCall{{ID: "call-1", Name: "buscar", Namespace: "demo"}}},
		{Text: "segue", Usage: harness.Usage{InputTokens: 10, OutputTokens: 5}},
	}}
	h := newAuditHarness(t, provider, []harness.ToolSource{source}, sink, memory.New(), nil)
	handler := &recordingHandler{}

	// Act
	result, err := h.Run(context.Background(), harness.RunRequest{UserID: "user-1"}, handler)

	// Assert — o turno conclui, nenhum evento persistiu e cada falha é observável.
	require.NoError(t, err, "falha de auditoria não derruba o turno")
	assert.Equal(t, harness.StopCompleted, result.StopReason)
	assert.Empty(t, sink.Events)
	require.Len(t, handler.errs, 3, "dois model_call + um tool_call")

	var escopos []harness.ErrorScope
	for _, ev := range handler.errs {
		assert.Equal(t, "falha ao registrar auditoria", ev.Message)
		assert.False(t, ev.Retryable)
		escopos = append(escopos, ev.Scope)
	}
	assert.Equal(t, []harness.ErrorScope{harness.ErrorModel, harness.ErrorTool, harness.ErrorModel}, escopos)
}
