package openai_responses

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStream_UsageComCache(t *testing.T) {
	// Arrange — response.completed com input_tokens_details.cached_tokens.
	const sse = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"oi\"}\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":100,\"output_tokens\":5,\"input_tokens_details\":{\"cached_tokens\":70}}}}\n\n"

	// Act
	resp, err := parseStream(context.Background(), strings.NewReader(sse), "m", nil)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, int64(100), resp.Usage.InputTokens)
	assert.Equal(t, int64(70), resp.Usage.CachedInputTokens)
}
