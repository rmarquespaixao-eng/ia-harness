# ADR 0028 — MCP stdio via CommandTransport do SDK

**Status**: Accepted
**Data**: 2026-09-22
**Feature**: `specs/nucleo/023-mcp-stdio`

## Contexto

O `mcpclient` fala só Streamable HTTP. Hosts como o `harness-cli` precisam de servidores MCP locais
(stdio) e, pela própria constitution, não podem montar o subprocesso por conta própria (ADR 0006 do
`harness-cli`). Iniciar processos traz riscos que precisam de uma política única: execução por shell,
herança do ambiente do host (credenciais de provedor), credenciais em argv, processos órfãos e
stderr sem limite.

## Decisão

1. O transporte stdio usa `mcp.CommandTransport` do SDK oficial (já dependência). O harness monta
   o `exec.Cmd`: executável resolvido (absoluto ou por `PATH`), `Args` como argv, **sem shell**,
   **sem contexto de request** (vida ligada ao `Client`).
2. O filho recebe um **ambiente mínimo por allowlist** (`PATH`, `HOME`, `LANG`, `TMPDIR`, e no
   Windows `SystemRoot`/`ComSpec`/`PATHEXT`) mais `Config.Env`. Credenciais entram só por
   `Config.EnvCredentials`, resolvidas pelo `CredentialProvider` a cada início de processo.
3. Em Unix, o processo roda em grupo próprio e o `Close` mata o grupo depois do encerramento
   stdin→SIGTERM→SIGKILL do SDK. No Windows, o encerramento é best-effort e está documentado.
4. O stderr do filho vai para `logger.Debug`, truncado e com as credenciais resolvidas redigidas.
5. Falhas de início (executável ausente, caminho relativo, credencial não resolvida) são
   permanentes e não disparam reconexão. Queda do processo usa a reconexão única existente.
6. O contrato `contracts/config` ganha os campos stdio em `mcp_server`, com exclusividade
   `endpoint` × `command`.

## Alternativas consideradas

- **Transporte próprio sobre `mcp.IOTransport`**: controle total, mas duplica framing e
  encerramento já testados no SDK. Rejeitada.
- **Host injeta `Deps.Transport`**: custo zero no harness, mas cada host reimplementa a parte
  sensível de segurança. Rejeitada.
- **Herdar o ambiente e filtrar por denylist**: mais compatível, porém um segredo com nome
  inesperado vaza. Rejeitada.
- **Ambiente vazio**: máximo isolamento, mas quebra executáveis que dependem de `PATH`/`HOME`.
  Rejeitada.
- **Job Objects no Windows**: encerramento completo da árvore, mas custa código e testes de
  plataforma sem demanda atual. Adiada.

## Consequências

- **Positivas**: um único ponto testado para subprocesso MCP; hosts só declaram
  `Command`/`Args`/`Env`/`EnvCredentials`; nenhuma dependência nova.
- **Negativas**: servidores que dependem de variáveis do host fora da allowlist precisam
  declará-las em `Env`; no Windows, netos podem sobreviver ao `Close`.
- **Mitigação**: a doc de feature lista a allowlist e orienta a declarar o necessário; a
  limitação do Windows fica documentada.
