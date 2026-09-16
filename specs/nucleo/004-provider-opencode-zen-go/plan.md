# Implementation Plan: OpenCode Zen/Go (004)

**Branch**: `nucleo/004-provider-opencode-zen-go` | **Date**: 2026-09-16 | **Spec**: [spec.md](./spec.md)

**Input**: `specs/nucleo/004-provider-opencode-zen-go/` (spec + research)

## Summary

Fechar a lacuna de provedor para OpenCode Zen/Go: (a) confirmar/documentar as famílias `/chat/completions` e `/messages` como **config-only** com os adapters existentes; (b) criar o adaptador **`adapters/provider/openai_responses`** para a Responses API (`/responses`), com streaming SSE, tool calling e usage; (c) permitir `User-Agent` e `x-opencode-session` por request. Decisão em **ADR 0011**.

## Technical Context

**Language**: Go 1.27 · **Dependencies**: nenhuma nova (parser SSE próprio, como os outros adapters) · **Storage**: n/a · **Testing**: `httptest` (sem rede) · **Constraints**: sem `net/http` na API pública; taxonomia de erro estável; redação invariável · **Scale**: 1 adapter novo.

## Constitution Check

| § | Requisito | Situação |
|---|---|---|
| §2 | Dependências mínimas; contrato gerado sem diff | OK (sem dep nova; sem mudança de schema) |
| §3 | Adaptador importa o núcleo; sem `net/http` público | **Aplica** |
| §4 | Credencial só por `CredentialProvider`; redação | **Aplica** — FR-ZG-005 |
| §5 | Log JSON, `trace_id`, erro não engolido | **Aplica** |
| §6 | API pública estável; eventos versionáveis | OK — adição por DI |
| §7 | Testes sem rede; `make verify` | **Aplica** |
| §8 | SDD + ADR | **Aplica** — ADR 0011 |

*Pós-design*: sem violação.

## Project Structure

```text
adapters/provider/openai_responses/   # NOVO: adapter da Responses API
  chat.go            # New/Chat/authorize/logCall (espelha provider/openai)
  stream.go          # parser SSE: output_text.delta, function_call_arguments.delta, completed.usage
  wire.go            # request (input/tools/store=false) e mapeamento item↔canônico
  *_test.go          # httptest do SSE, tool call, usage ausente, stream cortado, eventos desconhecidos
adapters/provider/openai/chat.go      # (inalterado) usado para /chat/completions do Zen/Go
adapters/provider/anthropic/chat.go   # (inalterado) usado para /messages do Zen/Go
examples/financeiro/di.go             # wiring de exemplo: 3 perfis Zen/Go (chat, messages, responses)
README.md · docs/features/nucleo-harness.md   # seção "Provedores: OpenCode Zen/Go" + tabela
docs/runbooks/AAAA-MM-DD-smoke-zen-go.md      # smoke real (operador)
docs/adr/0011-adapter-openai-responses-e-opencode-zen-go.md
```

## Design

### 1. Config-only (famílias já cobertas)
Documentar e exemplificar:
- Chat Completions: `openai.New(Config{BaseURL:"https://opencode.ai/zen/go/v1", CredentialRef:"env:OPENCODE_GO_API_KEY"})`.
- Messages: `anthropic.New(Config{BaseURL:"https://opencode.ai/zen/go", CredentialRef:"env:OPENCODE_GO_API_KEY"})` (adapter acrescenta `/v1/messages`).
Nenhuma mudança de código.

### 2. `openai_responses` (código novo)
- `Config{BaseURL, CredentialRef, DefaultModel, Headers, SessionHeader, Timeout}`, `Deps{Credentials, HTTPClient, Clock, Logger}` — espelha `provider/openai`.
- `Chat(ctx, req, onText)`: `POST {BaseURL}/responses` com `Authorization: Bearer` (via `CredentialProvider`), `Accept: text/event-stream`, `store:false`, `tools` em formato de função, `input` derivado do histórico canônico.
- `stream.go`: parser SSE tolerante; `response.output_text.delta`→texto; `function_call` (id/name) + `function_call_arguments.delta`→args; `response.completed.usage`→`Usage` reportado; ausência→`estimated=true`; EOF antes de `response.completed`→`provider_stream`/`io.ErrUnexpectedEOF`.
- Reusa a taxonomia de erro e o `httpx.NoCrossHostRedirect` (ADR 0007).

### 3. Sessão do cliente
- `SessionHeader` (ex.: `x-opencode-session`) preenchido com o `SessionID` do turno; `Headers` fixos carregam `User-Agent`. Sem `SessionHeader`/id, header omitido.

### 4. Testes
- `httptest` simulando SSE completo, tool call fragmentada, usage ausente, stream cortado, evento desconhecido, 429/timeout.
- Extensão do wiring de exemplo sem rede (só validação de `New`).

## Complexity Tracking

| Decisão | Alternativa | Por quê |
|---|---|---|
| Adapter dedicado `openai_responses` | `WireAPI` no `provider/openai` | protocolos e parsers distintos; separação clara |
| `store=false` | API stateful | histórico é do host; sem acoplamento |
| Header de sessão no adapter | sessão no núcleo | sem `net/http` público; o id já existe no turno |

## Gate

`make verify` verde + suíte existente intacta + smoke real com `gpt-5.6-luna` (operador). **Sem código antes da aprovação.**
