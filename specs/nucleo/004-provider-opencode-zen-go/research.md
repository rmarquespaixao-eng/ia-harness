# Research — 004 OpenCode Zen/Go

**Feature**: `specs/nucleo/004-provider-opencode-zen-go` | **Data**: 2026-09-16
Fonte primária: documentação pública OpenCode Zen (`https://opencode.ai/docs/zen/`) e Go (`https://opencode.ai/docs/go/`), 2026-09-16.

## R1 — Endpoints e famílias

| Família | Zen | Go | SDK |
|---|---|---|---|
| OpenAI Responses | `https://opencode.ai/zen/v1/responses` | `https://opencode.ai/zen/go/v1/responses` | `@ai-sdk/openai` |
| OpenAI Chat Completions | `https://opencode.ai/zen/v1/chat/completions` | `https://opencode.ai/zen/go/v1/chat/completions` | `@ai-sdk/openai-compatible` |
| Anthropic Messages | `https://opencode.ai/zen/v1/messages` | `https://opencode.ai/zen/go/v1/messages` | `@ai-sdk/anthropic` |
| Google Gemini (nativo) | `https://opencode.ai/zen/v1/models/gemini-*` | — | `@ai-sdk/google` |

Modelos GPT (`gpt-5.6-luna`, `gpt-5.6-terra`, `gpt-5.5`, …), Grok e Muse usam **Responses**; GLM/Kimi/DeepSeek/MiniMax/LongCat/MiMo/Hy usam **Chat Completions**; Claude/Qwen (e MiniMax/Qwen no Go) usam **Messages**.

**Conclusão**: as famílias Chat Completions e Messages são **config-only** no harness (adapters existentes). A lacuna de código é a **Responses API**.

## R2 — Mapeamento Responses API → canônico (a implementar)

Eventos SSE relevantes (nomes estáveis da Responses API):
- `response.created` / `response.in_progress` — início (ignorar conteúdo).
- `response.output_item.added` — surge um item (`message`, `function_call`, `reasoning`).
- `response.output_text.delta` → `delta` textual ⇒ `onText` + parte de texto.
- `response.function_call_arguments.delta` / `.done` → argumentos fragmentados de uma chamada de função; `response.output_item.added` com item `function_call` traz `name`/`call_id`.
- `response.output_item.done` — item finalizado.
- `response.completed` — fim, com `response.usage` (`input_tokens`/`output_tokens`).
- `response.failed` / `error` — falha.

Request: `POST /responses` com `{model, input:[...], tools:[...], stream:true, store:false}`; `input` é a lista de itens (mensagens com `content` tipado, `function_call`, `function_call_output`). Diferente do Chat Completions, o **tool result** vira item `function_call_output` e o assistant com tool calls vira itens `function_call`.

**Decisão (D-ZG-3)**: `store=false` (sem estado no provedor) e parser tolerante a eventos desconhecidos, como o parser Anthropic.

## R3 — Auth e headers

- **Go**: a doc instrui (1) User-Agent próprio (ex.: `ia-harness/0.1`), (2) header `x-opencode-session` estável por conversa. Sem eles, o Go monitora e pode degradar.
- **Auth**: chave de API do Zen/Go. A Responses/Chat usam `Authorization: Bearer`; a família Messages é Anthropic-compatible (`x-api-key`) — **confirmar no smoke**; o harness suporta `Headers` fixos e o `CredentialRef` do adapter.
- **Como propagar a sessão**: o adapter recebe `Headers` fixos (User-Agent) e um header de sessão derivado do `SessionID` do harness por request (sem expor `net/http`). Decisão: expor em `Config`/`Deps` um `SessionHeader string` e usar `SessionID` do contexto/req.

## R4 — Impacto no multimodal (feature 002)

`deepseek-v4-flash-vision-exp` (Chat Completions) converte imagem em tokens; a Responses API aceita input de imagem (`input_image`). O adapter Responses deve respeitar `Capabilities.Vision/Documents` do mesmo gate da 002 — sem quebrar o caminho de texto.

## R5 — Alternativas descartadas

- **Estender `provider/openai` com `WireAPI: chat|responses`**: mistura dois protocolos no mesmo pacote e no mesmo parser; um adapter dedicado é mais claro (D-ZG-2).
- **Depender do SDK oficial da OpenAI**: quebra a regra de dependência mínima e o controle do parser (AGENTS.md §3).
- **Usar a API stateful da Responses**: acopla o host ao provedor; o histórico é do host.

## Fontes

- OpenCode Zen — `/docs/zen/` (tabela de endpoints por modelo, 2026-09-16).
- OpenCode Go — `/docs/go/` (endpoints, requisitos de cliente/`x-opencode-session`, privacidade, 2026-09-16).
- OpenAI Responses API — eventos SSE e formato de itens (`output_text.delta`, `function_call`, `completed`), documentação pública, 2026.
