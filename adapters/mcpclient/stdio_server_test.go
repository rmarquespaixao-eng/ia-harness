package mcpclient_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// IAH_MCP_TESTSERVER é a variável que transforma o binário de teste em um
// servidor MCP stdio. O valor indica o modo do servidor.
const envTestServer = "IAH_MCP_TESTSERVER"

// envStartLog é o caminho de um arquivo onde o servidor grava uma linha por
// início de processo (usado pelos testes para contar inícios).
const envStartLog = "IAH_MCP_STARTLOG"

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
	if mode == "sleep-forever" {
		select {}
	}

	recordStart()

	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "v0.0.1"}, nil)
	registerTestTools(server, mode)

	var transport mcp.Transport = &mcp.StdioTransport{}
	if mode == "die-after-1" {
		transport = &mcp.IOTransport{
			Reader: os.Stdin,
			Writer: &dieAfterOneWriter{out: os.Stdout},
		}
	}

	session, err := server.Connect(context.Background(), transport, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "test-server: %v\n", err)
		return 1
	}
	// Bloqueia até o cliente fechar a conexão (stdin EOF).
	session.Wait()
	return 0
}

// recordStart grava uma linha no arquivo indicado por envStartLog, se houver.
func recordStart() {
	path := os.Getenv(envStartLog)
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, "start")
}

// pidTool devolve o PID do servidor.
func pidTool(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("%d", os.Getpid())}},
	}, nil, nil
}

// dieAfterOneWriter envolve a saída do servidor e encerra o processo depois de
// escrever a resposta da tool "ping" (marcador "pong"), garantindo que o
// servidor responde e só então morre — sem goroutine imediata nem sleep fixo.
type dieAfterOneWriter struct {
	out io.Writer
}

func (w *dieAfterOneWriter) Write(p []byte) (int, error) {
	n, err := w.out.Write(p)
	if bytes.Contains(p, []byte(`"pong"`)) {
		os.Exit(0)
	}
	return n, err
}

func (w *dieAfterOneWriter) Close() error { return nil }

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
		mcp.AddTool(server,
			&mcp.Tool{Name: "pid", Description: "Devolve o PID do servidor", InputSchema: map[string]any{"type": "object"}},
			pidTool)
	case "block":
		mcp.AddTool(server,
			&mcp.Tool{Name: "block", Description: "Bloqueia até o contexto da chamada cancelar", InputSchema: map[string]any{"type": "object"}},
			func(ctx context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
				fmt.Fprintln(os.Stderr, "STDIOBLOCKING")
				<-ctx.Done()
				return nil, nil, ctx.Err()
			})
		mcp.AddTool(server,
			&mcp.Tool{Name: "pid", Description: "Devolve o PID do servidor", InputSchema: map[string]any{"type": "object"}},
			pidTool)
	case "die-after-1":
		mcp.AddTool(server,
			&mcp.Tool{Name: "ping", Description: "Responde uma vez e sai", InputSchema: map[string]any{"type": "object"}},
			func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: "pong"}},
				}, nil, nil
			})
	case "spawn-child":
		grandchild := spawnGrandchild()
		mcp.AddTool(server,
			&mcp.Tool{Name: "pids", Description: "Devolve o PID do servidor e do neto", InputSchema: map[string]any{"type": "object"}},
			func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("%d %d", os.Getpid(), grandchild)}},
				}, nil, nil
			})
	case "stderr-flood":
		mcp.AddTool(server,
			&mcp.Tool{Name: "flood", Description: "Escreve muito no stderr", InputSchema: map[string]any{"type": "object"}},
			func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
				for i := 0; i < 10; i++ {
					fmt.Fprintf(os.Stderr, "STDERRFLOOD-LONG %s\n", strings.Repeat("L", 8192))
				}
				for i := 0; i < 300; i++ {
					fmt.Fprintf(os.Stderr, "STDERRFLOOD %04d %s\n", i, strings.Repeat("s", 512))
				}
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: "done"}},
				}, nil, nil
			})
	case "exit-now":
		// Sai antes do handshake (após registrar o início).
		os.Exit(1)
	}
}

// spawnGrandchild re-executa o binário de teste em modo sleep-forever e devolve
// o PID do neto, que herda o grupo de processos do servidor.
func spawnGrandchild() int {
	exe, err := os.Executable()
	if err != nil {
		return -1
	}
	cmd := exec.Command(exe)
	cmd.Env = []string{envTestServer + "=sleep-forever"}
	if err := cmd.Start(); err != nil {
		return -1
	}
	return cmd.Process.Pid
}
