# Plan: Janela de contexto por modelo e compactação automática (018)

**Feature**: `specs/nucleo/018-janela-por-modelo-e-compactacao/` · **Status**: Aprovado

Constitution: **não viola** nenhuma seção. Muda o motor (`internal/engine`), aditivamente na API pública; JSON Schema de contrato não muda (nenhum campo de fio/persistência novo — `ContextPolicy` e `TurnResult` são tipos Go de uso interno/host, não persistidos pelo núcleo).

## Arquitetura

1. **Orçamento por modelo** (`internal/engine/window.go`): função pura `contextBudget(profile) (budget, fromModel)`. Quando `Capabilities.MaxContextTokens > 0`, `budget = MaxContextTokens − MaxOutputTokens − SafetyMargin` (`fromModel=true`); senão cai para `Context.MaxTokens` (legado, `fromModel=false`). `budget ≤ 0` e sem fallback ⇒ janela desligada.
2. **Contagem completa**: o orçamento passa a contar `system prompt` + definições de tools + mensagens (`estimateTextTokens`, `estimateToolsTokens`), com `Tokenizer` injetado ou bytes/token.
3. **Janela por chamada de modelo**: o loop deixa de montar o contexto janelado uma vez no início; a cada iteração, antes de `ChatRequest`, aplica `applyWindow` sobre o contexto corrente (que cresce com os resultados de tool). Disparo em `before ≥ budget × ratio` (`ratio = CompactAtRatio` quando `fromModel`, default 0,8; `1,0` no fallback legado).
4. **Remoção pairing-aware**: o histórico é agrupado em *blocos* (assistant com `PartToolCall` + resultados subsequentes). Remove-se o bloco não-sistema mais antigo, preservando o último bloco — nunca se separa chamada de resultado.
5. **Compactação**: com `StrategySummarize` + `Summarizer`, o conteúdo removido vira um resumo injetado como `RoleSystem` com proveniência (`injectSummary`), no máximo 1×/turno (`allowSummarize`); falha/vazio cai para truncamento.
6. **Observabilidade aditiva**: `TurnResult.Compaction *CompactionInfo{TokensBefore, TokensAfter, MessagesRemoved, Summarized}`; interface opcional `CompactionHandler{Compaction(ctx, CompactionEvent)}` detectada por type assertion (não quebra `Handler` existentes).

## Contratos (aditivos)

- `ContextPolicy.CompactAtRatio float64` (≤0 ⇒ 0,8 quando `fromModel`) e `ContextPolicy.SafetyMargin int`.
- `TurnResult.Compaction *CompactionInfo`; `CompactionInfo`, `CompactionEvent`, `CompactionHandler`; aliases em `harness/alias.go`.

## Alternativas consideradas

- **Só subir o default de `Context.MaxTokens`**: continua artificial e não per-modelo. Rejeitada.
- **Um harness por perfil de modelo**: resolve orçamento por perfil, mas multiplica objetos/portas e o registry do host já existe; o motor pode ler a capacidade do perfil resolvido. Rejeitada.
- **Janela só no início do turno**: não cobre loop longo de tools (cresce no meio). Rejeitada.
- **Novo método em `Handler`**: quebraria implementadores do host (semver MAJOR). Interface opcional por type assertion. Rejeitada a quebra.
- **Reescrever o histórico persistido com o resumo**: fora do núcleo (decisão do host). Adiada.

## Riscos

- Mudança de ordem no loop pode afetar testes que assumem janela única; mitigado rodando `make verify` e mantendo `buildMessages` com o comportamento legado (ratio=1) para a suíte existente.
- Custo do resumo: 1 chamada de modelo extra por turno quando compacta — observável na trilha (host).
