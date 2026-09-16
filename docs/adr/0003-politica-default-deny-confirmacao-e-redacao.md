# ADR 0003 — Política default deny, confirmação síncrona e redação por allowlist

**Status**: Accepted
**Data**: 2026-09-15

## Contexto

O harness executa tools com efeito real em nome do usuário e recebe conteúdo não confiável em dois pontos: argumentos e resultados de tools (que podem conter texto malicioso de terceiros) e o próprio stream do modelo. A constitution §4 exige default deny, validação de schema, redação e confirmação de ações sensíveis. A spec mapeia os riscos R2 (prompt injection elevando privilégio) e R3 (vazamento de segredo/PII na trilha), e as pesquisas R8/R9 resolvem, respectivamente, o desenho de confirmação e a política de redação.

Caso concreto do primeiro host: tools como transferência, exclusão e execução de recorrência exigem `confirm: true`; a trilha de auditoria é persistida pelo host e não pode carregar payload sensível nem argumentos de chamadas negadas.

## Decisão

1. **Default deny de tools**. O motor de política (puro, no pacote `harness`) parte de "negar" e libera por allowlist explícita do host, com marcação de read-only e de `idempotent`. A decisão é tomada **fora do modelo**: texto de argumentos, resultados de tools e conteúdo do stream nunca elevam privilégio; instrução embutida em resultado de tool é tratada como dado, não como diretiva (FR-016/FR-019, R2).
2. **Confirmação síncrona com pausa e retomada**. A confirmação é request/response no handler (`OnConfirmation(ctx, req) (Decision, error)`); a sessão persiste o estado `awaiting_confirmation` com o pedido pendente (tool, `call_id` e argumentos **redigidos**) via `SessionStore`. Se o processo cair antes da decisão, `Run` retoma no estado pendente ou `ResolveConfirmation` decide. Aprovar executa; negar devolve recusa ao modelo como resultado de tool (FR-018, R8).
3. **Redação por allowlist de chaves + truncamento**. Chaves sensíveis conhecidas (`api_key`, `token`, `secret`, `password`, `authorization`, `cookie`, `document_base64`…) são redigidas e cada campo é truncado em 16 KiB (alinhado ao financeiro) **antes** de log, `AuditSink` e telemetria; eventos de política `denied` não carregam payload (FR-024, SC-005, R9).

## Alternativas consideradas

| Alternativa | Motivo da rejeição |
|---|---|
| Allow implícito (negar só o que está em denylist) | Falha aberta: tool nova ou desconhecida executaria sem revisão; inaceitável com tools destrutivas. |
| Confirmação assíncrona por fila | Protocolo extra de correlação/retomada sem caso de uso; o host já tem canal síncrono (SSE) e o padrão `confirm: true`. |
| Confirmação por reenvio do turno | Perde o contexto da chamada pendente e pode duplicar efeitos. |
| Redação por regex de conteúdo nos valores | Falsos negativos em segredos fora do padrão e falso senso de cobertura; allowlist por chave é determinística e testável. |
| Trilha sem redação (só controle de acesso) | Viola FR-024/SC-005 e a política já provada no financeiro (spec 020) para a mesma trilha. |
| Política delegada ao modelo (juiz LLM) | Injeção de prompt passa a decidir permissão; contraria §4 e D-04. |

## Consequências

**Positivas**
- Falha fechada por padrão: nenhuma tool executa sem entrada explícita na política.
- Imune a prompt injection para decisão de permissão: o modelo propõe, a política decide.
- Confirmação e retomada usam o mesmo estado persistido de sessão, sobrevivendo a restart do processo.
- Trilha auditável e consistente entre cliente (harness) e servidor (financeiro), com testes de isca de segredo.

**Negativas / trade-offs**
- Toda tool nova exige entrada de política no host — atrito deliberado, mitigado por config tipada e testes de default deny.
- A confirmação bloqueia o turno enquanto aguarda decisão humana; sem timeout/política de expiração ela pode ficar pendente indefinidamente (host define o limite).
- Truncar em 16 KiB pode omitir detalhes de payloads grandes na trilha; o conteúdo completo permanece na origem, fora da auditoria.
- A allowlist de chaves precisa acompanhar novos campos sensíveis; campo esquecido só é pego por revisão e testes de isca.
