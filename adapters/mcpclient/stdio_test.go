package mcpclient_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/adapters/mcpclient"
)

// testBinary devolve o caminho do binário de teste em execução (caminho
// absoluto real, via os.Executable — usado por todos os testes stdio que
// iniciam o servidor de teste, cobrindo o caso de caminho absoluto de verdade).
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

// syncBuffer é um buffer seguro para concorrência, usado como destino de
// *slog.Logger capturado nos testes (o SDK lê stderr do filho em goroutine
// própria, concorrente com o corpo do teste).
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// bufferLogger devolve um *slog.Logger em nível Debug gravando em um
// syncBuffer, e o próprio buffer para inspeção pelo teste.
func bufferLogger() (*slog.Logger, *syncBuffer) {
	buf := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return logger, buf
}

// startCount lê o arquivo de log de início (envStartLog) e devolve o número de
// vezes que o servidor de teste iniciou; arquivo ausente conta como zero.
func startCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	require.NoError(t, err)
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return 0
	}
	return len(strings.Split(trimmed, "\n"))
}

// withStartLog acrescenta a variável IAH_MCP_STARTLOG ao Env do Config,
// preservando o modo do servidor.
func withStartLog(mode, path string) func(*mcpclient.Config) {
	return func(cfg *mcpclient.Config) {
		env := map[string]string{"IAH_MCP_TESTSERVER": mode, envStartLog: path}
		for k, v := range cfg.Env {
			if k == "IAH_MCP_TESTSERVER" {
				continue
			}
			env[k] = v
		}
		cfg.Env = env
	}
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

// TestStdio_CredentialSecrecy cobre FR-STD-004/005/013: a isca resolvida por
// EnvCredentials nunca aparece em /proc/<pid>/cmdline (quando existir), no log
// capturado nem em mensagem de erro; credencial não resolvida MUST NOT iniciar
// o processo e MUST gerar um erro nomeado sem o valor (T2315).
func TestStdio_CredentialSecrecy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stdio test server requires Unix")
	}
	const isca = "isca-XYZ-123"

	t.Run("isca_absent_from_cmdline_and_log", func(t *testing.T) {
		logger, buf := bufferLogger()
		creds := &fakeCredentials{value: isca}
		client := mcpclient.New(stdioConfig(t, "env", func(cfg *mcpclient.Config) {
			cfg.EnvCredentials = map[string]string{"API_TOKEN": "vault:key"}
		}), mcpclient.Deps{Credentials: creds, Logger: logger})
		t.Cleanup(func() { _ = client.Close() })

		result, err := client.Call(context.Background(), "getenv", []byte(`{"name":"API_TOKEN"}`), nil)
		require.NoError(t, err)
		require.NotEmpty(t, result.Content)
		assert.Equal(t, isca, result.Content[0].Text, "a credencial deve chegar ao ambiente do filho")

		pidResult, err := client.Call(context.Background(), "pid", nil, nil)
		require.NoError(t, err)
		require.NotEmpty(t, pidResult.Content)
		pid, err := strconv.Atoi(pidResult.Content[0].Text)
		require.NoError(t, err)

		cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err == nil {
			assert.NotContains(t, string(cmdline), isca, "a isca não deve aparecer no argv do processo")
		}

		assert.NotContains(t, buf.String(), isca, "a isca não deve aparecer no log capturado")
	})

	t.Run("unresolved_reference_no_start_named_error", func(t *testing.T) {
		startLog := filepath.Join(t.TempDir(), "starts.log")
		creds := &fakeCredentials{value: isca, err: errors.New("credencial inexistente no cofre")}
		client := mcpclient.New(stdioConfig(t, "env", withStartLog("env", startLog), func(cfg *mcpclient.Config) {
			cfg.EnvCredentials = map[string]string{"API_TOKEN": "vault:key"}
		}), mcpclient.Deps{Credentials: creds})
		t.Cleanup(func() { _ = client.Close() })

		_, err := client.List(context.Background())
		require.Error(t, err)
		assert.True(t, errors.Is(err, mcpclient.ErrCredential), "erro deve ser ErrCredential: %v", err)
		assert.Contains(t, err.Error(), "API_TOKEN", "erro deve citar a variável")
		assert.Contains(t, err.Error(), "test-env", "erro deve citar o servidor")
		assert.NotContains(t, err.Error(), isca, "erro não deve conter o valor da credencial")
		assert.Equal(t, 0, startCount(t, startLog), "processo não deve iniciar")
	})

	t.Run("nil_credential_provider_no_start_named_error", func(t *testing.T) {
		startLog := filepath.Join(t.TempDir(), "starts.log")
		client := mcpclient.New(stdioConfig(t, "env", withStartLog("env", startLog), func(cfg *mcpclient.Config) {
			cfg.EnvCredentials = map[string]string{"API_TOKEN": "vault:key"}
		}), mcpclient.Deps{})
		t.Cleanup(func() { _ = client.Close() })

		_, err := client.List(context.Background())
		require.Error(t, err)
		assert.True(t, errors.Is(err, mcpclient.ErrCredential), "erro deve ser ErrCredential: %v", err)
		assert.Contains(t, err.Error(), "API_TOKEN")
		assert.Equal(t, 0, startCount(t, startLog), "processo não deve iniciar")
	})
}

// TestStdio_CancelDuringCall cobre FR-STD-006: cancelar o ctx de uma Call
// enquanto ela está em andamento não mata o servidor; a chamada seguinte usa o
// mesmo PID (T2317).
func TestStdio_CancelDuringCall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stdio test server requires Unix")
	}
	logger, buf := bufferLogger()
	client := mcpclient.New(stdioConfig(t, "block"), mcpclient.Deps{Logger: logger})
	t.Cleanup(func() { _ = client.Close() })

	_, err := client.List(context.Background())
	require.NoError(t, err)

	before, err := client.Call(context.Background(), "pid", nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, before.Content)

	ctx, cancel := context.WithCancel(context.Background())
	callErrCh := make(chan error, 1)
	go func() {
		_, callErr := client.Call(ctx, "block", nil, nil)
		callErrCh <- callErr
	}()

	// Aguarda a chamada estar de fato em andamento no servidor (marcador de
	// stderr), sem sleep fixo.
	require.Eventually(t, func() bool {
		return strings.Contains(buf.String(), "STDIOBLOCKING")
	}, 5*time.Second, 10*time.Millisecond, "chamada bloqueante deve ter iniciado no servidor")

	cancel()
	select {
	case err := <-callErrCh:
		require.Error(t, err, "chamada cancelada deve devolver erro")
	case <-time.After(5 * time.Second):
		t.Fatal("chamada não retornou após cancelamento")
	}

	after, err := client.Call(context.Background(), "pid", nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, after.Content)
	assert.Equal(t, before.Content[0].Text, after.Content[0].Text, "o servidor deve continuar sendo o mesmo processo")
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

// TestStdio_ReconnectionStartCounts cobre FR-STD-008 (CU-STD-2): die-after-1
// reconecta uma única vez e a segunda chamada conclui com exatamente dois
// inícios; exit-now falha sem laço (no máximo dois inícios); executável
// ausente não inicia nenhum processo (T2318).
func TestStdio_ReconnectionStartCounts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stdio test server requires Unix")
	}

	t.Run("die_after_1_second_call_completes_with_two_starts", func(t *testing.T) {
		startLog := filepath.Join(t.TempDir(), "starts.log")
		client := mcpclient.New(stdioConfig(t, "die-after-1", withStartLog("die-after-1", startLog)), mcpclient.Deps{})
		t.Cleanup(func() { _ = client.Close() })

		_, err := client.List(context.Background())
		require.NoError(t, err)

		result1, err := client.Call(context.Background(), "ping", nil, nil)
		require.NoError(t, err)
		require.NotEmpty(t, result1.Content)
		assert.Equal(t, "pong", result1.Content[0].Text)

		result2, err := client.Call(context.Background(), "ping", nil, nil)
		require.NoError(t, err, "a segunda chamada deve reconectar e concluir")
		require.NotEmpty(t, result2.Content)
		assert.Equal(t, "pong", result2.Content[0].Text)

		assert.Equal(t, 2, startCount(t, startLog), "exatamente dois inícios: conexão inicial + reconexão")
	})

	t.Run("exit_now_fails_with_at_most_two_starts", func(t *testing.T) {
		startLog := filepath.Join(t.TempDir(), "starts.log")
		client := mcpclient.New(stdioConfig(t, "exit-now", withStartLog("exit-now", startLog)), mcpclient.Deps{})
		t.Cleanup(func() { _ = client.Close() })

		_, err := client.List(context.Background())
		require.Error(t, err)

		count := startCount(t, startLog)
		assert.GreaterOrEqual(t, count, 1)
		assert.LessOrEqual(t, count, 2, "não deve haver laço de reconexão")
	})

	t.Run("missing_executable_zero_starts", func(t *testing.T) {
		startLog := filepath.Join(t.TempDir(), "starts.log")
		client := mcpclient.New(mcpclient.Config{
			Name:    "missing",
			Command: "nao-existe-iah-test-2318",
			Env:     map[string]string{envStartLog: startLog},
		}, mcpclient.Deps{})
		t.Cleanup(func() { _ = client.Close() })

		_, err := client.List(context.Background())
		require.Error(t, err)
		assert.Equal(t, 0, startCount(t, startLog), "executável ausente não deve iniciar processo algum")
	})
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

// TestStdio_StderrFloodLimited cobre FR-STD-009: as linhas do stderr do filho
// só chegam ao logger em nível Debug, permanecem dentro do teto por linha e
// por total, e nada vaza para o stdout/stderr do próprio processo de teste
// (T2321).
func TestStdio_StderrFloodLimited(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stdio test server requires Unix")
	}
	logger, buf := bufferLogger()
	client := mcpclient.New(stdioConfig(t, "stderr-flood"), mcpclient.Deps{Logger: logger})
	t.Cleanup(func() { _ = client.Close() })

	tools, err := client.List(context.Background())
	require.NoError(t, err)
	require.Len(t, tools, 1)

	result, err := client.Call(context.Background(), "flood", nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, result.Content)
	assert.Equal(t, "done", result.Content[0].Text)

	// O envio ao logger é assíncrono (goroutine de leitura do stderr do SDK);
	// aguarda até o total truncado aparecer, sem sleep fixo.
	require.Eventually(t, func() bool {
		return strings.Contains(buf.String(), "[TOTAL TRUNCATED]")
	}, 5*time.Second, 10*time.Millisecond)

	output := buf.String()
	assert.Contains(t, output, "level=DEBUG", "linhas do stderr só devem chegar em nível Debug")
	assert.Contains(t, output, "[TRUNCATED]", "linha maior que o teto deve ser truncada")
	assert.Contains(t, output, "[TOTAL TRUNCATED]", "total maior que o teto deve truncar a sessão")
	assert.LessOrEqual(t, len(output), 2*64*1024, "log capturado deve respeitar aproximadamente o teto total")
}
