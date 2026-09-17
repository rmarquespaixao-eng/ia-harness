package engine

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// routeStep é um passo roteirizado do provider fake interno.
type routeStep struct {
	Text string
	Err  error
}

// routeScriptedProvider espelha o contrato do testutil.ScriptedProvider, mas é
// local: o teste interno (package harness) não pode importar internal/testutil
// — o pacote importa harness e criaria ciclo de import de teste.
type routeScriptedProvider struct {
	Steps    []routeStep
	Calls    int
	Requests []ChatRequest
}

// Chat consome o próximo step e emite o texto em uma única passada.
func (p *routeScriptedProvider) Chat(_ context.Context, req ChatRequest, onText func(string)) (ChatResponse, error) {
	p.Requests = append(p.Requests, req)
	p.Calls++
	if p.Calls > len(p.Steps) {
		return ChatResponse{}, errors.New("route: script esgotado")
	}
	step := p.Steps[p.Calls-1]
	if step.Err != nil {
		return ChatResponse{}, step.Err
	}
	if onText != nil {
		onText(step.Text)
	}
	return ChatResponse{
		Message: Message{Role: RoleAssistant, Parts: []Part{{Kind: PartText, Text: step.Text}}},
	}, nil
}

// Close satisfaz Provider.
func (p *routeScriptedProvider) Close() error { return nil }

type routeCredentials struct{}

func (routeCredentials) Resolve(context.Context, string) (string, error) { return "", nil }

type routeSessions struct{}

func (routeSessions) Load(context.Context, string) (*Session, error) { return nil, nil }
func (routeSessions) Save(context.Context, *Session) error           { return nil }

var _ Provider = (*routeScriptedProvider)(nil)

// routeSink coleta os deltas de texto para os testes de rota.
type routeSink struct{ deltas *[]string }

func (s routeSink) Text(t string)                     { *s.deltas = append(*s.deltas, t) }
func (routeSink) Reasoning(string)                    {}
func (routeSink) ToolCallArgs(string, string, string) {}

// routeConfig monta a configuração primário+fallback usada nos testes de rota.
func routeConfig(primary, backup Provider) Config {
	return Config{
		Providers: map[string]Provider{"p-primary": primary, "p-backup": backup},
		Models: map[string]ModelProfile{
			"primary": {Provider: "p-primary", Model: "modelo-primario", Fallbacks: []string{"backup"}},
			"backup":  {Provider: "p-backup", Model: "modelo-backup"},
		},
		DefaultModel: "primary",
		Credentials:  routeCredentials{},
		Sessions:     routeSessions{},
		Logger:       slog.Default(),
	}
}

func newRouteHarness(t *testing.T, cfg Config) *Harness {
	t.Helper()
	h, err := New(cfg)
	require.NoError(t, err)
	return h
}

func TestRouteResolveModel(t *testing.T) {
	// Arrange
	h := newRouteHarness(t, routeConfig(&routeScriptedProvider{}, &routeScriptedProvider{}))

	t.Run("alias vazio usa o default", func(t *testing.T) {
		// Act
		profile, providerKey, err := h.resolveModel("")

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "modelo-primario", profile.Model)
		assert.Equal(t, "p-primary", providerKey)
	})

	t.Run("alias explícito resolve o perfil", func(t *testing.T) {
		// Act
		profile, providerKey, err := h.resolveModel("backup")

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "modelo-backup", profile.Model)
		assert.Equal(t, "p-backup", providerKey)
	})

	t.Run("alias desconhecido falha nomeado", func(t *testing.T) {
		// Act
		_, _, err := h.resolveModel("fantasma")

		// Assert
		var cfgErr *ConfigError
		require.ErrorAs(t, err, &cfgErr)
		assert.Equal(t, "model/alias-desconhecido", cfgErr.Code)
	})

	t.Run("provider não injetado falha nomeado", func(t *testing.T) {
		// Arrange
		h.cfg.Providers = map[string]Provider{}

		// Act
		_, _, err := h.resolveModel("primary")

		// Assert
		var cfgErr *ConfigError
		require.ErrorAs(t, err, &cfgErr)
		assert.Equal(t, "model/provider-desconhecido", cfgErr.Code)
	})
}

func TestRouteChatPrimarySuccess(t *testing.T) {
	// Arrange
	primary := &routeScriptedProvider{Steps: []routeStep{{Text: "resposta primária"}}}
	backup := &routeScriptedProvider{Steps: []routeStep{{Text: "resposta backup"}}}
	h := newRouteHarness(t, routeConfig(primary, backup))
	profile, providerKey, err := h.resolveModel("primary")
	require.NoError(t, err)
	var deltas []string

	// Act
	resp, effective, err := h.chat(context.Background(), profile, providerKey, ChatRequest{}, routeSink{deltas: &deltas}, NopHandler{})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "primary", effective, "modelo efetivo é o alias, não a chave do provider")
	assert.Equal(t, "resposta primária", resp.Message.Parts[0].Text)
	assert.Equal(t, []string{"resposta primária"}, deltas)
	assert.Equal(t, 1, primary.Calls)
	assert.Zero(t, backup.Calls, "fallback não é tentado quando o primário responde")
	assert.Equal(t, "modelo-primario", primary.Requests[0].Model)
}

func TestRouteChatFallbackAfterPrimaryError(t *testing.T) {
	// Arrange
	primary := &routeScriptedProvider{Steps: []routeStep{{Err: errors.New("429 rate limited")}}}
	backup := &routeScriptedProvider{Steps: []routeStep{{Text: "resposta do fallback"}}}
	h := newRouteHarness(t, routeConfig(primary, backup))
	profile, providerKey, err := h.resolveModel("primary")
	require.NoError(t, err)

	// Act
	resp, effective, err := h.chat(context.Background(), profile, providerKey, ChatRequest{}, nil, NopHandler{})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "backup", effective)
	assert.Equal(t, "resposta do fallback", resp.Message.Parts[0].Text)
	assert.Equal(t, 1, primary.Calls, "primário é tentado primeiro")
	assert.Equal(t, 1, backup.Calls, "fallback é tentado na sequência")
	assert.Equal(t, "modelo-primario", primary.Requests[0].Model)
	assert.Equal(t, "modelo-backup", backup.Requests[0].Model, "ChatRequest.Model acompanha o perfil efetivo")
}

func TestRouteChatFallbackAllModelsFail(t *testing.T) {
	// Arrange
	errPrimary := errors.New("429 primário")
	errBackup := errors.New("503 backup")
	primary := &routeScriptedProvider{Steps: []routeStep{{Err: errPrimary}}}
	backup := &routeScriptedProvider{Steps: []routeStep{{Err: errBackup}}}
	h := newRouteHarness(t, routeConfig(primary, backup))
	profile, providerKey, err := h.resolveModel("primary")
	require.NoError(t, err)

	// Act
	_, effective, err := h.chat(context.Background(), profile, providerKey, ChatRequest{}, nil, NopHandler{})

	// Assert
	require.Error(t, err)
	assert.ErrorIs(t, err, errPrimary, "causa do primário preservada")
	assert.ErrorIs(t, err, errBackup, "último erro preservado no wrap")
	assert.Contains(t, err.Error(), `harness: todos os modelos falharam (primário "primary")`)
	assert.Equal(t, "primary", effective)
	assert.Equal(t, 1, primary.Calls)
	assert.Equal(t, 1, backup.Calls)
}

func TestRouteChatFallbackEmpty(t *testing.T) {
	// Arrange
	errPrimary := errors.New("provider fora do ar")
	primary := &routeScriptedProvider{Steps: []routeStep{{Err: errPrimary}}}
	cfg := routeConfig(primary, &routeScriptedProvider{})
	cfg.Models["primary"] = ModelProfile{Provider: "p-primary", Model: "modelo-primario"}
	h := newRouteHarness(t, cfg)
	profile, providerKey, err := h.resolveModel("primary")
	require.NoError(t, err)

	// Act
	_, _, err = h.chat(context.Background(), profile, providerKey, ChatRequest{}, nil, NopHandler{})

	// Assert
	require.Error(t, err)
	assert.ErrorIs(t, err, errPrimary)
	assert.Equal(t, 1, primary.Calls)
}

func TestRouteChatFallbackSkipsConfigError(t *testing.T) {
	// Arrange: alias inválido no meio da cadeia não derruba a tentativa seguinte.
	errPrimary := errors.New("429 primário")
	primary := &routeScriptedProvider{Steps: []routeStep{{Err: errPrimary}}}
	backup := &routeScriptedProvider{Steps: []routeStep{{Text: "resposta do segundo fallback"}}}
	cfg := routeConfig(primary, backup)
	cfg.Models["primary"] = ModelProfile{Provider: "p-primary", Model: "modelo-primario", Fallbacks: []string{"fantasma", "backup"}}
	h := newRouteHarness(t, cfg)
	profile, providerKey, err := h.resolveModel("primary")
	require.NoError(t, err)

	// Act
	resp, effective, err := h.chat(context.Background(), profile, providerKey, ChatRequest{}, nil, NopHandler{})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "backup", effective)
	assert.Equal(t, "resposta do segundo fallback", resp.Message.Parts[0].Text)
	assert.Equal(t, 1, backup.Calls)
}

func TestRouteChatFallbackConfigErrorIsReported(t *testing.T) {
	// Arrange
	errPrimary := errors.New("429 primário")
	primary := &routeScriptedProvider{Steps: []routeStep{{Err: errPrimary}}}
	cfg := routeConfig(primary, &routeScriptedProvider{})
	cfg.Models["primary"] = ModelProfile{Provider: "p-primary", Model: "modelo-primario", Fallbacks: []string{"fantasma"}}
	h := newRouteHarness(t, cfg)
	profile, providerKey, err := h.resolveModel("primary")
	require.NoError(t, err)

	// Act
	_, _, err = h.chat(context.Background(), profile, providerKey, ChatRequest{}, nil, NopHandler{})

	// Assert
	require.Error(t, err)
	assert.ErrorIs(t, err, errPrimary)
	var cfgErr *ConfigError
	require.ErrorAs(t, err, &cfgErr)
	assert.Equal(t, "model/alias-desconhecido", cfgErr.Code)
	assert.Contains(t, err.Error(), `"fantasma"`, "alias inválido registrado no erro final")
}
