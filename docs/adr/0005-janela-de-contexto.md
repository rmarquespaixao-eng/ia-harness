# ADR 0005 — Janela de contexto: truncamento default e sumarização opt-in

**Status**: Accepted
**Data**: 2026-09-15

## Contexto

Sessões do harness acumulam mensagens (sistema, usuário, assistente, resultados de tool) e tools MCP podem devolver conteúdo volumoso; em algum ponto o prompt excede a janela do modelo (FR-021, D-06/R7). O núcleo não conhece o tokenizer exato de cada modelo e não deve embutir dependências de contagem por modelo.

Ao mesmo tempo, a decisão de corte precisa ser determinística, testável e auditável: o host tem de conseguir explicar por que uma mensagem antiga saiu do prompt. Falhar ao estourar a janela é a pior experiência (o turno inteiro morre por causa do histórico), e uma janela por número de mensagens ignora que uma única tool result pode ser maior que todo o restante.

## Decisão

1. **Orçamento de contexto por turno**: `ContextPolicy.max_tokens` limita o prompt montado a cada iteração; a estimativa usa a premissa `bytes_per_token` (default ~4), a mesma família de heurística do custo (ADR 0004) — sem tokenizer real no v1.

2. **Preservação obrigatória**: mensagens de sistema e o pedido atual (última mensagem do usuário) nunca são cortados; a poda considera apenas o miolo do histórico.

3. **Default `truncate_oldest`**: remove da mensagem mais antiga para a mais nova até caber no orçamento. Determinístico e testável; o corte é registrado em evento do turno e na trilha de auditoria (quantidade, ids, estimativa de tokens liberados), preservando a explicabilidade exigida por FR-021.

4. **Sumarização opt-in via porta `Summarizer`**: com `strategy=summarize`, o trecho removido é resumido antes da poda; guarda de **1 sumarização por turno** evita custo descontrolado; o default da porta é o próprio provedor configurado, mas o host pode injetar outro.

5. **Proveniência do resumo**: o resumo entra no prompt marcado como conteúdo sintetizado (origem, intervalo de mensagens, flag de resumo), para que o modelo e a auditoria saibam que não é fala literal do usuário; a sumarização é auditada como chamada de modelo, com custo reportado ou estimado.

6. **Escolha explícita do host**: `strategy=summarize` sem `Summarizer` disponível é erro nomeado na validação de `Config` (falha rápida), nunca fallback silencioso para truncamento.

## Alternativas consideradas

| Alternativa | Motivo da rejeição |
|---|---|
| Janela por número de mensagens | Ignora o tamanho real; uma única tool result grande estoura o contexto mesmo com poucas mensagens. |
| Truncar do mais novo (preservar o início) | Contraria o turno: a conversa recente e o pedido atual são mais relevantes que a abertura da sessão. |
| Tokenizer exato por modelo | Dependência pesada e desatualizada a cada release; a heurística rotulada basta para orçamento defensivo — porta `Tokenizer` fica como evolução. |
| Falhar ao estourar | Péssima experiência: o turno morre por causa do histórico, não do pedido atual; empurra para o host o que o núcleo pode resolver. |
| Sumarizar sempre (default) | Custa tokens, altera conteúdo e adiciona latência em todo turno; deve ser decisão explícita do host. |

## Consequências

**Positivas**: o turno não falha por excesso de histórico; o comportamento é determinístico e coberto por teste de unidade; corte e resumo ficam auditáveis com proveniência; a heurística é a mesma do custo, evitando premissas divergentes; o host escolhe o trade-off custo × fidelidade.

**Negativas/trade-offs**: truncar descarta informação antiga de forma irreversível no prompt (mitigado pela auditoria do corte e pelo histórico persistido na sessão); a estimativa pode cortar cedo ou tarde demais conforme idioma/JSON; sumarizar consome tokens do orçamento do turno e pode introduzir distorção — por isso é opt-in com guarda de 1×/turno; `max_tokens` mal configurado pelo host só se manifesta na qualidade da resposta, não em erro.
