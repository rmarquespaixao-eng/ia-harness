# ADR 0002 — Fallback por chamada de modelo e concorrência serializada por sessão

**Status**: Accepted
**Data**: 2026-09-15

## Contexto

O harness suporta perfis multi-provedor com fallback (FR-012–FR-014) e tools com efeitos colaterais reais — o primeiro host inclui transferências e exclusões. Dois riscos da spec dependem desta decisão: R1 (duplicação de efeito quando o fallback reexecuta trabalho já feito) e R4 parcial (turnos que disparam múltiplas tool calls). A pesquisa R12 resolve a semântica de fallback; D-10 fixa a concorrência. Sem uma regra explícita, um fallback ingênuo repetiria a tool, e paralelizar tool calls de um mesmo turno quebraria a ordem e poderia intercalar operações destrutivas.

A restrição de produto é single-tenant por instância, sessões curtas e uma execução de turno por sessão (plan §Scale/Scope); o consumidor (financeiro) recebe eventos por um canal síncrono (SSE). O orçamento do turno limita iterações, o que também restringe quantas tool calls um único turno pode emitir.

## Decisão

1. **Fallback por chamada de modelo, nunca por turno**. Se o provedor falha antes de qualquer tool executar, o próximo modelo do perfil recebe a mesma conversa; se falha depois de tools executadas, o próximo modelo recebe os resultados já registrados e a tool **não roda de novo** (FR-014). Tools marcadas `idempotent: true` pelo host são a única exceção e podem repetir em retry explícito; o default é `false`. Confirmação pendente sobrevive ao fallback, porque o estado é do turno, não do modelo. O evento registra modelo efetivo e motivo da troca (FR-013).
2. **Uma sessão = um turno por vez**. A sessão rejeita turno concorrente enquanto outro está ativo (ou pending de confirmação); o estado é persistido pelo `SessionStore`.
3. **Tool calls do turno executam em sequência, na ordem emitida pelo modelo**. Tool destrutiva ou sujeita a confirmação nunca roda em paralelo com outra; nenhuma tool roda antes da validação de argumentos e da decisão de política.
4. **Eventos síncronos no `Handler`**. O loop entrega cada evento (`TextDelta`, `ToolCallStarted/Finished`, `Progress`, `ConfirmationRequired`, `Error`, `TurnFinished`) na ordem em que ocorrem, sem fila interna; cancelamento propaga pelo `context` do host.

## Alternativas consideradas

| Alternativa | Motivo da rejeição |
|---|---|
| Reexecutar o turno inteiro no fallback | Duplicaria tool calls já executadas (R1): efeito colateral repetido em transferência/exclusão. |
| Execução paralela de tool calls do mesmo turno | Ordem indefinida, corrida entre tools destrutivas e auditoria ambígua; ganho de latência não compensa o risco. |
| Fila assíncrona de eventos | Complexidade de ordenação, backpressure e retomada sem caso de uso; o host já consome eventos por SSE. |
| Confirmação assíncrona como parte do fallback | Mistura duas máquinas de estado (decisão humana × troca de modelo); confirmação é ortogonal e sobrevive à troca. |

## Consequências

**Positivas**
- Nenhum efeito colateral duplicado por troca de modelo — fallback seguro para tools destrutivas.
- Ordem de execução determinística e auditável: a trilha registra a sequência real das tool calls.
- Modelo mental simples para o host: sessão é serializada; eventos chegam em ordem, sem coordenação extra.
- Cancelamento e confirmação pendente seguem um único caminho de estado persistido.
- Eventos síncronos simplificam a integração e o teste do host: sem ordenação fora de banda nem polling.

**Negativas / trade-offs**
- Sem paralelismo de tool calls, um turno com várias tools lentas soma as latências; aceito no v1 por segurança e simplicidade.
- O `Handler` roda no caminho do turno: host que bloqueie (ex.: UI lenta) atrasa a execução; o contrato orienta o host a enfileirar do seu lado.
- O fallback é por chamada: falha de nível de turno (ex.: orçamento estourado) não é recuperada por troca de modelo — fica com o host.
- Sessões longas com muitos turnos permanecem serializadas; escalar concorrência exige múltiplas sessões, não paralelismo interno.
