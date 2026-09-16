# Changelog

Todas as mudanças relevantes deste projeto são documentadas neste arquivo.
O formato segue [Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/) e o
versionamento segue [SemVer](https://semver.org/lang/pt-BR/).

## [0.2.0] - 2026-09-16

### Added

- **Janela de contexto por modelo e compactação automática (feature 018).** O orçamento passa a derivar de `Capabilities.MaxContextTokens` (menos a reserva de saída e a margem), com `ContextPolicy.CompactAtRatio` (default 0,8) e `SafetyMargin`; `Context.MaxTokens` vira fallback legado. O contexto é reavaliado **a cada chamada de modelo** e a remoção é **pairing-aware** (nunca separa chamada de tool e resultado). O resumo do histórico antigo é injetado com proveniência (`≤1×/turno`), observável em `TurnResult.Compaction` e na interface opcional `CompactionHandler`; a porta opcional `ModelSummarizer` permite resumir com o mesmo modelo do perfil. Supersede o ADR 0005 nas partes de orçamento/gatilho/observabilidade. A contagem agora inclui `system prompt` e definições de tools.

## [0.1.0] - 2026-09-15

Primeiro release do núcleo reutilizável (`specs/nucleo/001-harness-ia-reutilizavel`).
Módulo publicado como `github.com/rmarquespaixao-eng/ia-harness` (ADR 0022).

### Added

- Loop de agente com tool-calling e streaming, com eventos incrementais (`TextDelta`, `ToolCallEvent`, `ProgressEvent`, `UsageEvent`, `AuditEvent`) e `StopReason` sempre preenchido.
- Cliente MCP (Streamable HTTP, SDK oficial) com descoberta de catálogo por sessão, progresso (`notifications/progress`) e reconexão.
- Adaptadores `adapters/provider/openai` (OpenAI-compatible, SSE e tool calls fragmentadas) e `adapters/provider/anthropic` (Messages API, `input_json_delta`).
- Roteamento por alias com fallback por chamada, preservando a causa (`errors.Join`) e expondo o alias efetivo no `TurnResult`.
- Política **default deny** com allowlist por agente e confirmação de tools destrutivas (`awaiting_confirmation` + `ResolveConfirmation`).
- Sessões persistíveis via porta (`SessionStore`), com snapshot/`RestoreSession` validado por JSON Schema.
- Janela de contexto com estratégias `truncate` (default) e `summarize`.
- Auditoria com custo reportado × estimado (por volume × premissa de `Pricing`, marcado `estimated=true`).
- Redação por allowlist (segredos, Base64, assinaturas de URL) em log e auditoria, com cópia profunda nos sinks.
- Memória/RAG por portas (`MemoryStore`, `Retriever`, `Embedder`) com injeção de fatos/trechos citando a fonte.
- Gerador de contratos `cmd/contractgen` (JSON Schema → structs Go, `go generate` idempotente).
- `cmd/harnessctl` — CLI de dev/smoke (um turno com eventos legíveis; **não** é superfície de produção).
- **Entrada multimodal (feature 002):** partes `image`/`document` com `Media` (bytes inline ou referência), allowlist de MIME, teto por mídia e gate `Capabilities.Vision`/`Documents`; mapeamento nativo nos três adapters.
- **Adapter OpenAI Responses + OpenCode Zen/Go (feature 004):** `adapters/provider/openai_responses` (`/responses`, `store=false`, SSE de `response.*`) e `SessionHeader` (`x-opencode-session`) para o Go.
- **System prompt configurável (feature 005):** `Config.SystemPrompt` + `AgentPolicy.SystemPrompt` (override do agente vence), enviado em `ChatRequest.System`.
- **Prompt caching (feature 006):** `Capabilities.PromptCaching` + `cache_control` no adapter Anthropic, leitura de tokens cacheados nos três adapters e preço de cache no custo (`Pricing.CachedInputPriceMicrosPerMillion`/`CacheWritePriceMicrosPerMillion`), expostos em `Usage`/`UsageEvent`/`AuditEvent`.
- **Baseline P0:** retry classificado nos providers (007); loader do arquivo de configuração `adapters/config` (008); saída estruturada (`RunRequest.OutputSchema` + `*OutputError`, 009); tokenizer plugável (`Config.Tokenizer`, 010); tracer plugável para OTel GenAI (`Config.Tracer`, 011).
- **Pacote P1:** streaming de deltas (`StreamingProvider`/`StreamSink`, `ReasoningDelta`/`ToolCallDelta`, 012); teto de resultado de tool (`Config.ToolResultMaxBytes`, 013); middleware/decorators (`ProviderMiddleware`/`ToolMiddleware`/`HandlerMiddleware`, 014); tools em paralelo (`Config.ParallelTools`, 015); pacote `eval` (016); MCP resources/prompts no `mcpclient` (017).

### Notas de release

- Procedimento para publicar (decisão do **operador**, T090):
  1. `make verify` verde no commit do release (fmt/vet/staticcheck/generate sem diff/test/build/vuln).
  2. Criar a tag anotada `v0.1.0` nesse commit (`git tag -a v0.1.0 -m "release v0.1.0"`) e publicá-la (`git push origin v0.1.0`).
  3. O commit da tag e o push são atos do operador — o agente apenas prepara os artefatos (este CHANGELOG e o runbook do smoke).
- Repositório público e `module path` qualificado: `docs/runbooks/2026-09-16-publicar-modulo-github.md` (ADR 0022).
- Smoke real do núcleo contra o `/mcp` de homolog: `docs/runbooks/2026-09-15-smoke-homolog-financeiro.md` (T089, pendente de execução do operador).
