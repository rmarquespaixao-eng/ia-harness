# ADR 0018 — Observabilidade por porta de tracer (OTel GenAI no host)

**Status**: Accepted
**Data**: 2026-09-16
**Feature**: `specs/nucleo/011-tracer-otel`

## Contexto

O padrão da comunidade para observabilidade de agentes é OpenTelemetry GenAI (spans `inference`, `execute_tool`, `invoke_agent`; atributos `gen_ai.*`), consumido por Langfuse/MLflow/Phoenix. O harness tinha só `slog` + `trace_id`. Adicionar o SDK OTel ao núcleo traria dependência e acoplamento.

## Decisão

Expor as portas `Tracer`/`Span` (`StartTurn`, `StartModel`, `StartTool`; `Span.End(err)`), com atributos mínimos (sessão/usuário/agente/modelo, provider/modelo, tool/call). O engine abre: um span de agente por turno (`runTurnTraced`), um de inferência por chamada de modelo e um de tool por execução. `Config.Tracer` é opcional; default no-op. O host implementa o tracer ligando ao OTel e mapeando os atributos para `gen_ai.*`; a captura de conteúdo fica no host (respeitando a redação). O `trace_id` continua no contexto.

## Alternativas consideradas

- **SDK OTel no núcleo**: dependência e exportação de responsabilidade do host. Rejeitada.
- **Emitir eventos de auditoria como "spans"**: mistura trilha com trace; a trilha já existe (`AuditSink`). Rejeitada.
- **Só `slog`**: não atende o padrão de traces. Rejeitada.

## Consequências

- Traces por operação prontos para OTel sem dependência no núcleo; no-op quando não injetado.
- Novo `Config.Tracer` (aditivo). Testes: `harness/tracer_test.go` (turno/modelo/tool).
