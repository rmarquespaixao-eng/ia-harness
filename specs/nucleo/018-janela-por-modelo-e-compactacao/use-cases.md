# Use Cases: Janela de contexto por modelo e compactação (018)

Formato fully-dressed (Cockburn) + Gherkin. O operador implementa; o agente audita.

## CU-CTX-1 — Turno longo em modelo de janela grande

- **Identificador**: CU-CTX-1 · **Escopo**: motor do harness · **Nível**: subfunção
- **Ator primário**: host (biblioteca embarcada)
- **Partes interessadas e interesses**: operador (não quer teto artificial); usuário (conversa longa continua)
- **Pré-condições**: perfil com `Capabilities.MaxContextTokens > 0`; contexto acumulado ultrapassa o gatilho
- **Gatilho**: chamada de `Run`/`ResolveConfirmation`
- **Garantia de sucesso**: o contexto enviado ao modelo cabe no orçamento derivado do modelo e o turno conclui
- **Garantia mínima**: nenhuma mensagem de sistema nem o último bloco é removido; o turno nunca falha por "context window exceeded"
- **Mapeamento técnico**: `contextBudget` → `applyWindow` por iteração
- **Fluxo principal de sucesso**:
  1. O motor resolve o perfil do modelo.
  2. O orçamento = `MaxContextTokens − MaxOutputTokens − SafetyMargin`.
  3. O motor estima `system + tools + mensagens`.
  4. Se `estimate ≥ budget × ratio`, agrupa em blocos e remove os mais antigos (preservando o último e os pares de tool).
  5. Com `StrategySummarize` + `Summarizer`, resume o removido e injeta como sistema com proveniência.
  6. Envia o contexto compactado ao modelo; registra `TurnResult.Compaction`.
- **Fluxos alternativos e de exceção**:
  - `2a` Capacidade ausente (0): usa `Context.MaxTokens`; se ≤0, janela desligada.
  - `4a` Bloco único maior que o orçamento: mantém (não entra em loop) e segue.
  - `5a` `Summarizer` falha ou devolve vazio: segue com truncamento, log WARN.
  - `5b` Já sumarizou neste turno: trunca sem chamar de novo.
- **Regras de negócio**: RN-1 proveniência explícita do resumo; RN-2 pares de tool nunca separados; RN-3 ≤1 resumo/turno.
- **Dados e variações**: mensagens com mídia contam pelo payload efetivo.
- **Critérios de aceite (Gherkin)**:
```gherkin
Cenário: janela real do modelo
  Dado um perfil com MaxContextTokens=1_000_000 e MaxOutputTokens=8192
  Quando o contexto estimado passa do gatilho
  Então o motor compacta e o turno segue sem erro de janela

Cenário: par de tool preservado
  Dado um histórico onde a mensagem mais recente é um resultado de tool
  Quando a compactação dispara
  Então a chamada correspondente permanece junto do resultado
```
- **Rastreabilidade**: FR-CTX-001..006, 010.

## CU-CTX-2 — Modelo de janela pequena no mesmo host

- **Identificador**: CU-CTX-2 · **Escopo**: motor · **Nível**: subfunção
- **Ator primário**: host
- **Pré-condições**: dois perfis com capacidades diferentes no mesmo `Config`
- **Gatilho**: turno em cada perfil
- **Garantia de sucesso**: cada turno usa o orçamento do seu perfil
- **Fluxo principal**:
  1. Turno A no modelo de 1M: orçamento ~1M.
  2. Turno B no modelo de 100k: orçamento ~100k, compacta antes.
- **Critérios (Gherkin)**:
```gherkin
Cenário: orçamento por perfil
  Dado dois perfis com janelas diferentes
  Quando cada um roda um turno longo
  Então cada turno compacta conforme a sua própria janela
```
- **Rastreabilidade**: FR-CTX-001, 002.

## CU-CTX-3 — Reavaliação dentro do turno

- **Identificador**: CU-CTX-3 · **Escopo**: motor · **Nível**: subfunção
- **Ator primário**: host
- **Pré-condições**: turno com várias chamadas de tool
- **Garantia de sucesso**: o contexto não estoura no meio do turno
- **Fluxo principal**: cada resultado de tool é anexado; antes da próxima chamada ao modelo a janela é reavaliada.
- **Rastreabilidade**: FR-CTX-007.

## Requisitos especiais (NFR)

- Determinismo: nenhum teste do núcleo toca rede; `Summarizer` e `Provider` por fakes.
- Observabilidade: `CompactionInfo` com tokens antes/depois e mensagens removidas.
- Sem breaking change: `Handler`/`Config` permanecem compatíveis (campos aditivos).
