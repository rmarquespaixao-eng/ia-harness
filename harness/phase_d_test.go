package harness_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cacheinmem "github.com/rmarquespaixao-eng/ia-harness/adapters/cache/inmem"
	"github.com/rmarquespaixao-eng/ia-harness/harness"
	"github.com/rmarquespaixao-eng/ia-harness/internal/testutil"
)

// phaseDFixedClock devolve sempre o mesmo instante nos testes da Fase D.
type phaseDFixedClock struct{ now time.Time }

// Now devolve o instante fixo.
func (c phaseDFixedClock) Now() time.Time { return c.now }

// recordingWaiter registra as esperas sem dormir de verdade.
type recordingWaiter struct {
	waits []time.Duration
}

// Wait registra o prazo pedido e respeita o cancelamento do contexto.
func (w *recordingWaiter) Wait(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	w.waits = append(w.waits, d)
	return nil
}

var _ harness.Waiter = (*recordingWaiter)(nil)

// rateLimitRecorder observa os eventos de throttling (embedding NopHandler).
type rateLimitRecorder struct {
	harness.NopHandler
	events []harness.RateLimitEvent
}

// RateLimited acumula o evento.
func (r *rateLimitRecorder) RateLimited(_ context.Context, ev harness.RateLimitEvent) {
	r.events = append(r.events, ev)
}

var _ harness.RateLimitHandler = (*rateLimitRecorder)(nil)

// cacheRecorder observa os eventos de cache (embedding NopHandler).
type cacheRecorder struct {
	harness.NopHandler
	events []harness.CacheEvent
}

// Cache acumula o evento.
func (r *cacheRecorder) Cache(_ context.Context, ev harness.CacheEvent) {
	r.events = append(r.events, ev)
}

var _ harness.CacheHandler = (*cacheRecorder)(nil)

// subAgentRecorder observa delegações e resultados de tool.
type subAgentRecorder struct {
	harness.NopHandler
	events  []harness.SubAgentEvent
	results []harness.ToolResultEvent
}

// SubAgent acumula o evento de delegação.
func (r *subAgentRecorder) SubAgent(_ context.Context, ev harness.SubAgentEvent) {
	r.events = append(r.events, ev)
}

// ToolResult acumula o resultado de tool.
func (r *subAgentRecorder) ToolResult(_ context.Context, ev harness.ToolResultEvent) {
	r.results = append(r.results, ev)
}

var (
	_ harness.SubAgentHandler = (*subAgentRecorder)(nil)
	_ harness.Handler         = (*subAgentRecorder)(nil)
)

// snapshotStore persiste sessões pelo contrato (Snapshot/Restore), provando que
// o checkpoint durável sobrevive à serialização.
type snapshotStore struct {
	mu   sync.Mutex
	data map[string]harness.SessionSnapshot
}

// newSnapshotStore cria o store vazio.
func newSnapshotStore() *snapshotStore {
	return &snapshotStore{data: map[string]harness.SessionSnapshot{}}
}

// Save serializa a sessão pelo contrato.
func (s *snapshotStore) Save(_ context.Context, session *harness.Session) error {
	snap, err := harness.SnapshotSession(session)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[session.ID] = snap
	return nil
}

// Load restaura a sessão do snapshot.
func (s *snapshotStore) Load(_ context.Context, sessionID string) (*harness.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, ok := s.data[sessionID]
	if !ok {
		return nil, errSessionNotFound
	}
	return harness.RestoreSession(snap)
}

var _ harness.SessionStore = (*snapshotStore)(nil)

// phaseDConfig monta uma Config mínima válida com os ajustes informados.
func phaseDConfig(provider harness.Provider, store harness.SessionStore, clk harness.Clock) harness.Config {
	return harness.Config{
		Providers:    map[string]harness.Provider{"scripted": provider},
		Models:       map[string]harness.ModelProfile{"default": {Provider: "scripted", Model: "modelo-teste", Capabilities: harness.Capabilities{ToolCalling: true}}},
		DefaultModel: "default",
		Credentials:  &testutil.FakeCredentialProvider{Values: map[string]string{}},
		Sessions:     store,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Clock:        clk,
	}
}

func TestRateLimit_ThrottleEsperaEEvento(t *testing.T) {
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "ok-1"}, {Text: "ok-2"}}}
	waiter := &recordingWaiter{}
	recorder := &rateLimitRecorder{}
	cfg := phaseDConfig(provider, newMemSessions(), phaseDFixedClock{now: time.Unix(0, 0)})
	cfg.Waiter = waiter
	cfg.DefaultRateLimit = harness.RateLimit{RequestsPerMinute: 60, Burst: 1, MaxWait: time.Minute}

	h, err := harness.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })

	for i := 0; i < 2; i++ {
		_, err := h.Run(context.Background(), harness.RunRequest{
			UserID: "u1",
			Model:  "default",
			Input:  []harness.Part{{Kind: harness.PartText, Text: "oi"}},
		}, recorder)
		require.NoError(t, err)
	}

	assert.Equal(t, 2, provider.Calls)
	require.Len(t, waiter.waits, 1)
	assert.Greater(t, waiter.waits[0], time.Duration(0))
	require.Len(t, recorder.events, 1)
	assert.Equal(t, "scripted", recorder.events[0].Provider)
}

func TestRateLimit_TetoDeEspera(t *testing.T) {
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "ok-1"}, {Text: "ok-2"}}}
	cfg := phaseDConfig(provider, newMemSessions(), phaseDFixedClock{now: time.Unix(0, 0)})
	cfg.Waiter = &recordingWaiter{}
	cfg.DefaultRateLimit = harness.RateLimit{RequestsPerMinute: 1, Burst: 1, MaxWait: 100 * time.Millisecond}

	h, err := harness.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })

	first := harness.RunRequest{UserID: "u1", Model: "default", Input: []harness.Part{{Kind: harness.PartText, Text: "oi"}}}
	_, err = h.Run(context.Background(), first, nil)
	require.NoError(t, err)

	_, err = h.Run(context.Background(), first, nil)
	require.ErrorContains(t, err, "ratelimit/espera-excedida")
	assert.Equal(t, 1, provider.Calls)
}

func TestCacheSemantico_HitNaoChamaProvider(t *testing.T) {
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "resposta canonica"}}}
	recorder := &cacheRecorder{}
	cfg := phaseDConfig(provider, newMemSessions(), phaseDFixedClock{now: time.Unix(0, 0)})
	cfg.Cache = harness.SemanticCacheConfig{Enabled: true, MinScore: 0.9, MaxEntries: 10}
	cfg.Embedder = &fakeEmbedder{}
	cfg.CacheStore = cacheinmem.New(cfg.Cache, cfg.Clock)

	h, err := harness.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })

	req := harness.RunRequest{UserID: "u1", Model: "default", Input: []harness.Part{{Kind: harness.PartText, Text: "qual o saldo?"}}}
	first, err := h.Run(context.Background(), req, recorder)
	require.NoError(t, err)
	require.Nil(t, first.Cache)

	second, err := h.Run(context.Background(), req, recorder)
	require.NoError(t, err)

	assert.Equal(t, 1, provider.Calls, "o provedor só deve ser chamado no miss")
	require.NotNil(t, second.Cache)
	assert.True(t, second.Cache.Hit)
	assert.Zero(t, second.Usage.CostMicros)
	assert.Zero(t, second.Usage.InputTokens)
	require.Len(t, recorder.events, 2)
	assert.False(t, recorder.events[0].Hit)
	assert.True(t, recorder.events[1].Hit)
}

func TestCacheSemantico_TurnoComToolsIgnora(t *testing.T) {
	tools := &testutil.MemToolSource{Tools: []harness.Tool{{Name: "consultar"}}}
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "ok-1"}, {Text: "ok-2"}}}
	cfg := phaseDConfig(provider, newMemSessions(), phaseDFixedClock{now: time.Unix(0, 0)})
	cfg.Tools = []harness.ToolSource{tools}
	cfg.Cache = harness.SemanticCacheConfig{Enabled: true, MinScore: 0.9}
	cfg.Embedder = &fakeEmbedder{}
	cfg.CacheStore = cacheinmem.New(cfg.Cache, cfg.Clock)

	h, err := harness.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })

	req := harness.RunRequest{UserID: "u1", Model: "default", Input: []harness.Part{{Kind: harness.PartText, Text: "oi"}}}
	first, err := h.Run(context.Background(), req, nil)
	require.NoError(t, err)
	second, err := h.Run(context.Background(), req, nil)
	require.NoError(t, err)

	assert.Equal(t, 2, provider.Calls)
	assert.Nil(t, first.Cache)
	assert.Nil(t, second.Cache)
}

func TestDurable_RetomadaNaoReexecutaTool(t *testing.T) {
	tools := &testutil.MemToolSource{
		Tools:   []harness.Tool{{Name: "t1", Idempotent: false}},
		Results: map[string]harness.ToolResult{"t1": {Content: []harness.ResultContent{{Kind: harness.ResultText, Text: "efeito aplicado"}}}},
	}
	store := newSnapshotStore()
	cfg := phaseDConfig(&testutil.ScriptedProvider{}, store, phaseDFixedClock{now: time.Unix(0, 0)})
	cfg.Durable = true
	cfg.Tools = []harness.ToolSource{tools}
	cfg.Policy = harness.PolicyConfig{Default: harness.PolicyAllow}

	// Primeira execução: tool roda e a chamada seguinte de modelo falha (queda
	// simulada), deixando o checkpoint running no store.
	provider1 := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{
		{ToolCalls: []harness.ToolCall{{ID: "c1", Name: "t1", Args: json.RawMessage(`{}`)}}},
		{Err: errors.New("queda simulada")},
	}}
	cfg.Providers = map[string]harness.Provider{"scripted": provider1}
	h1, err := harness.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h1.Close() })

	runReq := harness.RunRequest{UserID: "u1", Model: "default", Input: []harness.Part{{Kind: harness.PartText, Text: "faça"}}}
	result, err := h1.Run(context.Background(), runReq, nil)
	require.Error(t, err)
	require.Len(t, tools.Calls, 1)
	require.NotEmpty(t, result.SessionID)

	// Segunda execução (novo harness, mesmo store): retoma sem reexecutar t1.
	provider2 := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "concluído após retomada"}}}
	cfg.Providers = map[string]harness.Provider{"scripted": provider2}
	h2, err := harness.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h2.Close() })

	resumed, err := h2.Run(context.Background(), harness.RunRequest{SessionID: result.SessionID, UserID: "u1"}, nil)
	require.NoError(t, err)
	assert.True(t, resumed.Resumed)
	assert.Equal(t, 1, len(tools.Calls), "a tool já executada não pode ser repetida")
	assert.Contains(t, outputTextOf(resumed.Output), "concluído após retomada")
	assert.Equal(t, 1, provider2.Calls)
}

func TestMultiAgente_Delegacao(t *testing.T) {
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{
		{ToolCalls: []harness.ToolCall{{ID: "c1", Namespace: "agents", Name: "delegate", Args: json.RawMessage(`{"agent":"resumo","task":"resuma o mês"}`)}}},
		{Text: "resposta do sub-agente"},
		{Text: "resposta final do orquestrador"},
	}}
	store := newMemSessions()
	recorder := &subAgentRecorder{}
	cfg := phaseDConfig(provider, store, phaseDFixedClock{now: time.Unix(0, 0)})
	cfg.Policy = harness.PolicyConfig{Default: harness.PolicyAllow}
	cfg.Agents = map[string]harness.AgentSpec{"resumo": {Description: "especialista em resumo", SystemPrompt: "você resume"}}
	cfg.MaxAgentDepth = 1

	h, err := harness.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })

	result, err := h.Run(context.Background(), harness.RunRequest{UserID: "u1", Model: "default", Input: []harness.Part{{Kind: harness.PartText, Text: "resuma"}}}, recorder)
	require.NoError(t, err)

	assert.Contains(t, outputTextOf(result.Output), "resposta final do orquestrador")
	require.Len(t, recorder.events, 2)
	assert.Equal(t, "resumo", recorder.events[0].Agent)
	assert.Equal(t, "completed", recorder.events[1].Status)
	require.Len(t, recorder.results, 1)
	assert.Contains(t, recorder.results[0].ResultSummary, "resposta do sub-agente")
	assert.Len(t, store.sessions, 2, "o sub-agente roda em sessão própria")
}

// outputTextOf concatena as partes de texto de uma saída.
func outputTextOf(parts []harness.Part) string {
	var b strings.Builder
	for _, part := range parts {
		if part.Kind == harness.PartText {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}
