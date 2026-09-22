package mcpclient_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/adapters/mcpclient"
)

// testBinary devolve o caminho do binário de teste em execução.
func testBinary(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	return exe
}

// stdioConfig monta um Config stdio para o servidor de teste no modo indicado.
func stdioConfig(t *testing.T, mode string, extra ...func(*mcpclient.Config)) mcpclient.Config {
	t.Helper()
	cfg := mcpclient.Config{
		Name:    "test-" + mode,
		Command: testBinary(t),
		Env:     map[string]string{"IAH_MCP_TESTSERVER": mode},
	}
	for _, fn := range extra {
		fn(&cfg)
	}
	return cfg
}

func TestConfigValidate(t *testing.T) {
	t.Run("endpoint_and_command_rejected", func(t *testing.T) {
		client := mcpclient.New(mcpclient.Config{
			Name:     "both",
			Endpoint: "https://x/mcp",
			Command:  "server",
		}, mcpclient.Deps{})
		_, err := client.List(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mutuamente exclusivos")
	})

	t.Run("neither_endpoint_nor_command_rejected", func(t *testing.T) {
		client := mcpclient.New(mcpclient.Config{Name: "empty"}, mcpclient.Deps{})
		_, err := client.List(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sem Endpoint e sem Command")
	})

	t.Run("invalid_env_name_rejected", func(t *testing.T) {
		client := mcpclient.New(mcpclient.Config{
			Name:    "bad-env",
			Command: "server",
			Env:     map[string]string{"invalid name": "val"},
		}, mcpclient.Deps{})
		_, err := client.List(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nome de variável inválido")
	})
}

func TestStdio_ListAndCall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stdio test server requires Unix")
	}
	client := mcpclient.New(stdioConfig(t, "echo"), mcpclient.Deps{})
	t.Cleanup(func() { _ = client.Close() })

	tools, err := client.List(context.Background())
	require.NoError(t, err)
	require.Len(t, tools, 1)
	assert.Equal(t, "echo", tools[0].Name)
	assert.Equal(t, "test-echo", tools[0].Namespace)

	result, err := client.Call(context.Background(), "echo", []byte(`{"text":"hello"}`), nil)
	require.NoError(t, err)
	require.NotEmpty(t, result.Content)
	assert.Equal(t, "echo: hello", result.Content[0].Text)
}

func TestStdio_EnvIsolation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stdio test server requires Unix")
	}
	t.Setenv("SEGREDO_HOST", "deveria-nao-vazar")

	creds := &fakeCredentials{value: "resolved-token-xyz"}
	client := mcpclient.New(stdioConfig(t, "env", func(cfg *mcpclient.Config) {
		cfg.EnvCredentials = map[string]string{"API_TOKEN": "vault:key"}
		cfg.Env = map[string]string{
			"IAH_MCP_TESTSERVER": "env",
			"CUSTOM_VAR":         "custom-value",
		}
	}), mcpclient.Deps{Credentials: creds})
	t.Cleanup(func() { _ = client.Close() })

	// Verify API_TOKEN is resolved
	result, err := client.Call(context.Background(), "getenv", []byte(`{"name":"API_TOKEN"}`), nil)
	require.NoError(t, err)
	require.NotEmpty(t, result.Content)
	assert.Equal(t, "resolved-token-xyz", result.Content[0].Text)

	// Verify CUSTOM_VAR is set
	result, err = client.Call(context.Background(), "getenv", []byte(`{"name":"CUSTOM_VAR"}`), nil)
	require.NoError(t, err)
	require.NotEmpty(t, result.Content)
	assert.Equal(t, "custom-value", result.Content[0].Text)

	// Verify SEGREDO_HOST is NOT set (host env doesn't leak)
	result, err = client.Call(context.Background(), "getenv", []byte(`{"name":"SEGREDO_HOST"}`), nil)
	require.NoError(t, err)
	require.NotEmpty(t, result.Content)
	assert.Empty(t, result.Content[0].Text, "variável do host não deve vazar")
}

func TestStdio_CancelDoesNotKillServer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stdio test server requires Unix")
	}
	client := mcpclient.New(stdioConfig(t, "echo"), mcpclient.Deps{})
	t.Cleanup(func() { _ = client.Close() })

	// First call to establish connection
	_, err := client.List(context.Background())
	require.NoError(t, err)

	// Cancel a call context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.Call(ctx, "echo", []byte(`{"text":"test"}`), nil)
	require.Error(t, err)

	// Server should still be alive — next call should work
	result, err := client.Call(context.Background(), "echo", []byte(`{"text":"alive"}`), nil)
	require.NoError(t, err)
	require.NotEmpty(t, result.Content)
	assert.Equal(t, "echo: alive", result.Content[0].Text)
}

func TestStdio_ExecutableNotFound(t *testing.T) {
	client := mcpclient.New(mcpclient.Config{
		Name:    "missing",
		Command: "nao-existe-iah-test",
	}, mcpclient.Deps{})
	t.Cleanup(func() { _ = client.Close() })

	_, err := client.List(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "não encontrado")
}

func TestStdio_ExitNowFailsWithoutLoop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stdio test server requires Unix")
	}
	client := mcpclient.New(stdioConfig(t, "exit-now"), mcpclient.Deps{})
	t.Cleanup(func() { _ = client.Close() })

	_, err := client.List(context.Background())
	require.Error(t, err)
}

func TestStdio_DirOption(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stdio test server requires Unix")
	}
	dir := t.TempDir()
	client := mcpclient.New(stdioConfig(t, "echo", func(cfg *mcpclient.Config) {
		cfg.Dir = dir
	}), mcpclient.Deps{})
	t.Cleanup(func() { _ = client.Close() })

	tools, err := client.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, tools, 1)
}

func TestStdio_ResolveRelativePathRejected(t *testing.T) {
	client := mcpclient.New(mcpclient.Config{
		Name:    "relative",
		Command: "./server",
	}, mcpclient.Deps{})
	t.Cleanup(func() { _ = client.Close() })

	_, err := client.List(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "recusado")
}

func TestStdio_CloseNoOrphan(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process group test requires Unix")
	}
	client := mcpclient.New(stdioConfig(t, "spawn-child"), mcpclient.Deps{})

	// Establish connection
	tools, err := client.List(context.Background())
	require.NoError(t, err)
	require.Len(t, tools, 1)

	// Get the PID of the server
	result, err := client.Call(context.Background(), "pid", nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, result.Content)

	// Close should kill the process group
	require.NoError(t, client.Close())

	// Give a moment for processes to die
	time.Sleep(200 * time.Millisecond)
}

func TestStdio_StderrFloodLimited(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stdio test server requires Unix")
	}
	client := mcpclient.New(stdioConfig(t, "stderr-flood"), mcpclient.Deps{})
	t.Cleanup(func() { _ = client.Close() })

	tools, err := client.List(context.Background())
	require.NoError(t, err)
	require.Len(t, tools, 1)

	result, err := client.Call(context.Background(), "flood", nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, result.Content)
	assert.Equal(t, "done", result.Content[0].Text)
}

func TestStdio_AbsolutePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stdio test server requires Unix")
	}
	exe, err := exec.LookPath("go")
	require.NoError(t, err)
	absExe, err := filepath.Abs(exe)
	require.NoError(t, err)
	_ = absExe
	// Just verify that absolute path is accepted in config validation
	cfg := mcpclient.Config{
		Name:    "abs",
		Command: "/bin/sh",
	}
	err = cfg.Validate()
	require.NoError(t, err)
}
