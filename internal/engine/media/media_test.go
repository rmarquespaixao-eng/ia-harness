package media

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	core "rmarquespaixao/ia-harness/internal/core"
)

func partImage(mime string, size int64) core.Part {
	return core.Part{Kind: core.PartImage, Media: &core.Media{MIME: mime, SizeBytes: size, Bytes: []byte("x")}}
}

func partDoc(mime string, size int64) core.Part {
	return core.Part{Kind: core.PartDocument, Media: &core.Media{MIME: mime, SizeBytes: size, Bytes: []byte("x")}}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		caps    core.Capabilities
		parts   []core.Part
		wantErr string
	}{
		{name: "imagem com visão", caps: core.Capabilities{Vision: true}, parts: []core.Part{partImage("image/png", 10)}},
		{name: "imagem sem visão", caps: core.Capabilities{}, parts: []core.Part{partImage("image/png", 10)}, wantErr: "media/visao-nao-suportada"},
		{name: "MIME não permitido", caps: core.Capabilities{Vision: true}, parts: []core.Part{partImage("image/tiff", 10)}, wantErr: "media/mime-nao-suportado"},
		{name: "tamanho zero", caps: core.Capabilities{Vision: true}, parts: []core.Part{partImage("image/png", 0)}, wantErr: "media/tamanho-invalido"},
		{name: "acima do teto do perfil", caps: core.Capabilities{Vision: true, MaxMediaBytes: 5}, parts: []core.Part{partImage("image/png", 10)}, wantErr: "media/grande"},
		{name: "PDF com documents", caps: core.Capabilities{Documents: true}, parts: []core.Part{partDoc("application/pdf", 10)}},
		{name: "PDF sem documents", caps: core.Capabilities{}, parts: []core.Part{partDoc("application/pdf", 10)}, wantErr: "media/documento-nao-suportado"},
		{name: "CSV textual dispensa documents", caps: core.Capabilities{}, parts: []core.Part{partDoc("text/csv", 10)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.caps, tt.parts)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			var cfgErr *core.ConfigError
			require.ErrorAs(t, err, &cfgErr)
			assert.Equal(t, tt.wantErr, cfgErr.Code)
		})
	}
}

func TestValidate_FonteExclusiva(t *testing.T) {
	// dupla fonte
	dupla := core.Part{Kind: core.PartImage, Media: &core.Media{MIME: "image/png", SizeBytes: 1, Bytes: []byte("x"), Reference: "https://x"}}
	var cfgErr *core.ConfigError
	require.ErrorAs(t, Validate(core.Capabilities{Vision: true}, []core.Part{dupla}), &cfgErr)
	assert.Equal(t, "media/fonte-invalida", cfgErr.Code)

	// somente reference é válido
	ref := core.Part{Kind: core.PartImage, Media: &core.Media{MIME: "image/png", SizeBytes: 1, Reference: "https://x/f.png"}}
	require.NoError(t, Validate(core.Capabilities{Vision: true}, []core.Part{ref}))
}

func TestSupportsERequired(t *testing.T) {
	vision, documents := Required([]core.Part{partImage("image/png", 1), partDoc("application/pdf", 1)})
	assert.True(t, vision)
	assert.True(t, documents)

	assert.True(t, Supports(core.Capabilities{Vision: true, Documents: true}, []core.Part{partImage("image/png", 1)}))
	assert.False(t, Supports(core.Capabilities{}, []core.Part{partImage("image/png", 1)}))
	assert.True(t, Supports(core.Capabilities{}, []core.Part{partDoc("text/csv", 1)}), "textual dispensa documents")
}
