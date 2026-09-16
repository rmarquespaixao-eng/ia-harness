package anthropic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/harness"
)

func TestUserBlocks_ImagemEDocumento(t *testing.T) {
	// Arrange
	m := harness.Message{Role: harness.RoleUser, Parts: []harness.Part{
		{Kind: harness.PartText, Text: "confira"},
		{Kind: harness.PartImage, Media: &harness.Media{MIME: "image/jpeg", SizeBytes: 1, Bytes: []byte("x")}},
		{Kind: harness.PartDocument, Media: &harness.Media{MIME: "application/pdf", Name: "f.pdf", SizeBytes: 1, Bytes: []byte("x")}},
	}}

	// Act
	blocks := userBlocks(m)

	// Assert
	require.Len(t, blocks, 3)
	assert.Equal(t, "text", blocks[0].Type)
	require.NotNil(t, blocks[1].Source)
	assert.Equal(t, "image", blocks[1].Type)
	assert.Equal(t, "base64", blocks[1].Source.Type)
	assert.Equal(t, "image/jpeg", blocks[1].Source.MediaType)
	require.NotNil(t, blocks[2].Source)
	assert.Equal(t, "document", blocks[2].Type)
	assert.Equal(t, "application/pdf", blocks[2].Source.MediaType)
}

func TestUserBlocks_ImagemPorURL(t *testing.T) {
	m := harness.Message{Role: harness.RoleUser, Parts: []harness.Part{
		{Kind: harness.PartImage, Media: &harness.Media{MIME: "image/png", SizeBytes: 1, Reference: "https://x/f.png"}},
	}}
	blocks := userBlocks(m)
	require.Len(t, blocks, 1)
	assert.Equal(t, "url", blocks[0].Source.Type)
	assert.Equal(t, "https://x/f.png", blocks[0].Source.URL)
}

func TestUserBlocks_TextualViraTexto(t *testing.T) {
	m := harness.Message{Role: harness.RoleUser, Parts: []harness.Part{
		{Kind: harness.PartDocument, Media: &harness.Media{MIME: "text/csv", SizeBytes: 3, Bytes: []byte("a,b")}},
	}}
	blocks := userBlocks(m)
	require.Len(t, blocks, 1)
	assert.Equal(t, "text", blocks[0].Type)
	assert.Equal(t, "a,b", blocks[0].Text)
}
