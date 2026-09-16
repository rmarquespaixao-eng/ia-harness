package anthropic

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/harness"
)

func TestBuildBody_PromptCachingMarcaSystemETools(t *testing.T) {
	// Arrange
	prov := New(Config{BaseURL: "http://exemplo"}, Deps{})
	req := harness.ChatRequest{
		Model:         "claude-x",
		System:        "instruções",
		PromptCaching: true,
		Tools:         []harness.Tool{{Name: "t", InputSchema: json.RawMessage(`{"type":"object"}`)}},
	}

	// Act
	raw, err := prov.buildBody("claude-x", req)

	// Assert
	require.NoError(t, err)
	var body struct {
		System []struct {
			Type         string `json:"type"`
			CacheControl *struct {
				Type string `json:"type"`
			} `json:"cache_control"`
		} `json:"system"`
		Tools []struct {
			CacheControl *struct {
				Type string `json:"type"`
			} `json:"cache_control"`
		} `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(raw, &body))
	require.Len(t, body.System, 1)
	require.NotNil(t, body.System[0].CacheControl)
	assert.Equal(t, "ephemeral", body.System[0].CacheControl.Type)
	require.Len(t, body.Tools, 1)
	require.NotNil(t, body.Tools[0].CacheControl, "último tool é o breakpoint")
}

func TestBuildBody_SemCacheSystemContinuaString(t *testing.T) {
	prov := New(Config{BaseURL: "http://exemplo"}, Deps{})
	raw, err := prov.buildBody("m", harness.ChatRequest{Model: "m", System: "sys"})
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"system":"sys"`)
}

func TestParseStream_UsageComCache(t *testing.T) {
	// Arrange — message_start com tokens de cache.
	const sse = "event: message_start\n" +
		"data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":100,\"cache_read_input_tokens\":900,\"cache_creation_input_tokens\":50}}}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"

	// Act
	resp, err := parseStream(context.Background(), strings.NewReader(sse), "m", nil)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, int64(1050), resp.Usage.InputTokens, "input total = não cacheado + leitura + escrita")
	assert.Equal(t, int64(900), resp.Usage.CachedInputTokens)
	assert.Equal(t, int64(50), resp.Usage.CacheWriteTokens)
	assert.False(t, resp.Usage.Estimated)
}
