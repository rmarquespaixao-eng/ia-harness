package mcpclient_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// IAH_MCP_TESTSERVER é a variável que transforma o binário de teste em um
// servidor MCP stdio. O valor indica o modo do servidor.
const envTestServer = "IAH_MCP_TESTSERVER"

// TestMain intercepta a execução dos testes: se IAH_MCP_TESTSERVER está
// presente, o binário vira um servidor MCP stdio no modo indicado.
func TestMain(m *testing.M) {
	mode := os.Getenv(envTestServer)
	if mode != "" {
		os.Exit(runTestServer(mode))
	}
	os.Exit(m.Run())
}

// runTestServer inicia o servidor MCP no modo indicado e serve por stdio.
func runTestServer(mode string) int {
	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "v0.0.1"}, nil)
	registerTestTools(server, mode)
	transport := &mcp.StdioTransport{}
	session, err := server.Connect(context.Background(), transport, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "test-server: %v\n", err)
		return 1
	}
	// Bloqueia até o cliente fechar a conexão (stdin EOF).
	session.Wait()
	return 0
}

// registerTestTools registra as tools conforme o modo do servidor de teste.
func registerTestTools(server *mcp.Server, mode string) {
	switch mode {
	case "echo":
		mcp.AddTool(server,
			&mcp.Tool{Name: "echo", Description: "Devolve o texto enviado", InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{"type": "string"},
				},
			}},
			func(_ context.Context, _ *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
				text, _ := args["text"].(string)
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: "echo: " + text}},
				}, nil, nil
			})
	case "env":
		mcp.AddTool(server,
			&mcp.Tool{Name: "getenv", Description: "Devolve o valor de uma variável de ambiente", InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
				},
			}},
			func(_ context.Context, _ *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
				name, _ := args["name"].(string)
				val := os.Getenv(name)
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: val}},
				}, nil, nil
			})
	case "die-after-1":
		mcp.AddTool(server,
			&mcp.Tool{Name: "ping", Description: "Responde uma vez e sai", InputSchema: map[string]any{"type": "object"}},
			func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
				result := &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: "pong"}},
				}
				go func() {
					// Sai após devolver a resposta.
					defer os.Exit(0)
				}()
				return result, nil, nil
			})
	case "spawn-child":
		mcp.AddTool(server,
			&mcp.Tool{Name: "pid", Description: "Devolve o PID do servidor", InputSchema: map[string]any{"type": "object"}},
			func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("%d", os.Getpid())}},
				}, nil, nil
			})
	case "stderr-flood":
		mcp.AddTool(server,
			&mcp.Tool{Name: "flood", Description: "Escreve muito no stderr", InputSchema: map[string]any{"type": "object"}},
			func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
				for i := 0; i < 100; i++ {
					fmt.Fprintf(os.Stderr, "stderr line %d\n", i)
				}
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: "done"}},
				}, nil, nil
			})
	case "exit-now":
		// Sai antes do handshake.
		os.Exit(1)
	}
}
