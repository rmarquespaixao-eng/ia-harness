package mcpclient_test

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmarquespaixao/ia-harness/adapters/mcpclient"
)

func TestList_MapeiaCatalogoComNamespaceESchema(t *testing.T) {
	// Arrange
	client := conectaMemoria(t, mcpclient.Config{Name: "financeiro", ToolTimeout: 3 * time.Second},
		func(server *mcp.Server) {
			mcp.AddTool(server, &mcp.Tool{
				Name:        "listar_contas",
				Description: "Lista as contas do usuário",
				InputSchema: esquemaObjeto(),
			}, func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, any, error) {
				return &mcp.CallToolResult{}, nil, nil
			})
		})

	// Act
	tools, err := client.List(context.Background())

	// Assert
	require.NoError(t, err)
	require.Len(t, tools, 1)
	tool := tools[0]
	assert.Equal(t, "listar_contas", tool.Name)
	assert.Equal(t, "financeiro", tool.Namespace)
	assert.Equal(t, "Lista as contas do usuário", tool.Description)
	assert.Equal(t, 3*time.Second, tool.Timeout)
	assert.False(t, tool.Destructive)
	assert.False(t, tool.Idempotent)

	var schema map[string]any
	require.NoError(t, json.Unmarshal(tool.InputSchema, &schema))
	assert.Equal(t, "object", schema["type"])
}

func TestList_UsaCachePorSessao(t *testing.T) {
	// Arrange
	var listagens atomic.Int32
	client := conectaMemoria(t, mcpclient.Config{}, func(server *mcp.Server) {
		server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
			return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
				if method == "tools/list" {
					listagens.Add(1)
				}
				return next(ctx, method, req)
			}
		})
		registraEco(server)
	})

	// Act
	primeiro, err := client.List(context.Background())
	require.NoError(t, err)
	segundo, err := client.List(context.Background())

	// Assert
	require.NoError(t, err)
	assert.Equal(t, primeiro, segundo)
	assert.EqualValues(t, 1, listagens.Load(), "a segunda listagem deve vir do cache da sessão")
}

func TestList_CacheInvalidadoNaReconexao(t *testing.T) {
	// Arrange
	server := mcp.NewServer(implServidor(), nil)
	registraEco(server)
	dialer := &dialerMemoria{servidor: make(chan *mcp.InMemoryTransport, 8)}
	sup := novaSupervisora(server, dialer)
	client := mcpclient.New(mcpclient.Config{Name: "financeiro"}, mcpclient.Deps{Transport: dialer})
	t.Cleanup(func() { _ = client.Close() })

	primeiro, err := client.List(context.Background())
	require.NoError(t, err)
	require.Len(t, primeiro, 1)

	primeira := sup.sessao(t)
	require.NoError(t, primeira.Close())

	// Act: a chamada falha na sessão morta, reconecta e invalida o cache.
	_, err = client.Call(context.Background(), "eco", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, sup.sessao(t))

	mcp.AddTool(server, &mcp.Tool{Name: "nova", InputSchema: esquemaObjeto()},
		func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{}, nil, nil
		})
	segundo, err := client.List(context.Background())

	// Assert
	require.NoError(t, err)
	assert.Len(t, segundo, 2, "catálogo deve ser redescoberto após a reconexão")
}
