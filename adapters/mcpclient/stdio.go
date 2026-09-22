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

// ErrCredential indica que uma credencial declarada em EnvCredentials não pôde
// ser resolvida (referência inválida ou Deps.Credentials ausente). O processo
// filho MUST NOT iniciar nesse caso (CU-STD-1 fluxo 3a, FR-STD-005); o erro
// nunca contém o valor da credencial.
var ErrCredential = errors.New("mcpclient: falha ao resolver credencial")

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

// Connect monta o exec.Cmd, resolve o executável, resolve as credenciais uma
// única vez com o ctx recebido e delega ao mcp.CommandTransport do SDK. Se
// alguma credencial declarada em EnvCredentials não puder ser resolvida, o
// processo filho MUST NOT iniciar (CU-STD-1 fluxo 3a, FR-STD-005).
func (t *stdioTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	resolved, err := proc.Resolve(t.cfg.Command)
	if err != nil {
		return nil, fmt.Errorf("mcpclient: servidor %q (%s): %w",
			t.cfg.Name, filepath.Base(t.cfg.Command), err)
	}

	env, secrets, err := t.resolveEnv(ctx)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(resolved, t.cfg.Args...)
	if t.cfg.Dir != "" {
		cmd.Dir = t.cfg.Dir
	}
	cmd.Env = proc.MinimalEnv(env)
	cmd.SysProcAttr = proc.Group()
	cmd.Stderr = proc.NewStderrSink(t.deps.Logger, secrets)

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

// resolveEnv combina Env + EnvCredentials resolvidas uma única vez com ctx e
// devolve também os valores secretos para redação do stderr (mesmos valores
// usados no ambiente do filho). Credencial não resolvida — referência inválida
// ou Deps.Credentials nil com EnvCredentials declarado — devolve um erro
// nomeado citando a variável e o servidor, nunca o valor (FR-STD-005).
func (t *stdioTransport) resolveEnv(ctx context.Context) (env map[string]string, secrets []string, err error) {
	env = make(map[string]string, len(t.cfg.Env)+len(t.cfg.EnvCredentials))
	for k, v := range t.cfg.Env {
		env[k] = v
	}
	for name, ref := range t.cfg.EnvCredentials {
		if t.deps.Credentials == nil {
			return nil, nil, fmt.Errorf("mcpclient: servidor %q: credencial de %q não resolvida: nenhum CredentialProvider injetado: %w",
				t.cfg.Name, name, ErrCredential)
		}
		val, resolveErr := t.deps.Credentials.Resolve(ctx, ref)
		if resolveErr != nil {
			return nil, nil, fmt.Errorf("mcpclient: servidor %q: credencial de %q não resolvida: %w",
				t.cfg.Name, name, ErrCredential)
		}
		env[name] = val
		if val != "" {
			secrets = append(secrets, val)
		}
	}
	return env, secrets, nil
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
		errors.Is(err, proc.ErrRelativePath) ||
		errors.Is(err, ErrCredential)
}
