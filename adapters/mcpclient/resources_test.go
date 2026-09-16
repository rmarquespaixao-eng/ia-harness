package mcpclient_test

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/adapters/mcpclient"
)

func registraRecursosEPrompts(server *mcp.Server) {
	server.AddResource(&mcp.Resource{URI: "mem://fatura", Name: "fatura", MIMEType: "text/plain"},
		func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{
				{URI: req.Params.URI, MIMEType: "text/plain", Text: "conteúdo da fatura"},
			}}, nil
		})
	server.AddPrompt(&mcp.Prompt{Name: "resumir", Arguments: []*mcp.PromptArgument{{Name: "texto", Required: true}}},
		func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return &mcp.GetPromptResult{Messages: []*mcp.PromptMessage{
				{Role: "user", Content: &mcp.TextContent{Text: "resuma: " + req.Params.Arguments["texto"]}},
			}}, nil
		})
}

func TestClient_ListaELeRecursosEPrompts(t *testing.T) {
	// Arrange
	client := conectaMemoria(t, mcpclient.Config{}, registraRecursosEPrompts)
	ctx := context.Background()

	// Act — recursos
	resources, err := client.ListResources(ctx)
	require.NoError(t, err)
	require.Len(t, resources, 1)
	assert.Equal(t, "mem://fatura", resources[0].URI)
	assert.Equal(t, "fatura", resources[0].Name)

	contents, err := client.ReadResource(ctx, "mem://fatura")
	require.NoError(t, err)
	require.Len(t, contents, 1)
	assert.Equal(t, "conteúdo da fatura", contents[0].Text)

	// Act — prompts
	prompts, err := client.ListPrompts(ctx)
	require.NoError(t, err)
	require.Len(t, prompts, 1)
	assert.Equal(t, "resumir", prompts[0].Name)
	require.Len(t, prompts[0].Arguments, 1)
	assert.True(t, prompts[0].Arguments[0].Required)

	messages, err := client.GetPrompt(ctx, "resumir", map[string]string{"texto": "oi"})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Equal(t, "user", messages[0].Role)
	assert.Equal(t, "resuma: oi", messages[0].Text)
}
