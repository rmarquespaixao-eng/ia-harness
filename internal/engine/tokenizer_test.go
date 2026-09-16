package engine

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// fakeTokenizer conta palavras (heurística de teste).
type fakeTokenizer struct{}

func (fakeTokenizer) Count(s string) int { return len(strings.Fields(s)) }

func TestTokenizeMessage_UsaTokenizerInjetada(t *testing.T) {
	// Arrange
	m := Message{Parts: []Part{{Kind: PartText, Text: "um dois três"}}}

	// Act
	got := tokenizeMessage(fakeTokenizer{}, m)

	// Assert
	assert.Equal(t, 3, got)
}

func TestEstimateWindowTokens_UsaTokenizerQuandoInjetada(t *testing.T) {
	// Arrange
	h := &Harness{cfg: Config{Tokenizer: fakeTokenizer{}}}
	messages := []Message{{Parts: []Part{{Kind: PartText, Text: "a b c d"}}}}

	// Act
	got := h.estimateWindowTokens(messages)

	// Assert
	assert.Equal(t, 4, got)
}
