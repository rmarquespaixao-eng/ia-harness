# Feature Specification: Tracer plugável (spans GenAI / OpenTelemetry)

**Feature Branch**: `nucleo/011-tracer-otel`

**Status**: Aprovado e implementado

**Input**: lacuna P0 — o padrão da comunidade é OTel GenAI (spans inference/execute_tool/invoke_agent); o harness só tinha `slog` + `trace_id`.

## 1. Problema

Observabilidade de agente exige traces por operação (turno, chamada de modelo, tool) com atributos `gen_ai.*`, consumíveis por Langfuse/MLflow/Phoenix. Adicionar o SDK OTel ao núcleo traria dependência e acoplamento; a solução é uma **porta** de tracer que o host liga ao OTel.

## 2. Requisitos (EARS)

- **FR-TRACE-001**: O harness MUST expor as portas `Tracer`/`Span` com `StartTurn`, `StartModel` e `StartTool`, recebendo atributos (ids de sessão/usuário/agente, provider/modelo, tool/call).
- **FR-TRACE-002**: O turno MUST emitir um span de agente; cada chamada de modelo, um span de inferência; cada execução de tool, um span de tool; `Span.End(err)` sinaliza sucesso/erro.
- **FR-TRACE-003**: Sem `Tracer` injetado, MUST usar um tracer no-op (comportamento anterior, sem custo).
- **FR-TRACE-004**: O núcleo MUST NOT adicionar dependência de OTel (o host pluga o SDK, mapeando os atributos para `gen_ai.*`).
- **FR-TRACE-005**: O trace MUST continuar correlacionado pelo `trace_id` no contexto (constitution §5).

## 3. Critérios de aceite (Gherkin)

```gherkin
Cenário: turno com tool emite spans
  Dado um Tracer injetado
  Quando um turno com tool executa
  Então são abertos spans de turno, modelo e tool

Cenário: sem tracer
  Dado nenhum Tracer
  Quando um turno executa
  Então usa o tracer no-op sem custo
```

## 4. Não-objetivos

- Implementar exportação OTel no núcleo (o host liga o SDK).
- Métricas (além de spans) no v1.
- Captura de conteúdo (prompts/respostas) por padrão — fica no host, respeitando a redação.
