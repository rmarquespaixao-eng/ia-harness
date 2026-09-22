# Plan: MCP stdio — servidor MCP como processo local (023)

**Feature**: `specs/nucleo/023-mcp-stdio/` · **Status**: Draft (aguardando portão)

Constitution: **não viola** nenhuma seção.
- §2: nenhuma dependência nova, porque `mcp.CommandTransport` já está no SDK em uso. O contrato de
  arquivo muda de forma aditiva em `contracts/config` e o gerado é regenerado.
- §3: o I/O fica no adaptador `mcpclient`. A infra de processo vai para `internal/platform/proc`
  (não pública). Nenhum tipo do SDK vaza para a API pública.
- §4: sem shell, ambiente mínimo, credencial só no env do filho e redigida em log/erro.
- §7: nenhum teste toca rede. O servidor de teste é o próprio binário de teste re-executado.

## Arquitetura

```text
Client.ensureSession ─► transport()
                         ├─ Deps.Transport != nil ─► usa direto (inalterado)
                         ├─ Endpoint != ""        ─► StreamableClientTransport (inalterado)
                         └─ Command != ""         ─► stdioTransport
                                                     │ Connect(ctx):
                                                     │  1. proc.Resolve(Command)       (FR-STD-003)
                                                     │  2. proc.MinimalEnv(Env)        (FR-STD-004)
                                                     │     + EnvCredentials resolvidas (FR-STD-005)
                                                     │  3. exec.Command (sem ctx)      (FR-STD-002/006)
                                                     │     Dir, SysProcAttr = proc.Group()
                                                     │     Stderr = proc.StderrSink    (FR-STD-009)
                                                     │  4. mcp.CommandTransport{Cmd, TerminateDuration}
                                                     │     .Connect(ctx)
                                                     └─► groupConn{Connection} ── Close():
                                                          conn.Close()  (stdin → SIGTERM → SIGKILL, SDK)
                                                          proc.KillGroup(pid)       (FR-STD-007)
```

1. **Config** (`adapters/mcpclient/client.go`), campos aditivos:
   `Command string`, `Args []string`, `Env map[string]string`,
   `EnvCredentials map[string]string` (variável → `CredentialRef`), `Dir string`,
   `TerminateTimeout time.Duration` (0 = 5 s). A validação (`Config.validate`) recusa `Endpoint` e
   `Command` juntos e nomes de variável inválidos. Ela roda em `transport()`, antes de qualquer
   processo.
2. **`internal/platform/proc`** (novo, regra de processo reutilizável):
   - `Resolve(cmd string) (string, error)`: aceita caminho absoluto; nome sem separador passa por
     `exec.LookPath`; relativo com separador é recusado. Erros `ErrNotFound`/`ErrRelativePath`.
   - `MinimalEnv(extra map[string]string) []string`: allowlist `PATH`, `HOME`, `LANG`, `TMPDIR`
     (+ `SystemRoot`, `ComSpec`, `PATHEXT` no Windows) lidas do host, mais `extra`, em ordem
     determinística.
   - `Group() *syscall.SysProcAttr` e `KillGroup(pid int) error`: `Setpgid` + `kill(-pgid, SIGKILL)`
     em `group_unix.go`; no-op best-effort em `group_other.go` (build tags).
   - `StderrSink`: `io.Writer` que quebra por linha, trunca (4 KiB por linha, 64 KiB por processo),
     substitui os valores secretos informados por `[REDACTED]` e emite `logger.Debug`.
3. **Transporte stdio** (`adapters/mcpclient/stdio.go`): `stdioTransport` implementa
   `mcp.Transport`. Monta o `exec.Cmd` sem contexto de request (FR-STD-006), delega o framing e o
   encerramento ao `mcp.CommandTransport` e devolve `groupConn`, que mata o grupo depois do `Close`
   do SDK (evita netos órfãos, FR-STD-007). As credenciais são resolvidas a cada `Connect`, então
   cada reinício relê o `CredentialProvider`.
4. **Reconexão** (`reconnect.go`): sem mudança de política. Falhas de início (`proc.ErrNotFound`,
   `ErrRelativePath`, credencial não resolvida, `exec` falhou) são classificadas como **permanentes**
   em `shouldReconnect`. Assim não há laço (FR-STD-008). Processo morto aparece como
   `mcp.ErrConnectionClosed`/EOF e reconecta uma vez, como no HTTP.
5. **Erros** (FR-STD-013): `mcpclient: servidor %q (%s): ...` com `filepath.Base(Command)`. `Args`
   só aparece em `logger.Debug`.
6. **Arquivo de config**:
   - `contracts/config/harness_config.json` → `mcp_server`: `required` passa a `["name"]`.
     Entram `command`, `args` (array de string), `env` e `env_credentials` (mapas string→string),
     `dir` e `terminate_timeout_seconds`. A exclusividade é `oneOf` entre
     `{required:[endpoint]}` e `{required:[command]}`.
   - `go generate ./...` regenera `contracts/gen/config.go`.
   - `adapters/config/file.go`: `MCPServer` ganha os campos e `MCPServers()` os copia.
7. **Docs**: `docs/features/mcp-stdio.md`, com uso, ambiente mínimo, credenciais e a limitação no
   Windows. Também ADR 0028 e CHANGELOG `[0.4.0]`.

## Contratos (aditivos)

- `mcpclient.Config`: `Command`, `Args`, `Env`, `EnvCredentials`, `Dir`, `TerminateTimeout`.
- `contracts/config/harness_config.json`: campos stdio em `mcp_server`. `endpoint` deixa de ser
  obrigatório isoladamente, mas o `oneOf` exige `endpoint` **ou** `command`. Todo arquivo que valida
  hoje continua validando.
- `adapters/config.MCPServer`: campos equivalentes.
- Release: `v0.4.0` (MINOR).

## Estratégia de teste (sem rede, sem `npx`)

- **Servidor de teste por re-execução**: `TestMain` em `adapters/mcpclient` verifica
  `IAH_MCP_TESTSERVER=<modo>`. Quando a variável está presente, o binário de teste vira um servidor
  MCP stdio do SDK (`mcp.StdioTransport`) em vez de rodar os testes. O `Client` usa
  `Command=os.Executable()` e passa o modo por `Config.Env`. Isso também prova que só o env declarado
  chega ao filho.
- **Modos do servidor**: `echo` (tool `echo`), `env` (tool que devolve uma variável pedida),
  `die-after-1` (sai após a 1ª chamada), `spawn-child` (cria um neto e informa o PID),
  `stderr-flood` (escreve muito no stderr) e `exit-now` (sai antes do handshake).
- **Testes puros** de `internal/platform/proc` (`Resolve`, `MinimalEnv`, `StderrSink`) são
  table-driven.
- Os testes de grupo de processos e de netos rodam só em Unix (`//go:build unix`).
- **Valor-isca** de credencial verificado em argv (`/proc/<pid>/cmdline` quando disponível), log
  capturado, erro e stderr redigido.

## Alternativas consideradas

- **Transporte próprio sobre `mcp.IOTransport`**: rejeitado no portão, porque duplica framing e
  encerramento já testados no SDK.
- **Host injeta `Deps.Transport`**: rejeitado no portão, porque espalha a parte de segurança por
  cada host.
- **`exec.CommandContext` com o ctx da operação**: rejeitado. O cancelamento de uma chamada
  mataria o servidor (FR-STD-006).
- **Exclusividade só em código (sem `oneOf`)**: fica como fallback caso o `go-jsonschema` não gere
  tipos utilizáveis com `oneOf`. Nesse caso a regra vai para `adapters/config.Load` e a divergência
  é registrada no ADR.

## Riscos

- **`oneOf` no gerador**: o `go-jsonschema` pode ignorar ou rejeitar `oneOf` no nível do objeto.
  Mitigação: T2302 valida cedo e aplica o fallback acima.
- **Kill do grupo depois do `Close` do SDK**: se o SDK já tiver matado o líder, o `kill(-pgid)`
  ainda alcança os netos, porque o grupo sobrevive ao líder. `ESRCH` é ignorado.
- **Flakiness de processo em CI**: tempos curtos (`TerminateTimeout` de 200 ms nos testes) e
  espera ativa limitada por prazo, nunca `sleep` fixo.
- **Windows**: sem cobertura automatizada de netos. O comportamento best-effort fica documentado.
