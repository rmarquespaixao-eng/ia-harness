package engine

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// minimalConfig monta uma config válida com stubs locais.
func minimalConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		Providers:   map[string]Provider{"stub": stubProvider{}},
		Models:      map[string]ModelProfile{"default": {Provider: "stub", Model: "stub-1"}},
		Credentials: stubCredentials{},
		Sessions:    stubSessions{},
		Logger:      slog.Default(),
	}
}

type stubProvider struct{}

func (p stubProvider) Chat(context.Context, ChatRequest, func(string)) (ChatResponse, error) {
	return ChatResponse{}, nil
}
func (stubProvider) Close() error { return nil }

type stubCredentials struct{}

func (stubCredentials) Resolve(context.Context, string) (string, error) { return "", nil }

type stubSessions struct {
	session *Session
	err     error
}

func (s stubSessions) Load(context.Context, string) (*Session, error) { return s.session, s.err }
func (stubSessions) Save(context.Context, *Session) error             { return nil }

func TestNewValidatesRequiredPorts(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*Config)
		wantCode string
	}{
		{"providers vazio", func(c *Config) { c.Providers = nil }, "config/providers-vazio"},
		{"provider nulo", func(c *Config) { c.Providers["nulo"] = nil }, "config/provider-nulo"},
		{"toolsource nulo", func(c *Config) { c.Tools = []ToolSource{nil} }, "config/toolsource-nulo"},
		{"modelo órfão", func(c *Config) { c.Models["orfao"] = ModelProfile{Provider: "nao-existe"} }, "config/model-provider-desconhecido"},
		{"default model desconhecido", func(c *Config) { c.DefaultModel = "nao-existe" }, "config/default-model-desconhecido"},
		{"credentials ausente", func(c *Config) { c.Credentials = nil }, "config/credentials-obrigatorio"},
		{"sessions ausente", func(c *Config) { c.Sessions = nil }, "config/sessions-obrigatorio"},
		{"logger ausente", func(c *Config) { c.Logger = nil }, "config/logger-obrigatorio"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := minimalConfig(t)
			tt.mutate(&cfg)

			// Act
			_, err := New(cfg)

			// Assert
			var cfgErr *ConfigError
			require.Error(t, err)
			require.ErrorAs(t, err, &cfgErr)
			assert.Equal(t, tt.wantCode, cfgErr.Code)
		})
	}
}

func TestNewAppliesDefaults(t *testing.T) {
	// Arrange
	cfg := minimalConfig(t)

	// Act
	h, err := New(cfg)

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, h.cfg.Clock)
	assert.NotNil(t, h.cfg.Audit)
	assert.Equal(t, 16*1024, h.cfg.Redaction.MaxFieldBytes)
	assert.Equal(t, StrategyTruncateOldest, h.cfg.Context.Strategy)
	assert.InDelta(t, 4, h.cfg.Pricing.BytesPerToken, 0)
}

func TestNewAcceptsEmptyTools(t *testing.T) {
	// Arrange
	cfg := minimalConfig(t)
	cfg.Tools = nil

	// Act
	_, err := New(cfg)

	// Assert
	require.NoError(t, err, "agente sem tools é válido")
}

func TestSessionRequiresIDAndWrapsStoreError(t *testing.T) {
	// Arrange
	h, err := New(minimalConfig(t))
	require.NoError(t, err)
	sentinel := errors.New("boom")
	h.cfg.Sessions = stubSessions{err: sentinel}

	// Act
	_, errEmpty := h.Session(context.Background(), "")
	_, errLoad := h.Session(context.Background(), "sess-1")

	// Assert
	var cfgErr *ConfigError
	require.ErrorAs(t, errEmpty, &cfgErr)
	assert.True(t, errors.Is(errLoad, sentinel), "erro do store deve ser preservado com %%w")
}

func TestCloseIsIdempotent(t *testing.T) {
	// Arrange
	h, err := New(minimalConfig(t))
	require.NoError(t, err)

	// Act
	errFirst := h.Close()
	errSecond := h.Close()

	// Assert
	require.NoError(t, errFirst)
	require.NoError(t, errSecond)
}

func TestClockIsUsedByDefault(t *testing.T) {
	// Arrange
	h, err := New(minimalConfig(t))
	require.NoError(t, err)

	// Act
	before := time.Now()
	now := h.cfg.Clock.Now()

	// Assert
	assert.WithinDuration(t, before, now, time.Second)
}
