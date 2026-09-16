# Feature Specification: Janela de contexto por modelo e compactação automática

**Feature Branch**: `nucleo/018-janela-por-modelo-e-compactacao` · **Status**: Draft (aguardando portão)

**Input**: pedido do operador — "não quero limites artificiais: se o modelo suporta 1M, tem que ser o que o modelo suporta; e a compactação da conversa tem que acontecer como num agente (resumir o histórico antigo e continuar)".

## Problema

A janela de contexto do harness é **global e artificial** e **não compacta**:

1. O orçamento vem de `Config.Context.MaxTokens` (`internal/engine/window.go:47`), um valor único. `Capabilities.MaxContextTokens` do perfil **nunca é lido** pelo motor (`internal/core/config.go:63`) — um modelo de 1M fica preso ao teto do host.
2. A janela é aplicada **uma vez por turno e antes de resolver o perfil** (`loop.go:255` vs `resolveModel` em `:327`); o histórico cresce a cada iteração de tool-call sem re-janelamento, então um turno longo pode estourar o modelo no meio.
3. A estratégia `summarize` existe (`window.go:56`), mas só dispara **ao estourar 100%** do orçamento, sem headroom, e o financeiro nem injeta `Summarizer` — na prática só trunca.
4. A contagem ignora `system prompt` e definições de tools (`estimateWindowTokens` soma só mensagens), então o orçamento não representa o que é realmente enviado.

## Requisitos (EARS)

### Orçamento por modelo

- **FR-CTX-001**: WHEN um turno chama o modelo, o orçamento de entrada MUST ser derivado de `Capabilities.MaxContextTokens` do perfil resolvido, reservando `Capabilities.MaxOutputTokens` e uma margem de segurança.
- **FR-CTX-002**: WHEN `Capabilities.MaxContextTokens` é zero/desconhecido, o harness MUST manter o comportamento anterior (usar `Context.MaxTokens`; se este for ≤ 0, a janela fica desligada).
- **FR-CTX-003**: O cálculo do orçamento MUST contar `system prompt`, definições de tools e mensagens — não apenas o texto das mensagens.

### Gatilho e compactação

- **FR-CTX-004**: WHEN o contexto estimado atinge uma fração configurável do orçamento (default 0,8 = 80%), o harness MUST compactar **antes** de chamar o modelo, deixando headroom.
- **FR-CTX-005**: A compactação MUST substituir as mensagens elegíveis mais antigas por **uma** mensagem de resumo com proveniência explícita, preservando as mensagens de sistema e os turnos recentes.
- **FR-CTX-006**: A compactação MUST preservar a integridade de pares: uma chamada de tool e seu resultado MUST ser removidos ou mantidos **juntos**; um resultado de tool órfão MUST NOT ir ao modelo.
- **FR-CTX-007**: A compactação MUST ser reavaliada **a cada chamada de modelo** dentro do turno (não só uma vez), de modo que um loop longo de tools permaneça dentro do orçamento.
- **FR-CTX-008**: O resumo MUST ser produzido pela porta `Summarizer` no máximo **uma vez por turno**; falha, erro ou resumo vazio MUST cair para truncamento sem derrubar o turno.
- **FR-CTX-009**: A compactação MUST ser observável ao host (evento tipado e/ou metadado do `TurnResult`) com tokens antes/depois e quantidade de mensagens substituídas.
- **FR-CTX-010**: `StrategyTruncateOldest` MUST continuar disponível como fallback e MUST NOT quebrar pares de tool (FR-CTX-006).
- **FR-CTX-011**: WHEN o `Summarizer` implementa a interface opcional `ModelSummarizer`, o harness MUST passar o alias do modelo do turno, para o host resumir com o mesmo modelo do perfil.

## Critérios (Gherkin)

```gherkin
Cenário: modelo de janela grande não é limitado pelo host
  Dado um perfil com MaxContextTokens = 1_000_000 e sem Context.MaxTokens global
  Quando o contexto acumulado passa de 800_000 tokens
  Então o harness compacta o histórico antigo e o turno segue no modelo, sem erro de janela

Cenário: compactação preserva os turnos recentes e o resumo
  Dado um histórico com mensagens antigas e o par tool_call/tool_result recente
  Quando a compactação dispara
  Então as antigas viram uma mensagem de sistema "resumo de histórico: ..."
  E o par de tool recente permanece íntegro

Cenário: resumidor falha
  Dado um Summarizer que devolve erro
  Quando a compactação dispara
  Então o harness segue com truncamento e o turno conclui

Cenário: tool loop longo não estoura
  Dado um turno que encadeia várias tools e faz o histórico crescer
  Quando o orçamento é atingido no meio do turno
  Então a janela é reavaliada antes da próxima chamada de modelo
```

## Casos de borda

- Mensagem única maior que o orçamento (não há o que sumarizar): truncar/preservar o essencial sem loop infinito.
- `Summarizer` devolve resumo maior que o orçamento: injetar mesmo assim e seguir (sem recursão); registrar.
- Confirmação pendente: o par da tool pendente nunca é compactado.
- Mídia (imagens/documentos) no histórico: contar pelo payload efetivo (como hoje) e não quebrar o par.
- Modelo sem tool-calling com tools no turno: erro nomeado preservado (FR-015), antes da janela.

## Não-objetivos (v1)

- Reescrever o histórico **persistido** (a compactação é do contexto enviado ao modelo; persistir o resumo é decisão do host).
- Modelo de resumo dedicado/configurável — o host pode implementar com a porta `Summarizer` (no financeiro, o mesmo modelo do perfil).
- Tokenizer por modelo embutido (a porta `Tokenizer`, feature 010, já cobre).
- UI de visualização do resumo — pertence ao host (feature espelho no financeiro).

## Dependências e espelho

- Consumidor direto: `financeiro-api-v2` feature espelho (a especificar) — injeta `Summarizer` no mesmo modelo, remove o teto artificial `ASSISTANT_CONTEXT_MAX_TOKENS` e passa a ler `max_context_tokens` do perfil.
- Feature 010 (`Tokenizer`) e 002 (mídia) já entregam as peças usadas por esta janela.
