# ADR 0011 — Adapter OpenAI Responses + OpenCode Zen/Go como provedor

**Status**: Accepted
**Data**: 2026-09-16
**Feature**: `specs/nucleo/004-provider-opencode-zen-go`
**Relaciona-se com**: ADR 0001 (MCP/canonical), ADR 0007 (revisão OWASP), features 002 (multimodal) e 003 (organização).

## Contexto

O OpenCode Zen (`opencode.ai/zen/v1`) e o Go (`opencode.ai/zen/go/v1`) expõem os modelos em três famílias de fio: OpenAI Chat Completions, Anthropic Messages e OpenAI **Responses** (`/responses`), que é onde vivem os modelos GPT (incl. `gpt-5.6-luna`, usado pelo operador) e Grok/Muse. O harness cobre Chat Completions (`adapters/provider/openai`) e Messages (`adapters/provider/anthropic`) — então as duas primeiras famílias já são usáveis **só por configuração**. A Responsify API não é falada por nenhum adapter. Além disso, o Go requer que o cliente se identifique (`User-Agent` próprio) e envie `x-opencode-session` estável por conversa.

## Decisão

1. **Config-only** para `/chat/completions` e `/messages`: documentar e exemplificar o `BaseURL` do Zen/Go nos adapters existentes; sem código novo.
2. **Novo adapter `adapters/provider/openai_responses`** para a Responses API, implementando `harness.Provider` com streaming SSE (`response.output_text.delta`, `function_call_arguments.delta`, `response.completed.usage`), tool calling por itens `function_call`/`function_call_output` e parser tolerante a eventos desconhecidos. Reusa a taxonomia de erro e o `httpx.NoCrossHostRedirect` do ADR 0007.
3. **`store=false`** (histórico é do host/harness; sem acoplamento à API stateful).
4. **Headers de cliente**: `Headers` fixos (`User-Agent`) + `SessionHeader` (ex.: `x-opencode-session`) preenchido com o `SessionID` do turno por request, sem expor `net/http` na API pública.
5. **Gemini nativo** fora do v1 (adapter Google separado, se necessário).

## Alternativas consideradas

- **`WireAPI: chat|responses` no `provider/openai`**: mistura dois protocolos e dois parsers no mesmo pacote; rejeitada pela separação (D-ZG-2).
- **SDK oficial da OpenAI**: quebra a regra de dependência mínima e o controle do parser; rejeitada.
- **API stateful da Responses (`store`/`previous_response_id`)**: acopla o host ao provedor; rejeitada.
- **Não suportar Zen/Go**: deixa o operador sem o modelo que usa; rejeitada.

## Consequências

- Mais um adapter a manter (testes `httptest`, taxonomia de erro compartilhada).
- As famílias chat/messages ficam cobertas por configuração — documentação e exemplo obrigatórios (FR-ZG-007).
- O feature 002 (multimodal) deve ser respeitada no adapter Responses via `Capabilities.Vision/Documents` (FR-ZG-009).
- Auth entre famílias (Bearer × `x-api-key`) fica como item de smoke/confirmacão; o harness suporta ambas via `CredentialRef`/`Headers`.
