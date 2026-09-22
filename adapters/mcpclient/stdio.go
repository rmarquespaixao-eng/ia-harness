package mcpclient

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rmarquespaixao-eng/ia-harness/internal/platform/proc"
)

const defaultTerminateTimeout = 5 * time.Second

// envNameRE valida nomes de variáveis de ambiente (FR-STD-005).
var envNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// validateStdio valida os campos stdio do Config.
func (c *Config) validateStdio() error {
	return c.Validate()
}

// Validate verifica a consistência do Config (Endpoint × Command, nomes de env).
func (c *Config) Validate() error {
	if c.Command == "" && c.Endpoint == "" {
		return fmt.Errorf("mcpclient: servidor %q sem Endpoint e sem Command", c.Name)
	}
	if c.Command != "" && c.Endpoint != "" {
		return fmt.Errorf("mcpclient: servidor %q: Endpoint e Command são mutuamente exclusivos", c.Name)
	}
	if c.Command != "" {
		for name := range c.EnvCredentials {
			if !envNameRE.MatchString(name) {
				return fmt.Errorf("mcpclient: servidor %q: nome de variável inválido: %q", c.Name, name)
			}
		}
		for name := range c.Env {
			if !envNameRE.MatchString(name) {
				return fmt.Errorf("mcpclient: servidor %q: nome de variável inválido: %q", c.Name, name)
			}
		}
	}
	return nil
}

// stdioTransport implementa mcp.Transport para servidores MCP stdio.
type stdioTransport struct {
	cfg  Config
	deps Deps
}

// Connect monta o exec.Cmd, resolve o executável, monta o ambiente mínimo,
// resolve credenciais e delega ao mcp.CommandTransport do SDK.
func (t *stdioTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	resolved, err := proc.Resolve(t.cfg.Command)
	if err != nil {
		return nil, fmt.Errorf("mcpclient: servidor %q (%s): %w",
			t.cfg.Name, filepath.Base(t.cfg.Command), err)
	}

	cmd := exec.Command(resolved, t.cfg.Args...)
	if t.cfg.Dir != "" {
		cmd.Dir = t.cfg.Dir
	}
	cmd.Env = proc.MinimalEnv(t.buildEnv(ctx))
	cmd.SysProcAttr = proc.Group()
	cmd.Stderr = proc.NewStderrSink(t.deps.Logger, t.secretValues())

	terminateTimeout := t.cfg.TerminateTimeout
	if terminateTimeout == 0 {
		terminateTimeout = defaultTerminateTimeout
	}

	ct := &mcp.CommandTransport{
		Command:           cmd,
		TerminateDuration: terminateTimeout,
	}
	conn, err := ct.Connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("mcpclient: servidor %q (%s): %w",
			t.cfg.Name, filepath.Base(t.cfg.Command), err)
	}
	return &groupConn{Connection: conn, pid: cmd.Process.Pid}, nil
}

// buildEnv combina Env + EnvCredentials resolvidas.
func (t *stdioTransport) buildEnv(ctx context.Context) map[string]string {
	env := make(map[string]string, len(t.cfg.Env)+len(t.cfg.EnvCredentials))
	for k, v := range t.cfg.Env {
		env[k] = v
	}
	for name, ref := range t.cfg.EnvCredentials {
		if t.deps.Credentials == nil {
			continue
		}
		val, err := t.deps.Credentials.Resolve(ctx, ref)
		if err != nil {
			continue
		}
		env[name] = val
	}
	return env
}

// secretValues devolve os valores resolvidos de credenciais para redação do stderr.
func (t *stdioTransport) secretValues() []string {
	if t.deps.Credentials == nil || len(t.cfg.EnvCredentials) == 0 {
		return nil
	}
	var secrets []string
	for _, ref := range t.cfg.EnvCredentials {
		val, err := t.deps.Credentials.Resolve(context.Background(), ref)
		if err != nil || val == "" {
			continue
		}
		secrets = append(secrets, val)
	}
	return secrets
}

// groupConn envolve o Connection do SDK e mata o grupo de processos no Close.
type groupConn struct {
	mcp.Connection
	pid int
}

// Close fecha a conexão do SDK e depois mata o grupo de processos.
func (c *groupConn) Close() error {
	err := c.Connection.Close()
	_ = proc.KillGroup(c.pid)
	return err
}

// isPermanentStdioError classifica erros de início como permanentes.
func isPermanentStdioError(err error) bool {
	return errors.Is(err, proc.ErrNotFound) ||
		errors.Is(err, proc.ErrRelativePath)
}
