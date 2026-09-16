# Feature Specification: Evals / regressão de agente

**Feature Branch**: `nucleo/016-evals` · **Status**: Aprovado e implementado

## Problema
Só havia testes unitários; faltava rodar cenários de agente (golden set) e verificar o resultado/eventos.

## Requisitos (EARS)
- **FR-EVAL-001**: O harness MUST oferecer `eval.Run(ctx, h, userID, agentID, cases)` com `Case{Name, Input, Model, OutputSchema, Check}`.
- **FR-EVAL-002**: Um `Recorder` MUST capturar os eventos (texto, tool calls/results, erros) para verificações.
- **FR-EVAL-003**: Scorers prontos MUST incluir `ContainsText` e `UsedTool`.
- **FR-EVAL-004**: O relatório MUST agregar pass/fail por caso; nenhum acesso à rede (usa o que o host injetou).

## Critérios (Gherkin)
```gherkin
Cenário: caso com tool
  Dado uma tool registrada e um roteiro que a chama
  Quando eval.Run roda o caso com UsedTool
  Então o caso passa

Cenário: verificação que falha
  Dado um caso cujo texto não contém o esperado
  Quando eval.Run roda
  Então o caso falha e o relatório marca Failed
```

## Não-objetivos
- LLM-as-judge embutido (o host pode implementar via `Provider`).
- Persistência de resultados/datasets.
