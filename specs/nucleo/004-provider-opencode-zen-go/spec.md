# Feature Specification: Suporte a OpenCode Zen/Go como provedores (Chat Completions, Messages e Responses)

**Feature Branch**: `nucleo/004-provider-opencode-zen-go`

**Created**: 2026-09-16

**Status**: Draft (**aguardando aprovação no portão SDD**)

**Input**: "e o suporte ao opencode go e zen?" (decisão do operador). Depende de `specs/nucleo/001-harness-ia-reutilizavel/` e relaciona-se com a 002 (multimodal) e 003 (organização).

## 1. Problema

O harness tem dois adaptadores de provedor: `adapters/provider/openai` (Chat Completions) e `adapters/provider/anthropic` (Messages API). O OpenCode **Zen** e o **Go** são gateways que expõem os modelos em **três famílias de fio distintas**, todas sob o mesmo host (`opencode.ai/zen[/go]/v1`):

1. `/chat/completions` — OpenAI Chat Completions → **já coberto** por `provider/openai` só por configuração.
2. `/messages` — Anthropic Messages API → **já coberto** por `provider/anthropic` só por configuração.
3. `/responses` — OpenAI **Responses API** → **não coberto**; é onde vivem os modelos GPT (incl. `gpt-5.6-luna`) e Grok/Muse no Zen/Go.

Consequência concreta: o operador usa `gpt-5.6-luna` no plano Go, e o harness não consegue falar com `/zen/go/v1/responses`. Além disso, o Go instrui clientes a identificar-se com `User-Agent` próprio e a enviar um `x-opencode-session` estável por conversa (roteamento/cache), o que hoje não há como propagar.

## 2. User Scenarios & Testing *(mandatory)*

### User Story 1 - Usar Zen/Go pelas famílias já cobertas, só por config (Priority: P1)

O host configura `provider/openai` apontando para `https://opencode.ai/zen/go/v1` (ou `/zen/v1`) e `provider/anthropic` apontando para `https://opencode.ai/zen/go` (ou `/zen`), com a API key do Zen/Go via `CredentialProvider`; nenhum código do harness muda.

**Independent Test**: com `httptest`, montar os adaptadores com esses `BaseURL` e verificar que a rota batida é `/go/v1/chat/completions` e `/go/v1/messages`, com o header de auth correto.

**Acceptance Scenarios**:

1. **Given** um perfil com `provider` = adapter OpenAI-compatible e `BaseURL=https://opencode.ai/zen/go/v1`, **When** o modelo é um GLM/Kimi/DeepSeek, **Then** a chamada vai para `/go/v1/chat/completions` e conclui.
2. **Given** um perfil com adapter Anthropic e `BaseURL=https://opencode.ai/zen/go`, **When** o modelo é Qwen/MiniMax, **Then** a chamada vai para `/go/v1/messages` e conclui.

### User Story 2 - Modelos GPT/Grok/Muse via Responses API (Priority: P1)

O host configura um perfil apontando para a Responses API do Zen/Go; o harness conversa, executa tools e faz streaming nesse formato.

**Why this priority**: é o modelo do operador (`gpt-5.6-luna`) e a lacuna real de código.

**Independent Test**: `httptest` simulando o SSE da Responses API (texto, tool call, `response.completed` com usage) e um turno com tool executada.

**Acceptance Scenarios**:

1. **Given** um perfil em `https://opencode.ai/zen/go/v1/responses`, **When** o usuário pede algo que exige tool, **Then** o adaptador mapeia a chamada de função, o harness executa a tool e o turno conclui com o texto final.
2. **Given** stream com texto incremental, **When** chegam `response.output_text.delta`, **Then** os fragmentos são repassados em `TextDelta`.
3. **Given** `response.completed` com `usage`, **When** o turno termina, **Then** o `Usage` é reportado (`estimated=false`); sem `usage`, estimado (`estimated=true`).
4. **Given** a Responses API devolve erro/stream cortado, **When** o adaptador processa, **Then** o erro é classificado na taxonomia estável e o turno é reportado (sem resposta fabricada).

### User Story 3 - Sessão e identidade do cliente Go (Priority: P2)

O host consegue enviar `x-opencode-session` por conversa e um `User-Agent` próprio, para o Go reconhecer a sessão e rotear/cachear bem, sem que o núcleo dependa de HTTP.

**Independent Test**: com `httptest`, verificar que o header `x-opencode-session` chega com o id da sessão do harness e que o `User-Agent` configurado é aplicado.

**Acceptance Scenarios**:

1. **Given** `Headers` fixos no adapter (`User-Agent`) e um id de sessão no contexto/Config, **When** a chamada é feita, **Then** ambos os headers chegam na requisição.
2. **Given** host sem `x-opencode-session`, **When** a chamada é feita, **Then** o header é omitido sem erro.

### Edge Cases

- **Responses API stateful (`store`)**: a Responses API pode ser stateful; o harness mantém o histórico localmente — decide-se `store=false` para não depender de estado do provedor (ADR).
- **Nomes de itens/eventos**: `response.output_item.added`/`response.function_call_arguments.delta`/`response.completed` devem ser tolerantes a eventos futuros (ignorar desconhecidos), como os outros adaptadores.
- **Reasoning tokens**: modelos GPT podem emitir itens de raciocínio; o adaptador não os expõe como texto (como hoje).
- **Gemini nativo**: fora do v1 — documentado como não suportado.
- **Auth**: confirmar se a família Messages aceita `x-api-key` (Anthropic-style) e se as demais usam `Authorization: Bearer` (research/smoke).

## 3. Requirements *(mandatory)*

### Functional Requirements (EARS)

- **FR-ZG-001**: O harness MUST permitir usar Zen/Go nas famílias `/chat/completions` e `/messages` **somente por `Config`** (BaseURL + CredentialRef + Headers), sem código novo do núcleo.
- **FR-ZG-002**: O harness MUST oferecer um adaptador para a **Responses API** (`/responses`) que implemente `Provider.Chat` com streaming SSE, tool calling e usage.
- **FR-ZG-003**: O adaptador Responses MUST mapear `response.output_text.delta` → texto incremental, `function_call`/argumentos fragmentados → `ToolCall`, `response.completed.usage` → `Usage` (reportado/estimado), e ignorar eventos desconhecidos.
- **FR-ZG-004**: O harness MUST permitir propagar um identificador de sessão estável (`x-opencode-session`) por request e um `User-Agent` configurável, sem expor `net/http` na API pública.
- **FR-ZG-005**: Credenciais do Zen/Go MUST entrar por `CredentialProvider` (ref `env:`/`file:`), nunca na `Config` (constitution §4).
- **FR-ZG-006**: Erros HTTP/stream da Responses MUST reusar a taxonomia estável do harness (rate_limited/timeout/unavailable/transport/stream) e preservar a causa.
- **FR-ZG-007**: O harness MUST documentar (README + `examples`/`di.go`) o wiring de Zen/Go nas três famílias e um runbook de smoke, sem segredo no repo.
- **FR-ZG-008**: Nenhum teste MUST tocar rede real; contratos dos adaptadores via `httptest` (constitution §7).
- **FR-ZG-009**: Suporte a imagem/documento do adapter Responses (quando combinado com a feature 002) MUST ser tratado pelo gate de capacidade, sem quebrar as famílias de texto.

### Key Entities

- **Provider Zen/Go**: gateway com host único e três rotas (`/chat/completions`, `/messages`, `/responses`) e auth por API key.
- **Sessão do cliente**: identificador estável enviado em `x-opencode-session` para roteamento/cache.

### Decisões desta spec (D-ZG)

- **D-ZG-1**: a lacuna de código é a **Responses API**; as outras duas famílias são config-only.
- **D-ZG-2**: adapter novo (`adapters/provider/openai_responses`), sem sobrecarregar o `provider/openai` de chat (evita dois protocolos no mesmo pacote).
- **D-ZG-3**: `store=false` (histórico é do host/harness) e tolerância a eventos desconhecidos.
- **D-ZG-4**: `x-opencode-session`/`User-Agent` via `Headers` + header de sessão derivado do contexto, sem `net/http` na API pública.
- **D-ZG-5**: Gemini nativo fora do v1.

## 4. Success Criteria *(mandatory)*

- **SC-ZG-001**: Um turno real com `gpt-5.6-luna` via `/zen/go/v1/responses` executa tool e responde (smoke homolog/operador).
- **SC-ZG-002**: Trocar entre as três famílias é **só configuração** (adapters/perfis), sem tocar no núcleo.
- **SC-ZG-003**: `make verify` verde e suíte existente intacta.
- **SC-ZG-004**: Nenhuma credencial em log/trilha (teste de redação) e nenhum teste com rede real.

## 5. Não-objetivos (v1)

- Gemini nativo do Zen/Go (adapter Google separado).
- API stateful da Responses (`store`/`previous_response_id`): histórico permanece do host.
- Cobrança/limites do Zen/Go (o harness só mede custo por premissa/uso reportado).
- Reimplementar os adapters de chat/messages (já cobrem as famílias 1 e 2).

## 6. Riscos

- **R-ZG-1 — Responsividade da API**: a Responses API evolui (eventos novos); mitigação FR-ZG-003 (ignorar desconhecidos) + testes de contrato.
- **R-ZG-2 — Auth ambígua entre famílias**: mitigação research/smoke e suporte a header custom.
- **R-ZG-3 — Uso considerado abuso pelo Go** (sem session/UA): mitigação FR-ZG-004.
- **R-ZG-4 — Reasoning itens vazando como texto**: mitigação mapear só `output_text`.
