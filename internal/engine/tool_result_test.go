package engine

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncateToolResult(t *testing.T) {
	t.Run("corta no teto e marca Truncated", func(t *testing.T) {
		res := ToolResult{Content: []ResultContent{{Kind: ResultText, Text: strings.Repeat("x", 100)}}}
		got := truncateToolResult(res, 10)
		assert.True(t, got.Truncated)
		require := got.Content
		assert.Len(t, require, 1)
		assert.Len(t, got.Content[0].Text, 10)
	})

	t.Run("abaixo do teto não altera", func(t *testing.T) {
		res := ToolResult{Content: []ResultContent{{Kind: ResultText, Text: "ok"}}}
		got := truncateToolResult(res, 10)
		assert.False(t, got.Truncated)
		assert.Equal(t, "ok", got.Content[0].Text)
	})

	t.Run("teto zero desliga", func(t *testing.T) {
		res := ToolResult{Content: []ResultContent{{Kind: ResultText, Text: strings.Repeat("x", 100)}}}
		assert.False(t, truncateToolResult(res, 0).Truncated)
	})
}
