package openai

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStream_UsageComCache(t *testing.T) {
	// Arrange — chunk final com prompt_tokens_details.cached_tokens.
	const sse = "data: {\"choices\":[{\"delta\":{\"content\":\"oi\"},\"finish_reason\":\"stop\"}]}\n" +
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":5,\"prompt_tokens_details\":{\"cached_tokens\":80}}}\n" +
		"data: [DONE]\n\n"

	// Act
	resp, err := parseStream(context.Background(), strings.NewReader(sse), "m", nil)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, int64(100), resp.Usage.InputTokens)
	assert.Equal(t, int64(80), resp.Usage.CachedInputTokens)
	assert.Equal(t, int64(0), resp.Usage.CacheWriteTokens)
}
