package mcpclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmarquespaixao/ia-harness/adapters/mcpclient"
	"rmarquespaixao/ia-harness/harness"
)

func TestCall_DevolveTextoDoServidor(t *testing.T) {
	// Arrange
	client := conectaMemoria(t, mcpclient.Config{}, registraEco)

	// Act
	result, err := client.Call(context.Background(), "eco", nil, nil)

	// Assert
	require.NoError(t, err)
	assert.False(t, result.IsError)
	require.Len(t, result.Content, 1)
	assert.Equal(t, harness.ResultText, result.Content[0].Kind)
	assert.Equal(t, "eco!", result.Content[0].Text)
}

func TestCall_ProgressoChegaAntesDoResultado(t *testing.T) {
	// Arrange: o servidor só confirma o resultado depois que o cliente
	// processou os dois progressos — o SDK entrega notificações de forma
	// assíncrona ao retorno da chamada, então a barreira torna a ordem
	// determinística.
	progresso := make(chan harness.ProgressUpdate, 4)
	processado := make(chan struct{})
	var recebidos atomic.Int32
	client := conectaMemoria(t, mcpclient.Config{}, func(server *mcp.Server) {
		mcp.AddTool(server, &mcp.Tool{Name: "longa", InputSchema: esquemaObjeto()},
			func(ctx context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
				token := req.Params.GetProgressToken()
				for passo := 1; passo <= 2; passo++ {
					err := req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
						ProgressToken: token,
						Message:       fmt.Sprintf("passo %d", passo),
						Progress:      float64(passo),
						Total:         2,
					})
					if err != nil {
						return nil, nil, err
					}
				}
				select {
				case <-processado:
				case <-ctx.Done():
					return nil, nil, ctx.Err()
				}
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "pronto"}}}, nil, nil
			})
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Act
	result, err := client.Call(ctx, "longa", nil, func(update harness.ProgressUpdate) {
		progresso <- update
		if recebidos.Add(1) == 2 {
			close(processado)
		}
	})

	// Assert
	require.NoError(t, err)
	require.Len(t, progresso, 2, "o progresso deve chegar antes do resultado")
	assert.Equal(t, harness.ProgressUpdate{Message: "passo 1", Progress: 1, Total: 2}, <-progresso)
	assert.Equal(t, harness.ProgressUpdate{Message: "passo 2", Progress: 2, Total: 2}, <-progresso)
	require.Len(t, result.Content, 1)
	assert.Equal(t, "pronto", result.Content[0].Text)
}

func TestCall_IsErrorDoServidorViraToolResultIsError(t *testing.T) {
	// Arrange
	client := conectaMemoria(t, mcpclient.Config{}, func(server *mcp.Server) {
		mcp.AddTool(server, &mcp.Tool{Name: "falha", InputSchema: esquemaObjeto()},
			func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, any, error) {
				return nil, nil, errors.New("saldo insuficiente")
			})
	})

	// Act
	result, err := client.Call(context.Background(), "falha", nil, nil)

	// Assert
	require.NoError(t, err)
	assert.True(t, result.IsError)
	require.NotEmpty(t, result.Content)
	assert.Contains(t, result.Content[0].Text, "saldo insuficiente")
}

func TestCall_ResultadoEstruturadoViraResultJSON(t *testing.T) {
	// Arrange
	type saidaContagem struct {
		Total int `json:"total"`
	}
	client := conectaMemoria(t, mcpclient.Config{}, func(server *mcp.Server) {
		mcp.AddTool(server, &mcp.Tool{Name: "contar", InputSchema: esquemaObjeto()},
			func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, saidaContagem, error) {
				return nil, saidaContagem{Total: 7}, nil
			})
	})

	// Act
	result, err := client.Call(context.Background(), "contar", nil, nil)

	// Assert
	require.NoError(t, err)
	var estruturado *harness.ResultContent
	for i := range result.Content {
		if result.Content[i].Kind == harness.ResultJSON {
			estruturado = &result.Content[i]
		}
	}
	require.NotNil(t, estruturado, "resultado deve conter o conteúdo estruturado")
	var decodificado saidaContagem
	require.NoError(t, json.Unmarshal(estruturado.JSON, &decodificado))
	assert.Equal(t, 7, decodificado.Total)
}

func TestCall_ToolDesconhecidaRetornaErroSemReconectar(t *testing.T) {
	// Arrange
	server := mcp.NewServer(implServidor(), nil)
	registraEco(server)
	dialer := &dialerMemoria{servidor: make(chan *mcp.InMemoryTransport, 8)}
	sup := novaSupervisora(server, dialer)
	client := mcpclient.New(mcpclient.Config{Name: "financeiro"}, mcpclient.Deps{Transport: dialer})
	t.Cleanup(func() { _ = client.Close() })

	_, err := client.List(context.Background())
	require.NoError(t, err)
	_ = sup.sessao(t)

	// Act
	_, err = client.Call(context.Background(), "nao_existe", nil, nil)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "nao_existe")
	assert.Never(t, func() bool { return len(sup.sessoes) > 0 }, 200*time.Millisecond, 20*time.Millisecond,
		"erro de protocolo não deve reconectar")
}

func TestCall_ArgsInvalidosNemConecta(t *testing.T) {
	// Arrange
	transport := &transporteQuebrado{}
	client := mcpclient.New(mcpclient.Config{Name: "financeiro"}, mcpclient.Deps{Transport: transport})
	t.Cleanup(func() { _ = client.Close() })

	// Act
	_, err := client.Call(context.Background(), "eco", json.RawMessage("{"), nil)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "argumentos")
	assert.Zero(t, transport.tentativas.Load())
}
