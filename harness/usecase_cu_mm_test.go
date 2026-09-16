package harness_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmarquespaixao/ia-harness/adapters/session/memory"
	"rmarquespaixao/ia-harness/harness"
	"rmarquespaixao/ia-harness/internal/testutil"
)

// newMMEnv monta um harness mínimo com as capacidades informadas, para testar o
// gate multimodal (feature 002).
func newMMEnv(t *testing.T, caps harness.Capabilities) (*harness.Harness, *testutil.ScriptedProvider) {
	t.Helper()
	provider := &testutil.ScriptedProvider{Steps: []testutil.ProviderStep{{Text: "ok"}}}
	cfg := harness.Config{
		Providers: map[string]harness.Provider{turnProviderKey: provider},
		Models: map[string]harness.ModelProfile{
			turnModelAlias: {Provider: turnProviderKey, Model: "fake-1", Capabilities: caps},
		},
		Credentials:  &testutil.FakeCredentialProvider{Values: map[string]string{}},
		Sessions:     memory.New(),
		Logger:       slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
		DefaultModel: turnModelAlias,
		Policy:       harness.PolicyConfig{Default: harness.PolicyAllow},
	}
	h, err := harness.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	return h, provider
}

// mmPergunta monta o RunRequest com as partes informadas.
func mmPergunta(parts ...harness.Part) harness.RunRequest {
	return harness.RunRequest{UserID: turnUserID, AgentID: turnAgentID, Model: turnModelAlias, Input: parts}
}

// imagemPNG monta uma Part de imagem inline válida.
func imagemPNG() harness.Part {
	return harness.Part{Kind: harness.PartImage, Media: &harness.Media{
		MIME: "image/png", Name: "f.png", SizeBytes: 4, Bytes: []byte("PNG!"),
	}}
}

func TestCU_MM_ImagemComVisaoChegaAoProvider(t *testing.T) {
	// Dado um perfil com visão.
	h, prov := newMMEnv(t, harness.Capabilities{ToolCalling: true, Streaming: true, Vision: true})

	// Quando o usuário anexa uma imagem.
	_, err := h.Run(context.Background(), mmPergunta(imagemPNG()), nil)

	// Então o turno conclui e o provider recebeu a mídia.
	require.NoError(t, err)
	require.Len(t, prov.Requests, 1)
	require.NotEmpty(t, prov.Requests[0].Messages)
	last := prov.Requests[0].Messages[len(prov.Requests[0].Messages)-1]
	require.Len(t, last.Parts, 1)
	require.NotNil(t, last.Parts[0].Media)
	assert.Equal(t, "image/png", last.Parts[0].Media.MIME)
}

func TestCU_MM_ImagemSemVisaoFalhaSemChamarProvider(t *testing.T) {
	// Dado um perfil sem visão.
	h, prov := newMMEnv(t, harness.Capabilities{ToolCalling: true})

	// Quando o usuário anexa uma imagem.
	_, err := h.Run(context.Background(), mmPergunta(imagemPNG()), nil)

	// Então erro nomeado e nenhuma chamada ao provedor.
	var cfgErr *harness.ConfigError
	require.ErrorAs(t, err, &cfgErr)
	assert.Equal(t, "media/visao-nao-suportada", cfgErr.Code)
	assert.Zero(t, prov.Calls)
}

func TestCU_MM_MIMEForaDaAllowlist(t *testing.T) {
	// Dado um perfil com visão.
	h, _ := newMMEnv(t, harness.Capabilities{ToolCalling: true, Vision: true})

	// Quando o MIME não é permitido.
	part := harness.Part{Kind: harness.PartImage, Media: &harness.Media{
		MIME: "image/tiff", SizeBytes: 4, Bytes: []byte("TIF!"),
	}}
	_, err := h.Run(context.Background(), mmPergunta(part), nil)

	// Então erro de MIME.
	var cfgErr *harness.ConfigError
	require.ErrorAs(t, err, &cfgErr)
	assert.Equal(t, "media/mime-nao-suportado", cfgErr.Code)
}

func TestCU_MM_TetoDeTamanho(t *testing.T) {
	// Dado um perfil com teto de 2 bytes.
	h, _ := newMMEnv(t, harness.Capabilities{ToolCalling: true, Vision: true, MaxMediaBytes: 2})

	// Quando a mídia excede o teto.
	_, err := h.Run(context.Background(), mmPergunta(imagemPNG()), nil)

	// Então erro de tamanho.
	var cfgErr *harness.ConfigError
	require.ErrorAs(t, err, &cfgErr)
	assert.Equal(t, "media/grande", cfgErr.Code)
}

func TestCU_MM_FonteDuplaEhInvalida(t *testing.T) {
	// Dado um perfil com visão.
	h, _ := newMMEnv(t, harness.Capabilities{ToolCalling: true, Vision: true})

	// Quando a mídia traz bytes E reference.
	part := harness.Part{Kind: harness.PartImage, Media: &harness.Media{
		MIME: "image/png", SizeBytes: 4, Bytes: []byte("PNG!"), Reference: "https://x/f.png",
	}}
	_, err := h.Run(context.Background(), mmPergunta(part), nil)

	// Então erro de fonte.
	var cfgErr *harness.ConfigError
	require.ErrorAs(t, err, &cfgErr)
	assert.Equal(t, "media/fonte-invalida", cfgErr.Code)
}

func TestCU_MM_PDFSemDocumentsFalha_TextualPassa(t *testing.T) {
	// Dado um perfil sem suporte a documento binário.
	h, _ := newMMEnv(t, harness.Capabilities{ToolCalling: true})
	pdf := harness.Part{Kind: harness.PartDocument, Media: &harness.Media{
		MIME: "application/pdf", Name: "f.pdf", SizeBytes: 4, Bytes: []byte("%PDF"),
	}}

	// Quando anexa PDF → erro.
	_, err := h.Run(context.Background(), mmPergunta(pdf), nil)
	var cfgErr *harness.ConfigError
	require.ErrorAs(t, err, &cfgErr)
	assert.Equal(t, "media/documento-nao-suportado", cfgErr.Code)

	// E um CSV textual passa mesmo sem `documents`.
	csv := harness.Part{Kind: harness.PartDocument, Media: &harness.Media{
		MIME: "text/csv", Name: "d.csv", SizeBytes: 3, Bytes: []byte("a,b"),
	}}
	_, err = h.Run(context.Background(), mmPergunta(csv), nil)
	require.NoError(t, err)
}
