# Feature 023: MCP stdio — servidor MCP como processo local

**Status**: Implementado · **Spec**: `specs/nucleo/023-mcp-stdio/` · **ADR**: 0028

## Objetivo

Permitir que o `mcpclient` consuma servidores MCP distribuídos como processo local (stdio),
sem que o host precise montar o subprocesso. O harness garante execução segura: sem shell,
ambiente mínimo, credenciais só por env do filho, grupo de processos e stderr truncado.

## Uso

```go
client := mcpclient.New(mcpclient.Config{
    Name:    "filesystem",
    Command: "/usr/local/bin/mcp-filesystem",
    Args:    []string{"/data"},
    Env:     map[string]string{"LOG_LEVEL": "info"},
    EnvCredentials: map[string]string{
        "API_TOKEN": "vault:fs-token",
    },
    TerminateTimeout: 10 * time.Second,
}, mcpclient.Deps{
    Credentials: credentialProvider,
    Logger:      logger,
})
defer client.Close()

tools, err := client.List(ctx)
```

## Contrato de config (arquivo)

```json
{
  "mcp_servers": [
    {
      "name": "filesystem",
      "command": "/usr/local/bin/mcp-filesystem",
      "args": ["/data"],
      "env": {"LOG_LEVEL": "info"},
      "env_credentials": {"API_TOKEN": "vault:fs-token"},
      "terminate_timeout_seconds": 10
    }
  ]
}
```

`endpoint` e `command` são mutuamente exclusivos (validado pelo schema `oneOf`).

## Ambiente mínimo (allowlist)

O filho recebe apenas:
- `PATH`, `HOME`, `LANG`, `TMPDIR` (do host, se definidas)
- `SystemRoot`, `ComSpec`, `PATHEXT` (Windows, se definidas)
- `Config.Env` (declarado)
- `Config.EnvCredentials` resolvidas pelo `CredentialProvider`

Variáveis do host fora da allowlist **não** são herdadas.

## Credenciais

- Entram **somente** por `EnvCredentials` (variável → `CredentialRef`).
- Resolvidas a cada início de processo (reconexão relê o provider).
- **Nunca** aparecem em `Args`, `Env`, log, erro ou auditoria.
- O stderr do filho é redigido com os valores resolvidos.

## Segurança

- **Sem shell**: `Command` é executado diretamente com `Args` como argv.
- **Caminho relativo com separador** (`./x`, `bin/x`) é recusado.
- **Grupo de processos** (Unix): `Close` mata a árvore inteira.
- **Stderr**: truncado (4 KiB/linha, 64 KiB/sessão), redigido, em `Debug`.

## Limitações

- **Windows**: encerramento best-effort; netos podem sobreviver ao `Close`.
- **Sem sandbox**: o host é responsável por políticas de executáveis.
- **Sem pool**: um processo por `Client`.

## Arquivos

| Arquivo | O que faz |
|---|---|
| `adapters/mcpclient/stdio.go` | `stdioTransport`, `groupConn`, validação |
| `adapters/mcpclient/client.go` | `Config` com campos stdio, `transport()` |
| `adapters/mcpclient/reconnect.go` | `shouldReconnect` com erros permanentes |
| `internal/platform/proc/resolve.go` | `Resolve` (executável sem shell) |
| `internal/platform/proc/env.go` | `MinimalEnv` (allowlist) |
| `internal/platform/proc/stderr.go` | `StderrSink` (truncamento + redação) |
| `internal/platform/proc/group_unix.go` | `Group`/`KillGroup` (Unix) |
| `internal/platform/proc/group_other.go` | No-op (não-Unix) |
| `contracts/config/harness_config.json` | Schema com `oneOf` endpoint/command |
