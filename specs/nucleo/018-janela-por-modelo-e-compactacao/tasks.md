# Tasks: Janela de contexto por modelo e compactação (018)

**Input**: `specs/nucleo/018-janela-por-modelo-e-compactacao/` (spec · plan · use-cases)
**Pré-requisitos**: plan aprovado. Cada task fecha com teste AAA e `make verify` verde.

## Fase 1 — Contratos aditivos

- [x] T1801 `internal/core/config.go`: `ContextPolicy.CompactAtRatio`, `ContextPolicy.SafetyMargin`, `CompactionInfo` e `TurnResult.Compaction` — FR-CTX-001/009
- [x] T1802 `internal/core/events.go`: `CompactionEvent` + interface opcional `CompactionHandler` — FR-CTX-009
- [x] T1803 `harness/alias.go`: aliases de `CompactionInfo`/`CompactionEvent`/`CompactionHandler` — API pública

## Fase 2 — Orçamento e contagem

- [x] T1804 `window.go`: `contextBudget(profile)` (capacidade do modelo vs. fallback global) — FR-CTX-001/002
- [x] T1805 `window.go`: `estimateTextTokens`/`estimateToolsTokens` e `applyWindow` contando system+tools+mensagens — FR-CTX-003
- [x] T1806 `window.go`: agrupamento pairing-aware (blocos) na remoção — FR-CTX-006/010

## Fase 3 — Compactação por chamada

- [x] T1807 `loop.go`: aplicar `applyWindow` a cada iteração (contexto cresce com tools), com `ratio` do gatilho — FR-CTX-004/007
- [x] T1808 `window.go`/`loop.go`: resumo ≤1×/turno com fallback para truncamento — FR-CTX-005/008
- [x] T1809 `loop.go`: preencher `TurnResult.Compaction` e emitir `CompactionEvent` quando o handler implementar a interface — FR-CTX-009

## Fase 4 — Testes e fechamento

- [x] T1810 `window_test.go`: orçamento por perfil, gatilho 80%, pairing-aware, contagem de system/tools — SC-018
- [x] T1811 `loop`/`harness` teste de turno: loop de tools reavalia a janela; `CompactionInfo` no resultado e evento emitido — CU-CTX-3
- [x] T1812 `make verify` verde + ADR 0023 + CHANGELOG + docs de feature; preparar release (operador) — Constitution §7
- [x] T1813 porta opcional `ModelSummarizer` (alias do modelo para o resumo) + precedência e teste — FR-CTX-011

## Dependências

- T1801→T1802→T1803; T1804/T1805/T1806→T1807→T1808/T1809; tudo→T1810–T1812.
- Depende de 010 (`Tokenizer`) e 002 (mídia) — já entregues.

## Rastreabilidade — tasks × requisitos

| Requisito | Tasks |
|---|---|
| FR-CTX-001/002 | T1801, T1804 |
| FR-CTX-003 | T1805 |
| FR-CTX-004 | T1807 |
| FR-CTX-005/008 | T1808 |
| FR-CTX-006/010 | T1806 |
| FR-CTX-007 | T1807 |
| FR-CTX-009 | T1802, T1803, T1809 |
| CU-CTX-1/2/3 | T1810, T1811 |
