# Data Model — Núcleo do harness

**Feature**: `specs/nucleo/001-harness-ia-reutilizavel` | **Data**: 2026-09-15
Tipos canônicos do núcleo (independentes de provedor). Persistência é responsabilidade do host através das portas; os tipos abaixo são serializáveis em JSON para esse fim. Os **contratos de fio/persistência** (sessão, eventos do turno, auditoria, config) são JSON Schema versionados em `contracts/<assunto>/*.json` — fonte única — e geram structs em `contracts/gen` (`cmd/contractgen`); o teste de contrato valida que o marshal destes tipos satisfaz o schema.

## Mensagem e partes

### `Part`
Unidade de conteúdo de uma mensagem — união tagueada (facilita serialização e redação).

| Campo | Tipo | Regra |
|-------|------|-------|
| `kind` | enum | `text` \| `tool_call` \| `tool_result` |
| `text` | string | obrigatório se `kind=text` |
| `call` | `ToolCall` | obrigatório se `kind=tool_call` |
| `result` | `ToolResult` | obrigatório se `kind=tool_result` |

### `Message`

| Campo | Tipo | Regra |
|-------|------|-------|
| `id` | string | gerado pelo núcleo (UUID v7) |
| `role` | enum | `system` \| `user` \| `assistant` \| `tool` |
| `parts` | []Part | ao menos 1; tool result só em `role=tool` |
| `created_at` | timestamp | injetado via `Clock` |

### `ToolCall`
| Campo | Tipo | Regra |
|-------|------|-------|
| `id` | string | id da chamada (gerado no turno; correlaciona resultado) |
| `name` | string | tool existente no catálogo da sessão |
| `namespace` | string | servidor MCP de origem (colisão de nome resolvida por prefixo/servidor) |
| `args` | JSON | **validado** contra o schema da tool antes da execução |

### `ToolResult`
| Campo | Tipo | Regra |
|-------|------|-------|
| `call_id` | string | referencia `ToolCall.id` |
| `content` | []ResultContent | texto/estruturado; binário nunca é propagado ao modelo |
| `is_error` | bool | erro de execução/validação devolvido ao modelo |
| `denied` | bool | negação de política (não carrega payload em auditoria) |
| `truncated` | bool | true se houve corte por limite de campo |

## Sessão

### `Session`
| Campo | Tipo | Regra |
|-------|------|-------|
| `id` | string | UUID; criado no primeiro `Run` sem `SessionID` |
| `user_id` | string | **isolamento obrigatório** (FR-022) |
| `agent_id` | string | escopo de política e de configuração |
| `model` | string | alias do `ModelProfile` efetivo |
| `state` | enum | `active` \| `awaiting_confirmation` \| `closed` |
| `messages` | []Message | histórico canônico (janela aplicada na montagem do prompt) |
| `pending` | `PendingConfirmation` | presente sse `state=awaiting_confirmation` |
| `usage` | `UsageTotals` | acumulado da sessão |
| `created_at` / `updated_at` | timestamp | via `Clock` |

### Transições de estado
```
active ──(tool exige confirmação)──► awaiting_confirmation
awaiting_confirmation ──(aprovar)──► active  (executa a tool e continua)
awaiting_confirmation ──(negar)────► active  (recusa devolvida ao modelo)
active ──(encerrar/fechar)─────────► closed
```

### `PendingConfirmation`
| Campo | Tipo | Regra |
|-------|------|-------|
| `call_id` | string | tool call pendente |
| `tool_name` | string | nome estável |
| `args_redacted` | string | argumentos **redigidos/truncados** para exibição e auditoria |
| `reason` | string | por que exige confirmação (política) |
| `requested_at` | timestamp | via `Clock` |

## Configuração (do host)

Endpoint e credencial de cada provedor/servidor MCP **não** moram no núcleo: ficam no `Config` do próprio adaptador (`adapters/provider/openai.Config`, `adapters/provider/anthropic.Config`, `mcpclient.Config`), injetado por DI (D-12; ver `contracts/library-api.md`). Referência de credencial (`credential_ref`, ex. `env:VAR`/`file:/path`) é resolvida pelo `CredentialProvider` do host em tempo de chamada.

### `ModelProfile`
| Campo | Tipo | Regra |
|-------|------|-------|
| `alias` | string | chave usada por sessão/agente (`fast`, `default`, `reasoning`) |
| `provider` | string | chave do `Config.Providers` (adapter injetado) |
| `model` | string | id do modelo no provedor |
| `capabilities` | struct | `tool_calling`, `streaming`, `max_context_tokens`, `max_output_tokens` |
| `params` | JSON | temperatura etc., repassados ao adaptador |
| `fallbacks` | []string | aliases tentados em ordem (R12) |

### `PolicyConfig` / `AgentPolicy` / `ToolPolicy`
| Campo | Tipo | Regra |
|-------|------|-------|
| `default` | enum | `deny` \| `allow` — **default do núcleo: `deny`** (constitution §4) |
| `agents[].mode` | enum | `deny` \| `allow` \| `read_only` |
| `agents[].allow_tools` / `deny_tools` | []string | glob por `nome` ou `servidor.nome` |
| `agents[].confirm_tools` | []string | tools que pausam para confirmação humana |
| `agents[].overrides[].idempotent` | bool | permite repetição em retry/fallback (default `false`) |
| `agents[].overrides[].timeout` | duration | timeout específico da tool |

### `Pricing` e `ContextPolicy`
| Campo | Tipo | Regra |
|-------|------|-------|
| `currency` | string | ISO (ex.: `BRL`, `USD`) |
| `input_price_micros_per_million` / `output_price_micros_per_million` | int64 | micros por 1M tokens |
| `bytes_per_token` | float | premissa de estimativa (default 4) |
| `max_tokens` | int | orçamento de contexto por turno |
| `strategy` | enum | `truncate_oldest` (default) \| `summarize` |
| `summarizer` | porta | obrigatória se `summarize`; default = provedor com guarda de 1/turno |

## Auditoria

### `AuditEvent` (um por chamada de modelo e um por execução/negativa de tool)
| Campo | Tipo | Regra |
|-------|------|-------|
| `id` | string | UUID |
| `kind` | enum | `model_call` \| `tool_call` |
| `trace_id` | string | do `context` do host (FR-026) |
| `user_id` / `session_id` / `agent_id` | string | escopo |
| `provider` / `model` | string | chamada de modelo |
| `tool` / `tool_call_id` | string | chamada de tool |
| `status` | enum | `ok` \| `error` \| `denied` \| `awaiting_confirmation` |
| `input_tokens` / `output_tokens` | *int64 | nulo quando desconhecido |
| `estimated` | bool | true quando tokens/custo vêm de premissa (FR-025) |
| `cost_micros` | *int64 | custo calculado |
| `currency` | string | moeda da premissa |
| `latency_ms` | int64 | duração da chamada |
| `args_redacted` / `result_redacted` | string | ≤ `max_field_bytes`; vazio em `denied` |
| `error` | string | mensagem estável, sem stack |
| `occurred_at` | timestamp | via `Clock` |

### `Usage` / `UsageTotals`
| Campo | Tipo | Regra |
|-------|------|-------|
| `input_tokens` / `output_tokens` | int64 | 0 quando desconhecido + `estimated=true` |
| `estimated` | bool | reportado × estimado |
| `cost_micros` / `currency` | int64/string | derivados de `Pricing` |

## Memória (portas do host)

### `Fact`
| Campo | Tipo | Regra |
|-------|------|-------|
| `id` | string | UUID |
| `user_id` | string | isolamento (FR-022/FR-027) |
| `kind` | enum | `fact` \| `preference` |
| `text` | string | conteúdo durável |
| `source` | string | proveniência (ex.: `user`, `session:<id>`) |
| `created_at` / `updated_at` | timestamp | via `Clock` |

### `RetrievedItem`
| Campo | Tipo | Regra |
|-------|------|-------|
| `id` | string | id no índice do host |
| `text` | string | trecho recuperado |
| `score` | float | similaridade |
| `source` | string | proveniência citável (FR-028) |

## Tool (visão do núcleo)

### `Tool`
| Campo | Tipo | Regra |
|-------|------|-------|
| `name` | string | nome publicado |
| `namespace` | string | servidor MCP de origem |
| `description` | string | repassada ao modelo |
| `input_schema` | JSON Schema | compilado uma vez (R2) para validação |
| `destructive` | bool | derivado da política do host (não do schema) |
| `idempotent` | bool | da política; default `false` |

## Rastreabilidade requisito → entidade

| Requisito | Entidades |
|-----------|-----------|
| FR-005/006/007 | Tool, ToolCall, ToolResult |
| FR-008 | TurnResult, Budget |
| FR-010 | Session.state, PendingConfirmation |
| FR-012/013/014 | ModelProfile, Provider (DI), AuditEvent.model_call |
| FR-016–019 | PolicyConfig, AgentPolicy, ToolPolicy, PendingConfirmation, AuditEvent.tool_call |
| FR-020/021/022 | Session, Message, ContextPolicy |
| FR-023–026 | AuditEvent, Usage, Pricing |
| FR-027–029 | Fact, RetrievedItem |
