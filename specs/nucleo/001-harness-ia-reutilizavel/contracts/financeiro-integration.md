# Contract — Integração com o primeiro host (`financeiro-api-v2`)

**Feature**: `specs/nucleo/001-harness-ia-reutilizavel` | **Data**: 2026-09-15
Este contrato descreve como o `financeiro-api-v2` consome o harness. **O código de integração pertence ao repo do financeiro** e será especificado em feature própria lá (spec espelho); aqui ficam apenas os pontos de acoplamento e o que o harness garante.

## Ligação MCP

| Item | Valor |
|------|-------|
| Transporte | Streamable HTTP (SDK oficial), `mcpclient.New(mcpclient.Config{Name: "financeiro", Endpoint: "https://<host-financeiro>/mcp", ...})` injetado em `Config.Tools` (DI — ver `contracts/library-api.md`) |
| Autenticação | header `x-api-key` resolvido pelo `CredentialProvider` do host (ref `file:`/`env:`); chave é do usuário/agente — **nunca** na `Config` |
| Tools | 96 publicadas; catálogo descoberto por sessão (FR-005); nomes namespaceados `financeiro.<tool>` |
| Progresso | o financeiro emite `notifications/progress` nas tools longas (recálculo de fatura/transferência) → mapeado para `ProgressEvent` sem configuração extra |
| Erros | tool que devolve `isError` vira `ToolResultEvent.IsError=true` com conteúdo para o modelo; falha de transporte vira `ErrorEvent.Scope=mcp` (FR-011) |

Observações validadas no servidor (specs 020/021 do financeiro, ADRs 0031/0032): proteção anti-DNS-rebinding desativada atrás do Caddy; header `Host` público é esperado; tools destrutivas exigem `confirm: true` no corpo.

## Política sugerida (ponto de partida, decidida no host)

```yaml
policy:
  default: deny
  agents:
    financeiro-chat:
      mode: allow
      allow_tools: ["financeiro.*"]          # amplia explicitamente sobre o default deny
      confirm_tools:                          # espelha o confirm:true do domínio
        - "financeiro.excluir_transacao"
        - "financeiro.excluir_transacoes_em_lote"
        - "financeiro.transferir_transacoes"
        - "financeiro.pagar_fatura"
        - "financeiro.executar_recorrencia_agora"
        - "financeiro.*bulk*delete*"
      # overrides idempotency: consultas/queries são idempotentes; escrita não
```

O host mantém a lista real alinhada ao catálogo do financeiro; o harness **não** infere destrutividade do schema (data-model: `Tool.destructive` vem da política). Quando o host carrega essa política de arquivo, ela é validada pelo schema `contracts/config/*.json` (mesmos schemas geram os tipos do host).

## Uso e custo

- Preferencial: o adaptador do provedor reporta `usage` (OpenAI-compatible com `stream_options.include_usage`; Anthropic em `message_*`).
- Sem reporte: estimativa por volume × `Pricing` (R5), marcada `estimated=true`.
- Opcional (paridade com o financeiro): enviar `_meta["financeiro/usage"]` `{model, input_tokens, output_tokens, cost_micros, currency}` no `tools/call` quando o host habilitar — o servidor já valida por allowlist/bounds (ADR 0032) e rotula como reportado.

## Trilha e sessões no host (fora deste repo)

O financeiro implementa as portas:

| Porta | Implementação sugerida no financeiro |
|-------|--------------------------------------|
| `SessionStore` | tabela Postgres do host (histórico por usuário; retomada após restart) |
| `AuditSink` | tabela de trilha no padrão da `mcp_call_logs` (redigida/truncada, retenção) |
| `MemoryStore`/`Retriever` | feature própria do host (pgvector) — o harness só consome |
| `CredentialProvider` | leitura da chave do usuário/agente já autenticado na API |

## Fluxo fim a fim (financeiro)

1. Usuário autenticado na API chama o endpoint de assistente (feature espelho no financeiro).
2. A API monta `RunRequest{UserID, AgentID, Model, Input}` e `Handler` que traduz eventos para o SSE da UI.
3. `Run` descobre tools no `/mcp`, chama o modelo, valida e executa tools; destrutiva → `Confirmation` → UI pergunta → `ResolveConfirmation`.
4. Eventos viram streaming na UI; `AuditEvent`s vão para a trilha; sessão persiste no Postgres.
5. UI usa o mesmo padrão de progresso já existente (SSE/canal por usuário).

## Fora do escopo deste repo

- Rotas, autenticação, SSE e UI do chat no financeiro (feature espelho `financeiro-api-v2` / `financeiro-ui-v2`).
- Criação/gestão de chaves de API do financeiro (já existente na spec 020).
- Infra (Caddy/Terraform): sem mudança — o harness roda in-process na API.
