# ADR 0023 — Janela de contexto por modelo e compactação automática

**Status**: Accepted
**Data**: 2026-09-16
**Feature**: `specs/nucleo/018-janela-por-modelo-e-compactacao`
**Supersede**: ADR 0005 (janela de contexto) nas partes de orçamento global, gatilho e observabilidade do corte.

## Contexto

O ADR 0005 fixou a janela por um `ContextPolicy.MaxTokens` **único e global** e um truncamento default. Na prática isso criou dois problemas: (1) `Capabilities.MaxContextTokens` do perfil **nunca era lido**, então um modelo de janela grande (ex.: 1M) ficava preso ao teto configurado pelo host; (2) a janela era aplicada **uma vez por turno, antes de resolver o perfil**, e o histórico cresce a cada iteração de tool — um turno longo podia estourar o modelo no meio. Além disso a remoção era mensagem a mensagem, podendo **separar uma chamada de tool do seu resultado** (estado inválido para o provedor), e o corte não tinha evento/metadado tipado (apesar de o ADR 0005 citá-lo).

## Decisão

1. **Orçamento por modelo**: `contextBudget(profile)` usa `Capabilities.MaxContextTokens − MaxOutputTokens − SafetyMargin` quando o perfil declara a janela; `ContextPolicy.MaxTokens` vira **fallback** (comportamento legado) e `budget ≤ 0` desliga a janela. Fim do teto artificial.
2. **Contagem completa**: o orçamento conta `system prompt` + definições de tools + mensagens (`estimateTextTokens`/`estimateToolsTokens`), com `Tokenizer` injetado ou bytes/token.
3. **Janela por chamada de modelo**: o loop aplica `applyWindow` **a cada iteração** (o histórico acumula resultados de tool), com gatilho `CompactAtRatio` (default 0,8) quando o orçamento vem do modelo e 1,0 no fallback legado.
4. **Remoção pairing-aware**: o histórico é agrupado em blocos (assistant com `PartToolCall` + resultados subsequentes); remove-se o bloco não-sistema mais antigo preservando o último — chamada e resultado nunca se separam.
5. **Compactação**: com `StrategySummarize` + `Summarizer`, o removido vira um resumo com proveniência (`injectSummary`), **≤1×/turno** (guarda no loop); falha/vazio cai para truncamento. Sem `Summarizer`, trunca (corrige a divergência do ADR 0005, que prometia erro de config). A porta opcional `ModelSummarizer` recebe o alias do modelo do turno, permitindo resumir com o **mesmo modelo do perfil** (precedência sobre `Summarize`).
6. **Observabilidade aditiva**: `TurnResult.Compaction *CompactionInfo` e a interface opcional `CompactionHandler` (type assertion — não quebra `Handler` existentes) publicam tokens antes/depois, mensagens removidas e se houve resumo.

## Alternativas consideradas

- **Subir o default de `MaxTokens`**: continua artificial e não per-modelo. Rejeitada.
- **Um `Harness` por perfil**: resolve orçamento, mas multiplica portas/objetos; o motor pode ler a capacidade do perfil resolvido. Rejeitada.
- **Janela só no início do turno**: não cobre loop longo de tools. Rejeitada.
- **Novo método em `Handler`**: quebra implementadores do host (MAJOR). Interface opcional por type assertion. Rejeitada a quebra.
- **Reescrever o histórico persistido com o resumo**: fora do núcleo (decisão do host). Adiada.

## Consequências

- O turno usa a janela real do modelo; conversas longas compactam em vez de morrer/estourar. `buildMessages` legado (ratio 1, sem system/tools) permanece para compatibilidade e suíte histórica.
- Custo: quando compacta, um resumo consome tokens do orçamento (o host o audita como chamada de modelo). A compactação é do **contexto enviado**, não do histórico persistido.
- Release MINOR (campos aditivos + mudança de comportamento documentada); o consumidor (financeiro) precisa alinhar o perfil e injetar o `Summarizer` (feature espelho).
- Testes: `window_test.go` (orçamento por modelo, gatilho, pairing, contagens) e `harness/compaction_test.go` (resumo + `CompactionInfo` + evento).
