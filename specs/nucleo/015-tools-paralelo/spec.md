# Feature Specification: Execução de tools em paralelo

**Feature Branch**: `nucleo/015-tools-paralelo` · **Status**: Aprovado e implementado

## Problema
Tool calls independentes do mesmo turno executavam em sequência, aumentando a latência.

## Requisitos (EARS)
- **FR-PAR-001**: WHEN `Config.ParallelTools` é verdadeiro e o turno tem >1 tool call, o harness SHALL pré-resolver, validar e autorizar **todas**; se qualquer uma for desconhecida, inválida, negada ou exigir confirmação, SHALL cair no caminho sequencial (nunca paraleliza decisão humana).
- **FR-PAR-002**: No lote seguro, as tools MUST executar concorrentemente e os eventos/resultados/histórico MUST permanecer **na ordem original das chamadas**.
- **FR-PAR-003**: Erro de transporte em uma execução vira `ErrorEvent` + resultado de erro (como no sequencial), sem derrubar o turno.
- **FR-PAR-004**: `Progress` pode ser emitido concorrentemente (o Handler precisa ser seguro); default desligado preserva o comportamento anterior.

## Critérios (Gherkin)
```gherkin
Cenário: duas tools independentes
  Dado Config.ParallelTools e duas tool calls permitidas
  Quando o turno roda
  Então as duas executam
  E os ToolResults saem na ordem das chamadas

Cenário: confirmação no lote
  Dado um lote com uma tool que exige confirmação
  Quando o turno roda
  Então o lote NÃO é paralelizado
```

## Não-objetivos
- Paralelizar tools destrutivas/confirmáveis (por design serial).
- Limite de concorrência configurável (futuro).
