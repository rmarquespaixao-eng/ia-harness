# Research — Núcleo de harness de IA reutilizável

**Feature**: `specs/nucleo/001-harness-ia-reutilizavel` | **Data**: 2026-09-15
**Objetivo**: resolver as lacunas L1–L6 da spec e as incógnitas técnicas do plan com evidência datada.

## R1 — Cliente MCP: SDK oficial Go

**Decisão**: usar `github.com/modelcontextprotocol/go-sdk` (`mcp`) como cliente, com transporte **Streamable HTTP** (`StreamableClientTransport`), sessão persistente e reconexão (`MaxRetries`), progresso via `ClientOptions.ProgressNotificationHandler` e testes por `mcp.NewInMemoryTransports()`. Versão pinada na mesma linha já usada pelo `financeiro-api-v2` (`v1.8.0`, `go.mod` local), que já valida o SDK server-side com SSE no `/mcp`.

**Rationale**: é o SDK oficial mantido com Google; cobre cliente e servidor; fornece transporte em memória (teste determinístico sem rede — FR-030); o financeiro já o usa, então cliente e servidor negociam a mesma linha de protocolo sem surpresa; `ProgressNotificationHandler` entrega as notificações que o financeiro emite nas tools longas.

**Alternativas**: `mark3labs/mcp-go` e `modelcontextprotocol-ce/go-sdk` (terceiros, menos garantia de conformidade e de manutenção); cliente JSON-RPC próprio (custo alto, sem ganho). Descartados.

**Atenção**: versões antigas do SDK têm advisories (GO-2026-5771/4770/4773, DNS rebinding etc.) — pinar versão recente e rodar `govulncheck` no gate.

**Fontes**: `go-sdk/docs/protocol.md` e exemplos do pacote `mcp` (pkg.go.dev, 2026); `financeiro-api-v2/go.mod:11`.

## R2 — Validação de argumentos de tool por JSON Schema

**Decisão**: validar todo argumento contra o JSON Schema publicado pela tool usando `github.com/santhosh-tekuri/jsonschema/v6` (draft 2020-12/2019-09/draft-7; v6.0.3, jun/2026). Compilar o schema uma vez ao descobrir a tool e reutilizar a `*Schema` compilada.

**Rationale**: schema é dinâmico (vem do MCP), então `validator/v10` com struct tags não serve. A lib é madura, passa a suíte oficial de testes (sem opcionais) e cobre drafts novos. O SDK já traz `github.com/google/jsonschema-go` para server-side, mas a validação de entrada do cliente é nossa.

**Alternativas**: `gojsonschema` (sem manutenção ativa), `xeipuuv/gojsonschema` (idem), validação manual (frágil). Descartadas.

**Fonte**: pkg.go.dev/jsonschema/v6 e repositório (última release v6.0.3, jun/2026).

## R3 — Abstração multi-provedor: tipos canônicos + adaptadores

**Decisão**: definir um modelo canônico próprio (`ChatRequest`, `ChatResponse`, `TextDelta`, `ToolCall`, `Usage`, `StopReason`) e um adaptador por família de API:
- `adapters/provider/openai` — **API compatível OpenAI** (`/v1/chat/completions`, streaming SSE, `tools`/`tool_calls`): cobre OpenAI, OpenRouter, Groq, DeepSeek, Ollama, vLLM, LM Studio e afins (baseURL + credencial por perfil).
- `adapters/provider/anthropic` — Messages API nativa (`/v1/messages`, SSE com `content_block_*`, `tool_use`, `stop_reason: tool_use`).
- Demais provedores (ex.: Google) entram por novo adaptador ou pelo adaptador OpenAI-compatible quando expuserem endpoint compatível (FR-031).

**Rationale**: um adaptador OpenAI-compatible já cobre a maior parte do mercado e do homelab (inclui Ollama/vLLM locais), e o adaptador Anthropic cobre a API que mais difere no streaming de tools. Não adotar SDK oficial de cada provedor mantém a árvore de dependências pequena e a API pública estável — trocar provedor é configuração, não código (SC-002/SC-008).

**Alternativas**: SDKs oficiais por provedor (peso e lock-in, tipos vazando na API pública), libs de orquestração tipo `langchaingo` (abstração larga demais, ciclo de release alheio), Responses API da OpenAI (menos suportada por terceiros hoje). Descartadas para o v1.

**Fontes**: docs de tool calling do OpenRouter (openai-compatible, streaming com tools, 2026); exemplo vLLM `/v1/chat/completions` (HuggingFace, 2026); Anthropic Messages/Streaming (platform.claude.com, 2026).

## R4 — Normalização de streaming

**Decisão**: o núcleo emite eventos canônicos (`TextDelta`, `ToolCallStarted`, `ToolCallArgsDelta` opcional, `ToolCallFinished`, `Progress`, `Usage`, `ConfirmationRequired`, `Error`, `TurnFinished`). Cada adaptador converte o formato nativo:
- OpenAI: chunks `choices[].delta.content` → texto; `delta.tool_calls[].function.{name,arguments}` fragmentado por índice → tool call; `finish_reason` → stop; `usage` no chunk final quando solicitado (`stream_options.include_usage`) ou ausente → estimativa.
- Anthropic: `content_block_start/delta/stop` (`text_delta`, `input_json_delta`), `message_delta` (stop_reason + usage de saída) e `message_start` (usage de entrada).

**Rationale**: o núcleo só conhece o modelo canônico; hosts e testes consomem um contrato estável (FR-009). Fragmentos de argumento de tool são acumulados e validados **depois** de completos (o JSON parcial não é validável).

**Fontes**: docs de streaming da Anthropic (eventos e `input_json_delta`, 2026); docs de tool calling do OpenRouter (2026).

## R5 — Uso e custo: reportado × estimado

**Decisão**: usar `Usage` reportado pelo provedor quando existir; quando não, **estimar** por volume de texto (bytes originais medidos antes da redação) ÷ premissa `bytes_per_token` × preços configurados (`Pricing`: entrada/saída por 1M tokens, moeda), marcando o evento como `estimated`. Mesmo desenho híbrido já aceito no financeiro (ADR 0032) — que também permite o cliente **reportar** uso via `_meta["financeiro/usage"]`; o harness, como cliente MCP, pode preencher essa extensão quando o host habilitar.

**Rationale**: nenhum provedor garante tokens em todo modo de streaming; estimar com rótulo é auditável e mantém SC/FR-025 verificável sem prometer precisão que não existe.

**Alternativas**: só reportado (métricas vazias em parte dos casos), contagem exata com tokenizer por modelo (dependência pesada e desatualizada — rejeitada no v1; fica como porta `Tokenizer` opcional).

**Fonte**: memória do projeto financeiro (ADR 0032, 2026-09-15) e spec 021.

## R6 — Memória e RAG: portas, sem armazenamento próprio

**Decisão**: núcleo define as portas `MemoryStore` (fatos/preferências por usuário), `Retriever` (busca semântica por usuário) e `Embedder` (embeddings). O núcleo entrega implementações em memória (dev/teste) e **não** cria banco. O host (financeiro) implementa a persistência real (ex.: pgvector) numa feature própria. Embeddings default por endpoint compatível com `/v1/embeddings` (OpenAI-compatible), injetado pelo host.

**Rationale**: mantém D-HAR-5 (o núcleo não abre banco), FR-029 e SC-001; desacopla o v1 de decisão de vector store do host.

**Alternativas**: embutir sqlite/vector store no núcleo (viola a constitution §3/§9), exigir pgvector (acopla ao financeiro). Descartadas.

**Fonte**: spec §3/§8; constitution §3.

## R7 — Janela de contexto

**Decisão**: orçamento de contexto por turno com estimador heurístico (`bytes_per_token` configurável, default ~4 chars/token) e política `ContextPolicy`:
1. preservar sempre mensagens de sistema e o pedido atual;
2. por default, **truncar do mais antigo** para o mais novo até caber, registrando no evento/auditoria o que foi removido;
3. opcionalmente, **sumarizar** o trecho removido via porta `Summarizer` (default: o próprio provedor, com guarda de 1 sumarização/turno) e injetar o resumo com proveniência.

**Rationale**: truncar é determinístico e testável; sumarizar é opt-in porque custa token e altera conteúdo. Ambos auditados (FR-021).

**Alternativas**: janela fixa por número de mensagens (ignora tamanho real), tokenizer exato (peso), falhar ao estourar (péssima UX). Descartadas.

## R8 — Confirmação de destrutivas: pausa e retomada

**Decisão**: confirmação é **request/response síncrono no handler** (`OnConfirmation(ctx, req) (Decision, error)`); a sessão persiste o estado `awaiting_confirmation` com o pedido pendente (`tool`, argumentos **redigidos** para o evento, `call_id`). Se o processo cair antes da decisão, a sessão retoma no estado pendente (`Run` retoma ou `ResolveConfirmation` decide). Aprovar executa; negar devolve recusa ao modelo como resultado de tool.

**Rationale**: atende FR-018 sem inventar protocolo assíncrono; o host (financeiro) já tem o padrão `confirm: true` e canal de UI/SSE para exibir o pedido.

**Alternativas**: fila assíncrona (complexidade sem caso de uso), confirmação por reenvio do turno (perde contexto). Descartadas.

## R9 — Redação e truncamento

**Decisão**: redação por **allowlist de chaves sensíveis** (`api_key`, `token`, `secret`, `password`, `authorization`, `cookie`, `document_base64`…) + truncamento por campo (default 16 KiB, alinhado ao financeiro). Mensagens, argumentos e resultados são redigidos **antes** de log, `AuditSink` e telemetria; evento `denied` não carrega payload. Valores-isca testados (SC-005).

**Rationale**: mesma política já provada no financeiro (spec 020, D-MCP-3) — consistência entre cliente e servidor da mesma trilha.

**Fonte**: memória/spec do financeiro 020 (trilha redigida/truncada, 2026-09-15).

## R10 — Módulo, versão Go e dependências

**Decisão**: módulo `github.com/rmarquespaixao-eng/ia-harness` (mesma convenção do `financeiro-api-v2`), `go 1.27` (mesma linha). Dependências diretas no v1: `modelcontextprotocol/go-sdk`, `santhosh-tekuri/jsonschema/v6`, `stretchr/testify`; `google/uuid` para IDs. Nada além disso sem justificativa em task (constitution §2).

**Fonte**: `financeiro-api-v2/go.mod` (go 1.27.1, module `rmarquespaixao/...`).

## R11 — Estratégia de testes

**Decisão**: fake de `Provider` dirigido por script (respostas/streams determinísticos), servidor MCP em memória (`mcp.NewInMemoryTransports()`) com tools de teste, stores/audit/memória em memória, `httptest` para casos de borda HTTP dos adaptadores (erro 429, corpo truncado, stream cortado). Nada de rede real no núcleo (FR-030/SC-009). Casos obrigatórios: schema inválido, tool desconhecida, negação de política, confirmação (aprovar/negar), fallback sem repetir tool, redação com isca, retomada de sessão, estouro de janela.

**Fontes**: exemplos do go-sdk com `NewInMemoryTransports` (2026).

## R12 — Semântica de fallback (resolve L2)

**Decisão**: fallback é **por chamada de modelo**, nunca por reexecução do turno:
- falha **antes** de qualquer tool executada → tenta o próximo modelo do perfil com a mesma conversa;
- falha **depois** de tool executada → tenta o próximo modelo **com os resultados já registrados**; a tool **não** roda de novo (FR-014);
- confirmação pendente **sobrevive** ao fallback (estado da sessão é do turno, não do modelo);
- tools marcadas `idempotent: true` (política do host) podem ser repetidas em retry explícito; default `false`.
Registrar modelo efetivo + motivo no evento (FR-013).

## R13 — Observabilidade e trilha (resolve L5/L6)

**Decisão**: `log/slog` JSON com `trace_id`/`correlation_id` extraídos do `context` do host (chave de contexto pública `harness.WithTraceID`); `AuditSink` recebe `AuditEvent` tipado por chamada de modelo e de tool (provedor, modelo, tokens, latência, status, custo reportado/estimado, correlação, redigido). O host decide persistência/transporte; o financeiro mapeia para uma tabela de trilha própria (padrão da `mcp_call_logs`). Formato dos eventos em `contracts/events.md`.

## R14 — Arquitetura de pacotes e contratos (espelhar o financeiro)

**Decisão**: adotar a mesma arquitetura do `financeiro-api-v2`: package-by-feature, regra pura separada de I/O, serviço dependendo de porta com implementação em pacote próprio (parede física), **contratos como JSON Schema versionado** (`contracts/<assunto>/*.json`), embed por `contracts/schemas.go` e structs geradas por `cmd/contractgen` com `github.com/atombender/go-jsonschema` (tool directive), `internal/platform` para infra não pública e `make verify` com `go generate` sem diff.

**Adaptação obrigatória em biblioteca**: o host importa os pacotes do harness (módulo à parte), então o modelo canônico e as portas ficam no pacote público `harness` (papel do `internal/core` do financeiro, público por ser contrato de biblioteca) e os adaptadores são **pacotes públicos injetados por DI** (padrão `cmd/api/di.go`) — `internal/features` não serve para adapters em biblioteca (ciclo inevitável + `internal` é inimportável por outro módulo). Registrado em ADR 0006 (plan D-11/D-12/D-13).

**Evidência local**: `financeiro-api-v2/`: `contracts/schemas.go`, `contracts/gen/doc.go` (`//go:generate ... go run ./cmd/contractgen`), `cmd/contractgen/main.go`, `go.mod` (`tool github.com/atombender/go-jsonschema v0.24.1`), `internal/features/transactions/{service,repo}.go` + `postgres/repo.go`, `internal/core`, `internal/platform`, `Makefile` (`verify`).

## Lacunas da spec — resolução

| Lacuna | Resolvida em |
|--------|--------------|
| L1 — provedores iniciais | R3: adaptador OpenAI-compatible (cobre OpenAI/OpenRouter/Groq/DeepSeek/Ollama/vLLM) + Anthropic; novo provedor = adaptador ou perfil compatível |
| L2 — fallback × destrutivas | R12 |
| L3 — embeddings/memória | R6: portas; host fornece índice; default OpenAI-compatible embeddings |
| L4 — janela de contexto | R7 |
| L5 — contrato de eventos | R4/R13 + `contracts/events.md` |
| L6 — telemetria | R13 |
