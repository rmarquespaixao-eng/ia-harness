# Runbook — Smoke real do adapter Responses contra OpenCode Zen/Go (T312)

- **Data:** 2026-09-16
- **Repo:** `ia-harness`
- **Feature:** `specs/nucleo/004-provider-opencode-zen-go/`
- **Status:** preparado pelo agente; **pendente de execução do operador** (AGENTS.md itens 8–9)

## Objetivo

Validar o adapter `adapters/provider/openai_responses` contra a Responses API real do OpenCode Go/Zen (`gpt-5.6-luna`), com tool MCP real e chave fora do repo.

## Contexto

A suíte cobre o adapter com `httptest` (sem rede), mas nenhuma chamada real foi feita. O Go pede `User-Agent` próprio e `x-opencode-session`; a Responses API usa `store=false` e eventos `response.*`. Este smoke prova SC-ZG-001 e confirma a auth (Bearer).

## Pré-condições

- Go ≥ 1.27 e `make verify` verde.
- Assinatura OpenCode Go ativa e API key obtida em `https://opencode.ai/auth` (revogável).
- VPN/chave do MCP de homolog do financeiro (ver runbook do smoke do financeiro).

## Passos

1. **Pré-checagem** — build e versão.
   ```bash
   go version && go build ./...
   ```
   Saída esperada: `go1.27...` e build sem saída.

2. **Gravar as chaves fora do repo** (sem eco).
   ```bash
   install -d -m 700 ~/.config/ia-harness/secrets
   umask 077
   read -rsp 'Chave OpenCode Go: ' K && printf '%s' "$K" > ~/.config/ia-harness/secrets/opencode-go-key; unset K
   read -rsp 'Chave MCP financeiro: ' M && printf '%s' "$M" > ~/.config/ia-harness/secrets/financeiro-mcp-key; unset M
   stat -c '%a %n' ~/.config/ia-harness/secrets/*
   ```
   Saída esperada: permissão `600` nos dois arquivos.

3. **Smoke de texto sem tool** (uma chamada simples) — usa `harnessctl` com o adapter? Hoje `harnessctl` só monta `openai`/`anthropic`; para o Responses, rode o exemplo de wiring e um turno pelo host. Alternativa mínima: um pequeno `go run` em `examples` apontando o perfil `go-gpt-luna`. Registre a saída.
   ```bash
   OPENCODE_GO_API_KEY="$(cat ~/.config/ia-harness/secrets/opencode-go-key)" \
     go run ./examples/financeiro
   ```
   Saída esperada: `wiring do harness financeiro válido` (valida a Config com os 4 providers, incl. Responses).

4. **Smoke com tool real** (SC-ZG-001) — via host de teste, perfil `go-gpt-luna` e MCP de homolog.
   - Critério: `TextDelta` incremental; `ToolCallEvent` de `financeiro.listar_contas`; `ToolResultEvent(ok)`; `response.completed.usage` → `estimated=false`.
   - Confirmar no header que `x-opencode-session` foi enviado (log do adapter ou proxy local).

5. **Registrar o Resultado** e conferir a auth real (Bearer aceito).

## Rollback

1. Revogar a chave em `https://opencode.ai/auth` (console).
2. `rm -f ~/.config/ia-harness/secrets/opencode-go-key` (e a do MCP, se criada).
3. Nada persiste no harness; nenhum arquivo do repo é alterado.

## Riscos e o que NÃO fazer

- **NÃO** colar a chave em `argv`, histórico ou repo.
- **NÃO** usar dados reais com tools destrutivas (default deny + confirmação já protegem).
- A Responses API pode evoluir; eventos desconhecidos são ignorados por design (FR-ZG-003).
- Sem `x-opencode-session`/`User-Agent`, o Go pode degradar/limitar — use `SessionHeader`/`Headers`.

## Registro

| Item | O que muda |
|---|---|
| Arquivos/configs | `~/.config/ia-harness/secrets/{opencode-go-key,financeiro-mcp-key}` (`0600`, fora do repo) |
| Pacotes/versões | Nenhum; `go run` |
| Serviços/portas | Tráfego de saída para `opencode.ai/zen/go` e `/mcp` de homolog |
| Credenciais/ENV | 1 chave OpenCode Go (revogável) + chave MCP; trilha do financeiro ganha linhas |

## Resultado

**Pendente — preencher após a execução (operador).**

- Data/executor: _a preencher_
- Passos 1–4: _saídas reais (sem segredos)_
- Auth confirmada (Bearer)? _a preencher_
- Desvios vs. plano: _a preencher_
