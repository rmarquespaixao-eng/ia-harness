# Contract — Eventos do turno e trilha de auditoria

**Feature**: `specs/nucleo/001-harness-ia-reutilizavel` | **Data**: 2026-09-15

## Handler (streaming para o host)

```go
// Handler recebe os eventos do turno em ordem. Todos os métodos são chamados
// de forma síncrona no fluxo do turno; o host decide transportar (SSE, WebSocket, UI).
// NopHandler (embeddable) implementa todos como no-op.
type Handler interface {
    TextDelta(ctx context.Context, ev TextDelta)                       // FR-009
    ToolCall(ctx context.Context, ev ToolCallEvent)                   // tool escolhida e validada
    ToolResult(ctx context.Context, ev ToolResultEvent)               // resultado/erro/negativa
    Progress(ctx context.Context, ev ProgressEvent)                   // tools longas (FR-009)
    Confirmation(ctx context.Context, ev ConfirmationEvent) (Decision, error) // FR-018
    Usage(ctx context.Context, ev UsageEvent)                         // fim de chamada de modelo
    Error(ctx context.Context, ev ErrorEvent)                         // erro recuperável/parcial
}

type NopHandler struct{} // embute todos os métodos no-op; host sobrescreve o que quiser
```

### Eventos

| Evento | Campos | Semântica |
|--------|--------|-----------|
| `TextDelta` | `SessionID`, `MessageID`, `Text` | fragmento incremental da resposta |
| `ToolCallEvent` | `SessionID`, `CallID`, `Tool`, `Namespace`, `ArgsRedacted`, `ArgsBytes` | argumentos prontos e **validados**; não inclui segredo |
| `ToolResultEvent` | `CallID`, `Tool`, `Status` (`ok`/`error`/`denied`), `IsError`, `Denied`, `Truncated`, `LatencyMS`, `ResultSummary` (redigido/truncado) | um por execução ou negativa |
| `ProgressEvent` | `CallID`, `Tool`, `Message`, `Progress`, `Total` | espelha `notifications/progress` do MCP |
| `ConfirmationEvent` | `SessionID`, `CallID`, `Tool`, `ArgsRedacted`, `Reason` | o handler retorna `Decision{Approve, Reason}`; erro no handler aborta o turno |
| `UsageEvent` | `Model`, `Provider`, `InputTokens`, `OutputTokens`, `Estimated`, `CostMicros`, `Currency` | fim de cada chamada de modelo |
| `ErrorEvent` | `Scope` (`model`/`tool`/`mcp`), `Tool`, `Message`, `Retryable` | erro reportado sem derrubar o turno quando recuperável |

**Ordem garantida por turno**: `TextDelta*` e/ou `ToolCall*` intercalados → `ToolResult` por chamada → `Usage` por chamada de modelo → (repetir enquanto houver tool calls) → fim do `Run` retorna `TurnResult`. `Confirmation` pausa a sequência até a decisão.

**Cancelamento**: `ctx` cancelado → nenhum evento novo é emitido; `Run` retorna `StopReason=cancelled`.

## Trilha de auditoria (`AuditSink`)

Um `AuditEvent` por **chamada de modelo** e por **execução/negativa de tool** — independente do streaming. O contrato **normativo** é `contracts/audit/audit_event.json` (JSON Schema → structs em `contracts/gen` via `cmd/contractgen`); o JSON abaixo é ilustrativo e o teste de contrato garante a conformidade do marshal.

```json
{
  "id": "0198f...",
  "kind": "tool_call",
  "trace_id": "req-abc123",
  "user_id": "3f2c...",
  "session_id": "0198a...",
  "agent_id": "financeiro-chat",
  "provider": "openrouter",
  "model": "default",
  "tool": "financeiro.listar_transacoes_conta",
  "tool_call_id": "call_9f3",
  "status": "ok",
  "input_tokens": 1543,
  "output_tokens": 87,
  "estimated": false,
  "cost_micros": 412,
  "currency": "USD",
  "latency_ms": 812,
  "args_redacted": "{\"accountId\":\"3f2c…\",\"month\":8,\"year\":2026}",
  "result_redacted": "{\"items\":[…truncado…]}",
  "occurred_at": "2026-09-15T20:31:07Z"
}
```

Regras:
- **Redação por allowlist de chaves sensíveis** (`api_key`, `token`, `secret`, `password`, `authorization`, `cookie`, `*_base64`) — valor vira `[REDACTED]`; detecção por chave, não por regex de conteúdo.
- **Truncamento** por campo em `redaction.max_field_bytes` (default 16 KiB); `truncated=true` no resultado quando corte ocorrer.
- `status=denied` ⇒ `args_redacted`/`result_redacted` **vazios** (FR-024).
- `estimated=true` ⇒ tokens/custo vieram de premissa (`Pricing`), não do provedor (FR-025).
- `error` é mensagem estável (código + causa curta); stack trace nunca entra no evento.
- `trace_id` vem de `harness.WithTraceID(ctx, id)` ou da chave de contexto equivalente; ausente → o núcleo gera e propaga (FR-026).

## Garantias do contrato de eventos (testáveis)

1. Nenhum evento contém credencial ou payload binário (teste com iscas — SC-005).
2. `ToolResultEvent.Denied=true` nunca foi precedido de chamada ao `ToolSource` (FR-017).
3. `ConfirmationEvent` sempre precede `ToolResultEvent` de tool confirmada; negar produz `Denied` com recusa textual ao modelo.
4. `ProgressEvent` respeita a ordem de `progress` crescente por `CallID` (ordem do servidor MCP).
5. `UsageEvent` é emitido exatamente uma vez por chamada de modelo, mesmo em fallback (com o modelo efetivo).
