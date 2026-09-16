# Feature Specification: Streaming de argumentos de tool e raciocínio

**Feature Branch**: `nucleo/012-streaming-deltas` · **Status**: Aprovado e implementado

## Problema
A UI só sabia da tool quando ela já ia executar; raciocínio de modelos de reasoning era descartado. A comunidade streama argumentos parciais e o thinking.

## Requisitos (EARS)
- **FR-STR-001**: O harness MUST expor a porta opcional `StreamingProvider` (`ChatStream(ctx, req, sink)`) e `StreamSink` (texto, raciocínio, fragmentos de args), **sem** quebrar `Provider`.
- **FR-STR-002**: O `Handler` MUST ganhar `ReasoningDelta` e `ToolCallDelta` (com no-op em `NopHandler`).
- **FR-STR-003**: Os adapters MUST emitir: args de tool fragmentados (OpenAI `tool_calls[].function.arguments`, Anthropic `input_json_delta`, Responses `function_call_arguments.delta`) e raciocínio quando o provedor expõe (`reasoning_content`, `thinking_delta`, `reasoning_summary_text.delta`).
- **FR-STR-004**: Sem `StreamingProvider`, o engine MUST cair em `Provider.Chat` com sink de texto (comportamento anterior).

## Critérios (Gherkin)
```gherkin
Cenário: provider streaming
  Dado um provider que implementa ChatStream
  Quando o turno roda
  Então o Handler recebe ReasoningDelta e ToolCallDelta
  E o texto continua em TextDelta
```

## Não-objetivos
- Persistir raciocínio no histórico (é efêmero).
- Streaming de content blocks multimodais.
