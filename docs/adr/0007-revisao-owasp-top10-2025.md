# ADR 0007 — Revisão OWASP Top 10:2025 do núcleo do harness

**Status**: Accepted
**Data**: 2026-09-15

## Contexto

Portão de segurança do SDD (constitution §8) antes do release inicial `v0.1.0`. Escopo auditado: todo o módulo `github.com/rmarquespaixao-eng/ia-harness` — loop do agente, política, cliente MCP, adaptadores de provedor, redação, auditoria, memória e CLI de desenvolvimento. O harness é uma **biblioteca embutida**: não expõe HTTP público, não autentica usuário final, não abre banco. Riscos relevantes concentram-se em injeção via conteúdo de tool, vazamento de credencial/PII e condições excepcionais.

## Cobertura (OWASP Top 10:2025)

| Categoria | Status | Evidência |
|---|---|---|
| A01 Broken Access Control | verificado | Isolamento por `user_id` em `Run`/`loadOrCreateSession` (`harness/harness.go`), `adapters/memory/inmem` e `adapters/session/memory`; retriever por usuário. O host é dono da autenticação (constitution §9) |
| A02 Security Misconfiguration | verificado | Default deny de tools (`harness/policy.go`); redação 16 KiB default; `cmd/harnessctl` exige arquivo de credencial `0600` (`cmd/harnessctl/main.go:341`); TLS padrão do `net/http` |
| A03 Software Supply Chain Failures | verificado (SEC-02 corrigido) | Dependências pinadas + `go.sum` + `tool` directive; `govulncheck` sem vulnerabilidades chamadas; `scripts/gen-ts.sh` passou a pinar o pacote npm (`json-schema-to-typescript@15`) |
| A04 Cryptographic Failures | verificado | Sem criptografia no núcleo; credenciais transitórias resolvidas por request (`CredentialProvider`), nunca persistidas/logadas |
| A05 Injection | verificado | Argumentos de tool validados por JSON Schema (`internal/platform/schema`); política decidida fora do modelo (FR-019); sem SQL/shell (o único `exec.Command` é o `contractgen` com args fixos); saída redigida por allowlist |
| A06 Insecure Design | verificado | Default deny, confirmação de destrutivas, orçamento de turno, fallback sem repetir tool, eventos redigidos — decisões com ADR 0002/0003/0004 |
| A07 Authentication Failures | verificado (SEC-01 corrigido) | `x-api-key` injetado por request via `RoundTripper` (`adapters/mcpclient/client.go`); redirect cross-host agora bloqueado em todos os clientes (`internal/platform/httpx`) |
| A08 Software or Data Integrity Failures | verificado | Contratos gerados de JSON Schema com gate de idempotência (`cmd/contractgen` + `contracts/conformance_test.go`); `RestoreSession` valida estado; eventos de auditoria validados contra schema |
| A09 Security Logging/Alerting Failures | verificado | `AuditEvent` por chamada com payload redigido/truncado e `denied` sem payload (`harness/audit_emit.go`, `harness/redact.go`); erros de provedor limitados a 300 runas e sem headers; falha de sink vira `ErrorEvent` |
| A10 Mishandling of Exceptional Conditions | verificado | Falha MCP vira resultado de erro ao modelo (FR-011); cancelamento persiste sessão com `context.WithoutCancel` (FR-010); save obrigatório antes da resposta; `errors.Is/As` preservados |

## Achados confirmados

### SEC-01 — [Média] Vazamento de credencial em redirect cross-host — CORRIGIDO
- **Categoria**: A07 Authentication Failures
- **Evidência**: `adapters/mcpclient/client.go` (`authTransport` injeta `x-api-key`) e `provider/{openai,anthropic}/chat.go` (`Config.Headers` customizados) usavam o `http.Client` sem `CheckRedirect`. O `net/http` remove headers sensíveis conhecidos (`Authorization`) em redirect cross-host, mas **não** headers customizados como `x-api-key`.
- **Cenário**: endpoint MCP/provedor comprometido ou interceptado responde `302` para host de terceiro; a credencial seria enviada a esse host.
- **Remediação**: `internal/platform/httpx.NoCrossHostRedirect` recusa redirect para host diferente, preservando `CheckRedirect` do cliente injetado; aplicado nos três clientes + teste regressivo (`internal/platform/httpx/safeclient_test.go`).

### SEC-02 — [Baixa] Pacote npm não pinado na geração de tipos TS — CORRIGIDO
- **Categoria**: A03 Software Supply Chain Failures
- **Evidência**: `scripts/gen-ts.sh` executava `npx -y json-schema-to-typescript` (versão flutuante, execução arbitrária de pacote de terceiro).
- **Remediação**: versão pinada (`@15`) no script; script é dev-only e não roda no gate.

## Riscos potenciais (não confirmados)

- **Handler panic**: exceção em callback do host (`Handler`) não é recuperada pelo loop; é código do host — documentado, sem exploração demonstrada.
- **Fallback pós-stream parcial**: texto já emitido por tentativa que falhou permanece visível (documentado em `provider_route.go`); afeta UX, não segurança.
- **Store em memória** (`adapters/session/memory`) é dev/teste; o host real persiste com seus controles.

## Consequências

- `govulncheck` limpo no gate (`make verify`); SDK MCP pinado em `v1.8.0` (linha do financeiro).
- Nenhuma mudança de contrato público; `internal/platform/httpx` nasce como utilitário de plataforma (constitution §3).
- Revisão registrada como ADR; próximas features que tocarem upload, novos provedores ou persistência real do host devem reabrir o escopo desta revisão.
