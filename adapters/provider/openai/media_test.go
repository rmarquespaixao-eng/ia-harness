package openai

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/harness"
)

func TestUserContent_ImagemComoDataURI(t *testing.T) {
	// Arrange
	m := harness.Message{Role: harness.RoleUser, Parts: []harness.Part{
		{Kind: harness.PartText, Text: "olha isso"},
		{Kind: harness.PartImage, Media: &harness.Media{MIME: "image/png", Name: "f.png", SizeBytes: 4, Bytes: []byte("PNG!")}},
	}}

	// Act
	content := userContent(m)

	// Assert
	arr, ok := content.([]any)
	require.True(t, ok, "com mídia o content vira array")
	require.Len(t, arr, 2)
	assert.Equal(t, "text", arr[0].(map[string]any)["type"])
	img := arr[1].(map[string]any)
	assert.Equal(t, "image_url", img["type"])
	url := img["image_url"].(map[string]any)["url"].(string)
	assert.True(t, strings.HasPrefix(url, "data:image/png;base64,"))
}

func TestUserContent_SomenteTextoContinuaString(t *testing.T) {
	m := harness.Message{Role: harness.RoleUser, Parts: []harness.Part{{Kind: harness.PartText, Text: "oi"}}}
	assert.Equal(t, "oi", userContent(m))
}

func TestDocumentContent_TextualInlineEBinarioFile(t *testing.T) {
	// textual → bloco text
	textual := documentContent(&harness.Media{MIME: "text/csv", Name: "d.csv", Bytes: []byte("a,b")})
	assert.Equal(t, "text", textual["type"])
	assert.Equal(t, "a,b", textual["text"])

	// binário → bloco file com data URI
	bin := documentContent(&harness.Media{MIME: "application/pdf", Name: "f.pdf", Bytes: []byte("%PDF")})
	assert.Equal(t, "file", bin["type"])
	file := bin["file"].(map[string]any)
	assert.Equal(t, "f.pdf", file["filename"])
	assert.True(t, strings.HasPrefix(file["file_data"].(string), "data:application/pdf;base64,"))
}
