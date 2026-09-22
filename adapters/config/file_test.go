package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/adapters/config"
	"github.com/rmarquespaixao-eng/ia-harness/harness"
)

const validJSON = `{
  "default_model": "go-luna",
  "system_prompt": "Você é o assistente financeiro.",
  "models": {
    "go-luna": {
      "provider": "opencode-go-responses",
      "model": "gpt-5.6-luna",
      "capabilities": {"tool_calling": true, "streaming": true, "prompt_caching": true, "vision": true, "max_media_bytes": 1048576},
      "fallbacks": ["go-kimi"]
    },
    "go-kimi": {"provider": "opencode-go-chat", "model": "kimi-k3"}
  },
  "policy": {
    "default": "allow",
    "agents": {
      "financeiro-chat": {
        "mode": "allow",
        "allow_tools": ["financeiro.*"],
        "confirm_tools": ["financeiro.excluir_transacao"],
        "system_prompt": "Você é o agente de conciliação.",
        "overrides": {"financeiro.excluir_transacao": {"confirm": true, "timeout_seconds": 30}}
      }
    }
  },
  "pricing": {"currency": "USD", "input_price_micros_per_million": 3000000, "output_price_micros_per_million": 15000000, "bytes_per_token": 4, "cached_input_price_micros_per_million": 300000},
  "context": {"max_tokens": 100000, "strategy": "truncate_oldest"},
  "redaction": {"sensitive_keys": ["api_key"], "max_field_bytes": 4096},
  "mcp_servers": [{"name": "financeiro", "endpoint": "https://x/mcp", "credential_ref": "env:KEY", "tool_timeout_seconds": 120}]
}`

func TestParseApply(t *testing.T) {
	// Act
	file, err := config.Parse([]byte(validJSON))
	require.NoError(t, err)
	var cfg harness.Config
	file.Apply(&cfg)

	// Assert — campos aplicados ao Config do host.
	assert.Equal(t, "go-luna", cfg.DefaultModel)
	assert.Equal(t, "Você é o assistente financeiro.", cfg.SystemPrompt)

	require.Contains(t, cfg.Models, "go-luna")
	luna := cfg.Models["go-luna"]
	assert.Equal(t, "opencode-go-responses", luna.Provider)
	assert.Equal(t, "gpt-5.6-luna", luna.Model)
	assert.True(t, luna.Capabilities.PromptCaching)
	assert.True(t, luna.Capabilities.Vision)
	assert.Equal(t, int64(1048576), luna.Capabilities.MaxMediaBytes)
	assert.Equal(t, []string{"go-kimi"}, luna.Fallbacks)

	assert.Equal(t, harness.PolicyAllow, cfg.Policy.Default)
	agent, ok := cfg.Policy.Agents["financeiro-chat"]
	require.True(t, ok)
	assert.Equal(t, harness.PolicyAllow, agent.Mode)
	assert.Equal(t, []string{"financeiro.*"}, agent.AllowTools)
	assert.Equal(t, "Você é o agente de conciliação.", agent.SystemPrompt)
	override, ok := agent.Overrides["financeiro.excluir_transacao"]
	require.True(t, ok)
	assert.True(t, override.Confirm)
	assert.Equal(t, 30*time.Second, override.Timeout)

	assert.Equal(t, "USD", cfg.Pricing.Currency)
	assert.Equal(t, int64(3000000), cfg.Pricing.InputPriceMicrosPerMillion)
	assert.Equal(t, int64(300000), cfg.Pricing.CachedInputPriceMicrosPerMillion)
	assert.Equal(t, 100000, cfg.Context.MaxTokens)
	assert.Equal(t, harness.StrategyTruncateOldest, cfg.Context.Strategy)
	assert.Equal(t, 4096, cfg.Redaction.MaxFieldBytes)

	servers := file.MCPServers()
	require.Len(t, servers, 1)
	assert.Equal(t, "financeiro", servers[0].Name)
	assert.Equal(t, "env:KEY", servers[0].CredentialRef)
	assert.Equal(t, 120*time.Second, servers[0].ToolTimeout)
}

func TestParseRejeitaSchemaInvalido(t *testing.T) {
	// policy exige "default" e o root exige models+policy.
	_, err := config.Parse([]byte(`{"models":{},"policy":{}}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config")

	_, err = config.Parse([]byte(`nao é json`))
	require.Error(t, err)
}

const stdioJSON = `{
  "models": {},
  "policy": {"default": "deny"},
  "mcp_servers": [{
    "name": "local",
    "command": "/usr/bin/server",
    "args": ["--stdio"],
    "env": {"FOO": "bar"},
    "env_credentials": {"TOKEN": "vault:x"},
    "dir": "/tmp",
    "terminate_timeout_seconds": 10
  }]
}`

func TestParseMCPServerStdio(t *testing.T) {
	file, err := config.Parse([]byte(stdioJSON))
	require.NoError(t, err)

	servers := file.MCPServers()
	require.Len(t, servers, 1)
	s := servers[0]
	assert.Equal(t, "local", s.Name)
	assert.Equal(t, "/usr/bin/server", s.Command)
	assert.Equal(t, []string{"--stdio"}, s.Args)
	assert.Equal(t, map[string]string{"FOO": "bar"}, s.Env)
	assert.Equal(t, map[string]string{"TOKEN": "vault:x"}, s.EnvCredentials)
	assert.Equal(t, "/tmp", s.Dir)
	assert.Equal(t, 10*time.Second, s.TerminateTimeout)
	assert.Empty(t, s.Endpoint)
}
