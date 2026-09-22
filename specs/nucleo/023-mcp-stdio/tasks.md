# Tasks: MCP stdio — servidor MCP como processo local (023)

**Input**: `specs/nucleo/023-mcp-stdio/` (spec · plan · use-cases) · ADR 0028
**Pré-requisitos**: plan aprovado. TDD estrito: cada teste entra em Red antes do código. Cada fase
fecha com `make verify` verde.

## Fase 1 — Contrato de config

- [x] T2301 Teste Red: conformidade de `mcp_server` stdio (aceita `command`+`args`, recusa
  `endpoint`+`command`, recusa nenhum dos dois, arquivo HTTP atual continua válido) —
  `contracts/conformance_test.go` — FR-STD-001/012
- [x] T2302 `contracts/config/harness_config.json`: campos stdio + `oneOf`; `go generate ./...`
  (fallback do plan se o gerador não suportar `oneOf`) — FR-STD-001/012
- [x] T2303 Teste Red + impl: `adapters/config.MCPServer`/`MCPServers()` com os campos stdio —
  `adapters/config/file_test.go`, `file.go` — FR-STD-001

## Fase 2 — `internal/platform/proc` (regra pura e SO)

- [x] T2304 [P] Teste Red: `Resolve` (absoluto ok, nome por PATH, relativo com separador recusado,
  inexistente → `ErrNotFound`, nome com espaço recusado) — `internal/platform/proc/resolve_test.go` —
  FR-STD-002/003
- [x] T2305 `proc.Resolve` — `internal/platform/proc/resolve.go` — FR-STD-003
- [x] T2306 [P] Teste Red: `MinimalEnv` (só a allowlist do host, `extra` sobrepõe, ordem
  determinística, variável do host fora da lista ausente) — `internal/platform/proc/env_test.go` —
  FR-STD-004
- [x] T2307 `proc.MinimalEnv` — `internal/platform/proc/env.go` — FR-STD-004
- [x] T2308 [P] Teste Red: `StderrSink` (quebra por linha, trunca por linha e total, redige os
  valores-isca, emite `Debug`) — `internal/platform/proc/stderr_test.go` — FR-STD-009
- [x] T2309 `proc.StderrSink` — `internal/platform/proc/stderr.go` — FR-STD-009
- [x] T2310 Teste Red (unix): `Group`+`KillGroup` matam filho e neto; `ESRCH` ignorado —
  `internal/platform/proc/group_unix_test.go` — FR-STD-007
- [x] T2311 `proc.Group`/`KillGroup` (`group_unix.go`, `group_other.go` best-effort) — FR-STD-007

## Fase 3 — Transporte stdio no `mcpclient`

- [x] T2312 Servidor de teste por re-execução (`TestMain` + modos `echo`, `env`, `die-after-1`,
  `spawn-child`, `stderr-flood`, `exit-now`) — `adapters/mcpclient/stdio_server_test.go` — NFR
- [x] T2313 Teste Red: `Config.validate` (`Endpoint`+`Command`, nome de env inválido) e
  `Config` sem `Command` inalterado — `adapters/mcpclient/stdio_test.go` — FR-STD-001/012
- [x] T2314 Teste Red: `List`/`Call` via processo publica tools com namespace — CU-STD-1 —
  FR-STD-001/002/011
- [ ] T2315 Teste Red: env do host não vaza; `Env` e `EnvCredentials` chegam; isca ausente de
  argv, log e erro — CU-STD-1 — FR-STD-004/005/013
- [ ] T2316 `stdioTransport` + `groupConn` + validação + ramo em `transport()` —
  `adapters/mcpclient/stdio.go`, `client.go` — FR-STD-001..006/010/013
- [ ] T2317 Teste Red: cancelar o ctx de uma `Call` não mata o servidor; próxima `Call` usa o
  mesmo PID — FR-STD-006
- [ ] T2318 Teste Red: `die-after-1` reconecta uma vez; `exit-now` e executável ausente falham
  sem laço (contagem de inícios) — CU-STD-2 — FR-STD-008
- [x] T2319 `shouldReconnect`: falhas de início permanentes — `adapters/mcpclient/reconnect.go` —
  FR-STD-008
- [ ] T2320 Teste Red (unix): `Close` sem órfão (`spawn-child`) dentro de `TerminateTimeout` +
  margem — CU-STD-3 — FR-STD-007
- [ ] T2321 Teste Red: `stderr-flood` limitado e só em `Debug` — FR-STD-009

## Fase 4 — Fechamento

- [x] T2322 `docs/features/mcp-stdio.md` (uso, allowlist, credenciais, limitação Windows) e
  exemplo em `examples/` se aplicável
- [ ] T2323 ADR 0028 → `Accepted`; CHANGELOG `[0.4.0]`; `make verify` + `go test -race ./...` verdes
- [ ] T2324 Release `v0.4.0` (tag) conforme runbook de publicação existente — destrava
  `harness-cli` T2050–T2053

## Dependências

- T2301→T2302→T2303.
- T2304→T2305; T2306→T2307; T2308→T2309; T2310→T2311 (pares independentes entre si, [P]).
- Fase 2 + T2312 → T2313–T2321 (Red antes de T2316/T2319).
- Tudo → T2322→T2323→T2324.

## Rastreabilidade — tasks × requisitos

| Requisito | Tasks |
|---|---|
| FR-STD-001 | T2301–T2303, T2313, T2314, T2316 |
| FR-STD-002 | T2304, T2314, T2316 |
| FR-STD-003 | T2304, T2305 |
| FR-STD-004 | T2306, T2307, T2315 |
| FR-STD-005 | T2315, T2316 |
| FR-STD-006 | T2316, T2317 |
| FR-STD-007 | T2310, T2311, T2320 |
| FR-STD-008 | T2318, T2319 |
| FR-STD-009 | T2308, T2309, T2321 |
| FR-STD-010 | T2316 |
| FR-STD-011 | T2314 |
| FR-STD-012 | T2301, T2302, T2313 |
| FR-STD-013 | T2315, T2316 |
| CU-STD-1/2/3 | T2314–T2320 |
