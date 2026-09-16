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

	"github.com/rmarquespaixao-eng/ia-harness/adapters/session/memory"
	"github.com/rmarquespaixao-eng/ia-harness/harness"
	"github.com/rmarquespaixao-eng/ia-harness/internal/testutil"
)

// cuHar4Clock devolve sempre o mesmo instante (sem relógio real no teste).
type cuHar4Clock struct{ at time.Time }

func (c cuHar4Clock) Now() time.Time { return c.at }

// cuHar4StoreFalho é um SessionStore local cujo Save sempre falha (T064).
type cuHar4StoreFalho struct{ err error }

func (s cuHar4StoreFalho) Load(context.Context, string) (*harness.Session, error) {
	return nil, s.err
}

func (s cuHar4StoreFalho) Save(context.Context, *harness.Session) error { return s.err }

// cuHar4NewHarness monta o harness com o store e o provider injetados; mutate
// ajusta a config do cenário (ex.: janela de contexto).
func cuHar4NewHarness(t *testing.T, sessions harness.SessionStore, provider harness.Provider, mutate func(*harness.Config)) *harness.Harness {
	t.Helper()
	cfg := harness.Config{
		Providers: map[string]harness.Provider{"stub": provider},
		Models: map[string]harness.ModelProfile{
			"default": {Provider: "stub", Model: "stub-1"},
		},
		DefaultModel: "default",
		Credentials:  &testutil.FakeCredentialProvider{Values: map[string]string{}},
		Sessions:     sessions,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Clock:        cuHar4Clock{at: time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC)},
	}
	if mutate != nil {
		mutate(&cfg)
	}
	h, err := harness.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, h.Close()) })
	return h
}

// cuHar4Input cria a entrada textual de um turno.
func cuHar4Input(text string) []harness.Part {
	return []harness.Part{{Kind: harness.PartText, Text: text}}
}

func TestCU_HAR4_RetomadaAposRestartCarregaHistoricoEContinuaOTurno(t *testing.T) {
	// Arrange — primeiro harness grava a sessão no store compartilhado.
	ctx := context.Background()
	store := memory.New()
	primeiro := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "primeira resposta"}}}
	harnessUm := cuHar4NewHarness(t, store, primeiro, nil)
	primeira, err := harnessUm.Run(ctx, harness.RunRequest{
		UserID: "user-1",
		Input:  cuHar4Input("primeira pergunta"),
	}, harness.NopHandler{})
	require.NoError(t, err)
	require.Equal(t, harness.StopCompleted, primeira.StopReason)

	// Act — o processo "reinicia": novo Harness, mesmo SessionStore, novo provider.
	segundo := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "resposta retomada"}}}
	harnessDois := cuHar4NewHarness(t, store, segundo, nil)
	continuacao, err := harnessDois.Run(ctx, harness.RunRequest{
		SessionID: primeira.SessionID,
		UserID:    "user-1",
		Input:     cuHar4Input("segunda pergunta"),
	}, harness.NopHandler{})

	// Assert — histórico persistido reenviado ao modelo sem reenvio manual.
	// (A entrada atual é sempre a última; o histórico persistido inclui a
	// resposta anterior do assistente.)
	require.NoError(t, err)
	assert.Equal(t, primeira.SessionID, continuacao.SessionID)
	require.Len(t, segundo.Requests, 1)
	mensagens := segundo.Requests[0].Messages
	require.NotEmpty(t, mensagens)
	var textos []string
	for _, mensagem := range mensagens {
		for _, parte := range mensagem.Parts {
			textos = append(textos, parte.Text)
		}
	}
	assert.Contains(t, textos, "primeira resposta", "histórico persistido deve ser reenviado")
	assert.Equal(t, "segunda pergunta", textos[len(textos)-1], "entrada atual é sempre a última")
}

func TestCU_HAR4_JanelaAplicadaNaRetomada(t *testing.T) {
	// Arrange — sessão com histórico persistido.
	ctx := context.Background()
	store := memory.New()
	criador := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "resposta antiga"}}}
	harnessCriador := cuHar4NewHarness(t, store, criador, nil)
	primeira, err := harnessCriador.Run(ctx, harness.RunRequest{
		UserID: "user-1",
		Input:  cuHar4Input("pergunta antiga"),
	}, harness.NopHandler{})
	require.NoError(t, err)

	// Act — novo harness com orçamento mínimo: só a entrada atual entra.
	continuador := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "ok"}}}
	harnessContinuador := cuHar4NewHarness(t, store, continuador, func(cfg *harness.Config) {
		cfg.Context = harness.ContextPolicy{MaxTokens: 1}
	})
	_, err = harnessContinuador.Run(ctx, harness.RunRequest{
		SessionID: primeira.SessionID,
		UserID:    "user-1",
		Input:     cuHar4Input("pedido atual"),
	}, harness.NopHandler{})

	// Assert — histórico cortado, entrada atual preservada.
	require.NoError(t, err)
	require.Len(t, continuador.Requests, 1)
	mensagens := continuador.Requests[0].Messages
	require.Len(t, mensagens, 1)
	assert.Equal(t, harness.RoleUser, mensagens[0].Role)
	assert.Equal(t, "pedido atual", mensagens[0].Parts[0].Text)
}

func TestCU_HAR4_IsolamentoEntreUsuariosESessoes(t *testing.T) {
	// Arrange — sessão do user-1 no store compartilhado.
	ctx := context.Background()
	store := memory.New()
	providerUm := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "resposta do user-1"}}}
	harnessUm := cuHar4NewHarness(t, store, providerUm, nil)
	primeira, err := harnessUm.Run(ctx, harness.RunRequest{
		UserID: "user-1",
		Input:  cuHar4Input("segredo do user-1"),
	}, harness.NopHandler{})
	require.NoError(t, err)

	// Act — user-2 tenta retomar a sessão do user-1.
	_, errIntruso := harnessUm.Run(ctx, harness.RunRequest{
		SessionID: primeira.SessionID,
		UserID:    "user-2",
		Input:     cuHar4Input("invasão"),
	}, harness.NopHandler{})

	// Assert — erro nomeado de isolamento e nenhuma chamada nova ao provider.
	var cfgErr *harness.ConfigError
	require.Error(t, errIntruso)
	require.ErrorAs(t, errIntruso, &cfgErr)
	assert.Equal(t, "run/sessao-de-outro-usuario", cfgErr.Code)
	assert.Equal(t, 1, providerUm.Calls, "nenhuma chamada ao modelo após a violação de isolamento")

	// Sessões distintas no mesmo store não se enxergam.
	providerDois := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "resposta do user-2"}}}
	harnessDois := cuHar4NewHarness(t, store, providerDois, nil)
	segunda, err := harnessDois.Run(ctx, harness.RunRequest{
		UserID: "user-2",
		Input:  cuHar4Input("mensagem do user-2"),
	}, harness.NopHandler{})
	require.NoError(t, err)
	require.NotEqual(t, primeira.SessionID, segunda.SessionID)
	require.Len(t, providerDois.Requests, 1)
	serializado, err := json.Marshal(providerDois.Requests[0].Messages)
	require.NoError(t, err)
	assert.Contains(t, string(serializado), "mensagem do user-2")
	assert.NotContains(t, string(serializado), "segredo do user-1")
}

func TestCU_HAR4_SessionIDInexistenteDevolveErro(t *testing.T) {
	// Arrange
	ctx := context.Background()
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "não deve responder"}}}
	h := cuHar4NewHarness(t, memory.New(), provider, nil)

	// Act
	_, err := h.Run(ctx, harness.RunRequest{
		SessionID: "fantasma",
		UserID:    "user-1",
		Input:     cuHar4Input("oi"),
	}, harness.NopHandler{})

	// Assert — sem sessão fantasma e sem chamada ao modelo.
	require.Error(t, err)
	assert.ErrorIs(t, err, memory.ErrNotFound)
	assert.Zero(t, provider.Calls)
}

func TestCU_HAR4_FalhaDoSaveFazOTurnoFalharSemRespostaConfirmada(t *testing.T) {
	// Arrange — store T064: Save sempre falha.
	ctx := context.Background()
	sentinela := errors.New("falha de persistência")
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "resposta que não pode ser confirmada"}}}
	h := cuHar4NewHarness(t, cuHar4StoreFalho{err: sentinela}, provider, nil)

	// Act
	resultado, err := h.Run(ctx, harness.RunRequest{
		UserID: "user-1",
		Input:  cuHar4Input("pergunta"),
	}, harness.NopHandler{})

	// Assert — erro propagado com causa; o turno não vale como concluído.
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinela)
	assert.ErrorContains(t, err, "salvar sessão")
	assert.Equal(t, 1, provider.Calls, "o modelo foi chamado, mas nada foi persistido")
	assert.Equal(t, harness.StopCompleted, resultado.StopReason, "a falha está na persistência, não no turno")
}
