package openai_responses

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmarquespaixao/ia-harness/harness"
)

func TestResponsesUserContent_ImagemEArquivo(t *testing.T) {
	// Arrange
	m := harness.Message{Role: harness.RoleUser, Parts: []harness.Part{
		{Kind: harness.PartText, Text: "veja"},
		{Kind: harness.PartImage, Media: &harness.Media{MIME: "image/png", SizeBytes: 4, Bytes: []byte("PNG!")}},
		{Kind: harness.PartDocument, Media: &harness.Media{MIME: "application/pdf", Name: "f.pdf", SizeBytes: 4, Bytes: []byte("%PDF")}},
	}}

	// Act
	content := responsesUserContent(m)

	// Assert
	arr, ok := content.([]any)
	require.True(t, ok)
	require.Len(t, arr, 3)
	assert.Equal(t, "input_text", arr[0].(map[string]any)["type"])
	img := arr[1].(map[string]any)
	assert.Equal(t, "input_image", img["type"])
	assert.True(t, strings.HasPrefix(img["image_url"].(string), "data:image/png;base64,"))
	file := arr[2].(map[string]any)
	assert.Equal(t, "input_file", file["type"])
	assert.Equal(t, "f.pdf", file["filename"])
}

func TestResponsesUserContent_TextualInline(t *testing.T) {
	m := harness.Message{Role: harness.RoleUser, Parts: []harness.Part{
		{Kind: harness.PartDocument, Media: &harness.Media{MIME: "text/plain", SizeBytes: 2, Bytes: []byte("oi")}},
	}}
	content := responsesUserContent(m)
	arr, ok := content.([]any)
	require.True(t, ok)
	require.Len(t, arr, 1)
	assert.Equal(t, "input_text", arr[0].(map[string]any)["type"])
	assert.Equal(t, "oi", arr[0].(map[string]any)["text"])
}
