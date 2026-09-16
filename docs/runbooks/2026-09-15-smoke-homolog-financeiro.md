# Runbook — Smoke real do harness contra o `/mcp` de homolog do financeiro (T089)

- **Data:** 2026-09-15
- **Repo:** `ia-harness`
- **Feature:** `specs/nucleo/001-harness-ia-reutilizavel/` (T089; valida `quickstart.md` §Smoke com o financeiro)
- **Status:** preparado pelo agente; **pendente de execução do operador** (o operador executa, o agente audita — AGENTS.md itens 8 e 9)

## Objetivo

Validar o harness real (biblioteca + `cmd/harnessctl`) contra o servidor `/mcp` de homologação do `financeiro-api-v2`, com provedor real e chave fora do repo.

## Contexto

O núcleo está implementado e coberto pelo gate local (`make verify`, testes `-race`, sem rede), mas nenhuma execução real ponta a ponta foi feita: o smoke local usa provider fake e servidor MCP em memória. Falta provar o núcleo contra um servidor MCP real — a primeira integração é o `financeiro-api-v2` em homolog (`https://homolog-api-financeiro.homelab-cloud.com/mcp`, 96 tools, SSE, progresso em recálculo/transferência e destrutivas com `confirm:true`). Este runbook cobre SC-001/SC-002 (loop + tool real), SC-003 (confirmação de destrutiva) e SC-005 (auditoria/redação); a execução é manual, fora do gate, e nada do harness persiste após o processo.

## Pré-condições

- `go version` ≥ **1.27** e o repositório `ia-harness` com `make verify` verde (T088).
- `cmd/harnessctl` disponível (T080 — CLI de dev/smoke da Fase 9; sem garantia de produto).
- VPN ativa (o host resolve para `10.8.0.1`) e `curl -fsS https://homolog-api-financeiro.homelab-cloud.com/healthz` = `{"status":"ok"}`.
- Chave de API do MCP de homolog **criada na UI do financeiro** para o usuário operador (revogável a qualquer momento) e gravada em arquivo `0600` **fora do repo**.
- Um provedor real com credencial em variável de ambiente (ex.: `OPENROUTER_API_KEY` para o adapter OpenAI-compatible; `ANTHROPIC_API_KEY` para o Anthropic).
- Nada de segredo em chat, `argv` ou histórico do shell; nenhuma credencial entra na `Config` (constitution §4).

## Passos

1. **Pré-checagem (leitura)** — VPN, health da API, versão de Go e build do CLI.
   ```bash
   curl -fsS https://homolog-api-financeiro.homelab-cloud.com/healthz
   curl -s -o /dev/null -w '%{http_code}\n' -X POST https://homolog-api-financeiro.homelab-cloud.com/mcp -d '{}'
   go version
   go build ./cmd/harnessctl
   ```
   O que faz: confirma que o endpoint está no ar, que sem chave ele responde `401` e que o CLI compila.
   Saída esperada: `{"status":"ok"}`, `401`, `go1.27...` e build sem saída.
   Como verificar: os três valores acima; qualquer timeout indica VPN inativa.

2. **Criar a chave do MCP na UI e gravá-la em arquivo `0600` fora do repo** — na UI (`https://homolog-financeiro.homelab-cloud.com` → API keys, aba Configurações) crie uma chave dedicada (ex.: `ia-harness-smoke-2026-09-15`) e copie o valor.
   ```bash
   install -d -m 700 ~/.config/ia-harness/secrets
   umask 077
   read -rsp 'Chave do MCP (colar e Enter): ' MCP_KEY && printf '%s' "$MCP_KEY" > ~/.config/ia-harness/secrets/financeiro-mcp-key; unset MCP_KEY
   stat -c '%a %s %n' ~/.config/ia-harness/secrets/financeiro-mcp-key
   ```
   O que faz: lê a chave **sem eco** e grava o arquivo sem newline; a chave nunca passa por `argv`/histórico.
   Saída esperada: permissão `600` e tamanho de 52 bytes (`fin_` + 48 hex) ou o prefixo/tamanho vigente do homolog.
   Como verificar: `stat` mostra `600`; `test -s ~/.config/ia-harness/secrets/financeiro-mcp-key && echo ok`.

3. **Disponibilizar a credencial do provedor em ENV** — crie o arquivo com editor (sem colar no shell) e carregue na sessão.
   ```bash
   ${EDITOR:-vi} ~/.config/ia-harness/secrets/provider.env   # conteúdo: OPENROUTER_API_KEY=<chave-do-provedor>
   chmod 600 ~/.config/ia-harness/secrets/provider.env
   set -a; . ~/.config/ia-harness/secrets/provider.env; set +a
   test -n "$OPENROUTER_API_KEY" && echo 'credencial do provedor carregada'
   ```
   O que faz: carrega a credencial do provedor como variável de ambiente da sessão, referenciada por `env:OPENROUTER_API_KEY`.
   Saída esperada: `credencial do provedor carregada`.
   Como verificar: a variável existe sem que o valor seja impresso.

4. **Smoke de leitura (SC-001/SC-002)** — um turno real com tool MCP e streaming.
   ```bash
   export FINANCEIRO_MCP_URL=https://homolog-api-financeiro.homelab-cloud.com/mcp
   go run ./cmd/harnessctl \
     -prompt "qual meu saldo deste mês?" \
     -mcp-url "$FINANCEIRO_MCP_URL" \
     -mcp-credential-ref "file:$HOME/.config/ia-harness/secrets/financeiro-mcp-key" \
     -credential-ref env:OPENROUTER_API_KEY \
     -allow-tools 'financeiro.*'
   ```
   O que faz: monta o harness com o `mcpclient` apontando para o `/mcp` de homolog (chave via `CredentialProvider`, ref `file:`), um provedor real (ref `env:`) e política `default deny` ampliada apenas para `financeiro.*`; roda um turno e imprime os eventos.
   Saída esperada: `TextDelta` incremental; `ToolCallEvent` de uma tool de leitura (ex.: `financeiro.listar_contas`); `ToolResultEvent(ok)`; resposta final coerente com os dados do homolog; `UsageEvent` com custo reportado (`estimated=false`) ou estimado (`estimated=true`).
   Como verificar: a resposta cita valores reais do usuário; nenhuma confirmação é pedida neste passo; o turno termina com `StopReason=completed`.

5. **Conferir a trilha redigida no financeiro (SC-005/FR-024)** — abra a UI (`https://homolog-financeiro.homelab-cloud.com` → **Logs** / Chamadas MCP) ou a rota REST `GET /api/v1/users/{userId}/mcp-calls`; opcionalmente peça à própria trilha via tool `financeiro.listar_chamadas_mcp` em outra sessão.
   O que faz: comprova que a chamada do harness chegou ao servidor e foi auditada do lado do financeiro.
   Saída esperada: chamada com `status=success`, `duration_ms` e `client_name` do smoke; payloads redigidos/truncados — **sem** a API key, Base64 de documento ou assinatura de URL.
   Como verificar: procurar a chamada do passo 4 pelo horário; confirmar redação no detalhe da chamada.

6. **Confirmação nas destrutivas (SC-003)** — peça uma ação destrutiva contra um **UUID de teste inexistente** (nada real é tocado):
   ```bash
   go run ./cmd/harnessctl \
     -prompt "exclua a transação 00000000-0000-0000-0000-000000000000" \
     -mcp-url "$FINANCEIRO_MCP_URL" \
     -mcp-credential-ref "file:$HOME/.config/ia-harness/secrets/financeiro-mcp-key" \
     -credential-ref env:OPENROUTER_API_KEY \
     -allow-tools 'financeiro.*'
   ```
   O que faz: o exemplo do `harnessctl` carrega a política do contrato (`allow financeiro.*` + `confirm` nas destrutivas); o turno deve pausar em `awaiting_confirmation` antes de qualquer chamada MCP.
   Saída esperada: `State=awaiting_confirmation` com `Pending` preenchido; ao **negar** no prompt, o turno conclui sem executar a tool (nenhuma chamada aparece na trilha); ao **aprovar**, a chamada retorna `isError` `NOT_FOUND` e nada é alterado.
   Como verificar: repetir o passo 5 e confirmar que a trilha não registra execução no caminho da negativa.

7. **Progresso em operação longa (SC-001)** — em homolog, peça um recálculo reenviando **os mesmos** dias de fechamento/vencimento do cartão (operação longa, sem mudança de cadastro):
   ```bash
   go run ./cmd/harnessctl \
     -prompt "atualize o cartão <final-4> mantendo os dias de fechamento e vencimento atuais para recalcular a fatura" \
     -mcp-url "$FINANCEIRO_MCP_URL" \
     -mcp-credential-ref "file:$HOME/.config/ia-harness/secrets/financeiro-mcp-key" \
     -credential-ref env:OPENROUTER_API_KEY \
     -allow-tools 'financeiro.*'
   ```
   O que faz: dispara a operação longa do financeiro (`atualizar_cartao`), que envia `notifications/progress`.
   Saída esperada: `ProgressEvent`s chegando **antes** de `ToolResultEvent`.
   Como verificar: ordem dos eventos no CLI; se o modelo escolher `financeiro.transferir_transacoes`, **não aprove** — é destrutiva e sai do escopo deste smoke (o passo 6 já cobre confirmação).

8. **Registrar o resultado** — preencha a seção **Resultado** abaixo com data/executor, saídas reais e desvios; desvio de requisito reabre o artefato SDD, não se corrige direto no editor.

## Rollback

1. Revogar a chave criada no passo 2: UI (`https://homolog-financeiro.homelab-cloud.com` → API keys) ou `UPDATE user_api_keys SET revoked_at = now() WHERE key_prefix = '<fin_xxxx>'` no Postgres de homolog.
2. Apagar os segredos locais: `rm -f ~/.config/ia-harness/secrets/financeiro-mcp-key ~/.config/ia-harness/secrets/provider.env` (e remover a ENV do shell, se carregada).
3. **Nada persiste no harness**: a sessão vive só em memória do CLI e morre com o processo; nenhum arquivo do repo é alterado. No financeiro, as linhas da trilha permanecem e expiram pela retenção (90 dias).

## Riscos e o que NÃO fazer

- **NÃO** colar a chave em `argv`, no histórico, no chat ou no repo — só no arquivo `0600` fora do repo, via `read -rsp`/editor.
- **NÃO** usar a chave `sk_ag_…` de produção nem apontar para o MCP legado (`financeiro-mcp.homelab-cloud.com`), que tem outro auth e será aposentado.
- **NÃO** rodar com `-allow-tools '*'` sem necessidade: amplia o acesso além do caso de uso e fere o default deny.
- **NÃO** aprovar destrutivas com dados reais; sem `confirm` o financeiro rejeita (`VALIDATION`) e, com `confirm:true`, a alteração é real e irreversível.
- **NÃO** executar o passo 7 com dias diferentes dos atuais em homolog sem aceite do dono dos dados: o recálculo altera faturas.
- Rate limit de 120 req/min por chave (429 vira `denied` na trilha), corpo ≤ 16 MiB, upload ≤ 10 MiB/arquivo, timeout de 120 s por tool; restart da API derruba as sessões MCP (em memória).
- O modelo pode alucinar uma tool inexistente: esperado `ToolResultEvent(is_error)` com o erro devolvido ao modelo — não é falha do harness.

## Registro

| Item | O que muda |
|---|---|
| Arquivos/configs | `~/.config/ia-harness/secrets/financeiro-mcp-key` e `provider.env` (`0600`, fora do repo); **nenhum** arquivo do repo `ia-harness` |
| Pacotes/versões | Nenhum pacote instalado; execução via `go run` (Go ≥ 1.27) |
| Serviços/portas | Nenhum serviço local; só tráfego de saída para o `/mcp` de homolog e para o provedor do modelo |
| Credenciais/ENV | 1 chave de API nova no financeiro (revogável), credencial do provedor na ENV da sessão; `mcp_call_logs` (trilha) ganha linhas |

## Resultado

**Pendente — preencher após a execução (operador).**

- Data/executor: _a preencher_
- Passos 1–7: _saídas reais (colar as linhas relevantes de cada passo, sem segredos)_
- Desvios vs. plano: _a preencher_
- Rollback executado? _a preencher (chave revogada? arquivos removidos?)_
