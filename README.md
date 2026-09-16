# ia-harness

Biblioteca Go **embutível no processo de um sistema hospedeiro** que entrega o lado agente de IA: loop com tool-calling e streaming, cliente MCP (SDK oficial, Streamable HTTP, progresso e reconexão), multi-provedor/modelo com fallback por alias, política de permissão com confirmação de operações destrutivas, sessões persistíveis, trilha de auditoria com custo reportado/estimado e memória de longo prazo com recuperação semântica. Persistência, credenciais, índice e telemetria entram por **portas injetadas pelo host** — o núcleo não abre banco, não guarda segredo e não é serviço (D-HAR-1/2/5 na [spec](specs/nucleo/001-harness-ia-reutilizavel/spec.md); princípios na [constitution](.specify/memory/constitution.md)).

O primeiro host é o `financeiro-api-v2` (integração in-process via `/mcp` do financeiro). Documentação de código: [docs/features/nucleo-harness.md](docs/features/nucleo-harness.md) · validação: [quickstart](specs/nucleo/001-harness-ia-reutilizavel/quickstart.md) · contratos: [`contracts/`](specs/nucleo/001-harness-ia-reutilizavel/contracts/library-api.md).

## Requisitos

- **Go 1.27+** (o `go.mod` declara `go 1.27.1`).
- Nenhum serviço externo para desenvolver/testar: a suíte do núcleo roda **sem rede real** (fakes, transporte MCP em memória e `httptest` — FR-030).
- Para o gate local, as ferramentas (`staticcheck`, `govulncheck`) vêm do bloco `tool` do `go.mod`; o primeiro `make verify` baixa os módulos.

## Instalação

```bash
go get rmarquespaixao/ia-harness
```

Enquanto o módulo não estiver publicado em tag/proxy (release `v0.1.0` pendente — T090), o host consome o clone local via `replace`:

```bash
go mod edit -replace rmarquespaixao/ia-harness=../ia-harness
go mod tidy
```

Nada de SDK de provedor ou de MCP vaza para a API pública: o host importa apenas `harness` (modelo/portas) e os adaptadores que escolher.

## Exemplo mínimo (DI completa)

O host monta os adaptadores no seu `di.go` e passa tudo em `harness.Config`; `New` só valida e compõe (padrão do `cmd/api/di.go` do financeiro — ADR 0006). O exemplo **executável** deste repositório é [`examples/financeiro/di.go`](examples/financeiro/di.go) — `go run ./examples/financeiro` monta o wiring completo (mcpclient para o `/mcp` de homolog, provider OpenAI-compatible, política default deny com `confirm` nas destrutivas, `Pricing`/`Context`) e valida com `harness.New` sem abrir rede nem resolver credencial:

```go
// Resumo de examples/financeiro/di.go (imports e o CredentialProvider do
// host omitidos): todas as credenciais entram por referência env:/file:.
cfg := harness.Config{
	Providers: map[string]harness.Provider{
		"openai": openai.New(openai.Config{
			BaseURL:       "https://openrouter.ai/api/v1",
			CredentialRef: "env:OPENROUTER_API_KEY",
			DefaultModel:  "openai/gpt-4o-mini",
		}, openai.Deps{Credentials: credentials, Logger: logger}),
	},
	Models: map[string]harness.ModelProfile{
		"financeiro-default": {
			Provider: "openai",
			Model:    "openai/gpt-4o-mini",
			Capabilities: harness.Capabilities{
				ToolCalling: true, Streaming: true, MaxContextTokens: 128_000, MaxOutputTokens: 4_096,
			},
		},
	},
	Tools: []harness.ToolSource{
		mcpclient.New(mcpclient.Config{
			Name:          "financeiro",
			Endpoint:      "https://<host-financeiro>/mcp",
			CredentialRef: "env:FINANCEIRO_MCP_KEY",
			ToolTimeout:   2 * time.Minute,
		}, mcpclient.Deps{Credentials: credentials, Logger: logger}),
	},
	Policy: harness.PolicyConfig{
		Default: harness.PolicyDeny,
		Agents: map[string]harness.AgentPolicy{
			"financeiro-chat": {
				Mode:         harness.PolicyAllow,
				AllowTools:   []string{"financeiro.*"},
				ConfirmTools: []string{"financeiro.excluir_transacoes_em_lote", "financeiro.transferir_transacoes", "financeiro.pagar_fatura"},
			},
		},
	},
	Pricing:      harness.Pricing{Currency: "BRL", InputPriceMicrosPerMillion: 500_000, OutputPriceMicrosPerMillion: 1_500_000, BytesPerToken: 4},
	Context:      harness.ContextPolicy{MaxTokens: 120_000, Strategy: harness.StrategyTruncateOldest},
	DefaultModel: "financeiro-default",
	Credentials:  credentials, // CredentialProvider do host (nunca segredo na Config)
	Sessions:     memory.New(),         // host: SessionStore real (ex.: Postgres)
	Audit:        auditlog.New(logger), // host: trilha própria se quiser
	Logger:       logger,
}

h, err := harness.New(cfg) // falha rápida: valida sem abrir rede nem ler credencial
```

Para executar um turno, use `cmd/harnessctl` (dev/smoke) ou a fachada no host:

```go
res, err := h.Run(ctx, harness.RunRequest{
	UserID:  "user-1",
	AgentID: "financeiro-chat",
	Input:   []harness.Part{{Kind: harness.PartText, Text: "qual meu saldo deste mês?"}},
}, handler) // embuta harness.NopHandler e sobrescreva só o que precisar
if res.State == harness.SessionAwaitingConfirmation {
	res, err = h.ResolveConfirmation(ctx, res.SessionID, res.Pending.CallID, harness.Decision{Approve: true}, handler)
}
```

Fachada: `harness.New` (não abre rede nem lê credencial) · `Run` (um turno; `SessionID` vazio cria sessão) · `ResolveConfirmation` (retoma sessão em `awaiting_confirmation`) · `Session` (inspeção) · `Close` (idempotente). Eventos do turno (texto incremental, tool call/resultado, progresso, confirmação, uso e erro) chegam pelo `Handler` de forma síncrona e ordenada; para o host que não consome nada, `harness.NopHandler{}` resolve. O `Handler` de referência, que imprime os eventos e colhe a decisão no terminal, está em [`cmd/harnessctl/main.go`](cmd/harnessctl/main.go).

## Portas implementáveis pelo host

| Porta | Obrigatória? | Papel e implementações disponíveis |
|---|---|---|
| `Provider` | sim (≥ 1 em `Providers`) | Adaptador de modelo com streaming. Prontos: `adapters/provider/openai` (Chat Completions: OpenAI, OpenRouter, Groq, DeepSeek, Ollama, vLLM, LM Studio e **Zen/Go `/chat/completions`**), `adapters/provider/anthropic` (Messages: Anthropic e **Zen/Go `/messages`**) e `adapters/provider/openai_responses` (**Responses API**: GPT/Grok/Muse no Zen/Go, incl. `gpt-5.6-luna`). Provedor novo = adaptador novo por DI, sem tocar no núcleo (FR-031). |
| `ToolSource` | opcional (vazio = agente sem tools) | Cliente MCP. Pronto: `mcpclient` sobre o SDK oficial (`List`/`Call` com progresso e reconexão). |
| `SessionStore` | **sim** | Persistência do snapshot da sessão (dono do formato: `contracts/session`). Em dev/teste: `adapters/session/memory`; em produção o host implementa (Postgres etc.). |
| `CredentialProvider` | **sim** | Resolve referências (`env:VAR`, `file:/caminho`) a cada chamada; nenhum segredo entra na `Config`, log ou trilha (FR-004). |
| `Logger` | **sim** | `*slog.Logger` com JSON (`slog.NewJSONHandler`). |
| `AuditSink` | não (o núcleo usa sink nop quando ausente) | Destino da trilha: `adapters/audit/log` (JSON via slog, resumo em `Info` e payload redigido em `Debug`) ou `adapters/audit/mem` (teste). |
| `MemoryStore` / `Retriever` | opcionais | Fatos duráveis e busca semântica escopados por usuário. Em dev/teste: `adapters/memory/inmem`; em produção o host mantém o índice (ex.: pgvector). |
| `Embedder` | opcional (junto do `Retriever`) | Embeddings: `adapters/embed/openai` (`/v1/embeddings` compatível). |
| `Summarizer` | opcional | Resume o histórico cortado quando `Context.Strategy = summarize` (default: `truncate_oldest`). |
| `Clock` | default do sistema | Relógio injetável para testes determinísticos; `time.Now()` só existe em `internal/platform/clock`. |

`harness.New` falha rápido com erro nomeado (`*ConfigError` com `Code`) se faltar `Credentials`, `Sessions` ou `Logger`, se um `Provider` for nulo ou se um modelo referenciar provider inexistente; `Tools` vazio é válido.

## Provedores: OpenCode Zen e Go (feature 004)

O gateway do OpenCode expõe os modelos em **três famílias de fio**. As duas primeiras são cobertas só por configuração; a terceira (Responses) tem adapter dedicado:

| Família | `BaseURL` (Go) | Adapter |
|---|---|---|
| Chat Completions | `https://opencode.ai/zen/go/v1` | `adapters/provider/openai` |
| Messages (Anthropic) | `https://opencode.ai/zen/go` | `adapters/provider/anthropic` |
| Responses (OpenAI) | `https://opencode.ai/zen/go/v1` | `adapters/provider/openai_responses` |

Troque `/zen/go` por `/zen` para o Zen avulso. A credencial entra por `CredentialProvider` (`env:OPENCODE_GO_API_KEY`; nunca na `Config`). O plano **Go** pede que o cliente se identifique e envie um id de sessão estável: use `Headers: map[string]string{"User-Agent": "seu-agente/1.0"}` e `SessionHeader: "x-opencode-session"` (o harness preenche com o `SessionID` do turno). Wiring completo e válido em [`examples/financeiro/di.go`](examples/financeiro/di.go); smoke real (operador) em [`docs/runbooks/2026-09-16-smoke-zen-go.md`](docs/runbooks/2026-09-16-smoke-zen-go.md).

## Entrada multimodal (imagem e documento)

O usuário pode anexar **imagem** (`image/png`, `image/jpeg`, `image/webp`, `image/gif`) e **documento** (`application/pdf`, `text/plain`, `text/csv`) como partes da mensagem:

```go
input := []harness.Part{
    {Kind: harness.PartText, Text: "esse valor bate com a fatura?"},
    {Kind: harness.PartImage, Media: &harness.Media{
        MIME: "image/png", Name: "fatura.png", SizeBytes: int64(len(png)), Bytes: png,
    }},
}
```

O suporte é **por capacidade do modelo**: o perfil precisa de `Capabilities.Vision` para imagem e `Documents` para PDF (texto/CSV passa como texto). MIME fora da allowlist, mídia acima de `Capabilities.MaxMediaBytes` (default 5 MiB) ou Part sem fonte única (bytes **xor** `reference`) falham com erro nomeado (`*ConfigError` `media/…`) **antes** de qualquer chamada ao provedor. Bytes e URL de mídia nunca saem em log/auditoria. Os três adapters mapeiam para o formato nativo (OpenAI `image_url`/`file`, Anthropic `image`/`document`, Responses `input_image`/`input_file`).

## System prompt (persona/instruções)

Configure o prompt de sistema global na `Config` e, se quiser, sobreponha por agente na política — o override do agente vence:

```go
cfg := harness.Config{
    SystemPrompt: "Você é o assistente financeiro pessoal; responda em PT-BR e seja objetivo.",
    Policy: harness.PolicyConfig{
        Default: harness.PolicyAllow,
        Agents: map[string]harness.AgentPolicy{
            "financeiro-chat": {Mode: harness.PolicyAllow, AllowTools: []string{"financeiro.*"}},
            "conciliacao":     {Mode: harness.PolicyAllow, SystemPrompt: "Você é o agente de conciliação."},
        },
    },
    // ...
}
```

O prompt resolvido vai em `ChatRequest.System` (um bloco de sistema, antes do histórico), separado das mensagens de sistema que o harness injeta para memória/RAG e para o resumo da janela. Sem prompt configurado, o campo fica vazio.

## Prompt caching (reduzir custo)

Ligue o cache por perfil com `Capabilities.PromptCaching` e informe o preço de cache na `Pricing`:

```go
cfg := harness.Config{
    Models: map[string]harness.ModelProfile{
        "claude": {Provider: "anthropic", Model: "claude-...", Capabilities: harness.Capabilities{
            ToolCalling: true, Streaming: true, PromptCaching: true,
        }},
    },
    Pricing: harness.Pricing{
        Currency:                         "USD",
        InputPriceMicrosPerMillion:       3_000_000,
        OutputPriceMicrosPerMillion:      15_000_000,
        CachedInputPriceMicrosPerMillion: 300_000,  // ~0.1× input
        CacheWritePriceMicrosPerMillion:  3_750_000, // ~1.25× input
        BytesPerToken:                    4,
    },
}
```

Com isso, o adapter Anthropic envia `cache_control` no `system` e no último tool; os três adapters leem os tokens cacheados do `usage` (`cache_read_input_tokens`, `prompt_tokens_details.cached_tokens`, `input_tokens_details.cached_tokens`) e o custo aplica o preço de cache. Os campos aparecem em `UsageEvent`/`AuditEvent` e no acumulado da sessão (`CachedInputTokens`/`CacheWriteTokens`). Sem `PromptCaching`, nada muda.

## Baseline (P0): retry, config, saída estruturada, tokenizer e tracing

- **Retry de provider:** os três adapters repetem erros transitórios (429, timeout, 5xx, transporte) com backoff+jitter, **antes** de consumir o stream; 4xx e cancelamento não repetem. `Config.MaxAttempts` (default 3), `RetryBaseDelay`/`RetryMaxDelay` (200ms/2s).
- **Arquivo de configuração:** `adapters/config.Load(path)` valida pelo schema canônico e `File.Apply(&cfg)` aplica models/policy/pricing/context/system_prompt/redaction; `File.MCPServers()` devolve os servidores MCP. Providers/Tools/portas continuam por DI.
- **Saída estruturada:** `RunRequest.OutputSchema` (JSON Schema) vai ao provedor (`response_format`/`text.format`) e o texto final é validado; divergência vira `*OutputError` (`output/schema-invalido`).
- **Tokenizer plugável:** `Config.Tokenizer` (`Count(text) int`) faz a janela de contexto contar tokens reais; sem ele, mantém a heurística bytes/token. Sem dependência de tokenizer no núcleo.
- **Tracing (OTel):** `Config.Tracer` recebe spans de turno/modelo/tool (`StartTurn`/`StartModel`/`StartTool`, `Span.End(err)`); o host liga ao OpenTelemetry GenAI. Sem tracer, no-op.
```go
file, err := config.Load("harness.json")   // valida contra contracts/config
cfg := harness.Config{ /* Providers/Tools/portas por DI */ }
file.Apply(&cfg)
run := harness.RunRequest{
    SessionID: sessionID, UserID: userID, Model: "go-luna",
    Input:        []harness.Part{{Kind: harness.PartText, Text: "extraia o total da fatura"}},
    OutputSchema: json.RawMessage(`{"type":"object","properties":{"total":{"type":"integer"}},"required":["total"]}`),
}
```

## Recursos (P1): streaming, paralelismo, middleware, evals e MCP

- **Streaming de deltas:** providers que implementam `ChatStream` (interface opcional `StreamingProvider`) emitem `ReasoningDelta` e `ToolCallDelta` além de `TextDelta`; adapters antigos caem no caminho de texto. `NopHandler` já traz os no-ops.
- **Tools em paralelo:** `Config.ParallelTools` executa tool calls independentes do turno em paralelo quando **todas** forem permitidas e sem confirmação; eventos/histórico saem na ordem original (o `Handler` precisa ser seguro para o progresso).
- **Middleware:** `Config.ProviderMiddleware`/`ToolMiddleware` (decorators aplicados no boot) e `HandlerMiddleware` (por turno).
- **Teto de resultado:** `Config.ToolResultMaxBytes` (default 32 KiB) corta o resultado que entra no histórico, marcando `Truncated`.
- **Evals:** pacote `eval` roda cenários (`eval.Run`) com `Recorder` e scorers (`ContainsText`, `UsedTool`).
- **MCP resources/prompts:** `mcpclient` lista/lê `ListResources`/`ReadResource` e `ListPrompts`/`GetPrompt` (elicitation/OAuth ficam para depois; sampling é deprecado na spec).

## Gate local (`make verify`)

```bash
make verify   # fmt-check + vet + staticcheck + generate (sem diff) + test + build + govulncheck
```

- `make test` roda a suíte completa (unit AAA + integração do loop com fakes + testes de contrato), **sem rede real**.
- `make generate` roda `go generate ./...` e regenera `contracts/gen`; o gate falha se o gerado divergir do commitado.
- O `Makefile` traz ainda `build`, `vet`, `fmt-check`, `staticcheck`, `vuln` e `clean`.

## Contratos (JSON Schema → Go/TS)

`contracts/{config,session,events,audit}/*.json` são a **fonte única** dos contratos de fio/persistência (ADR 0006). As structs Go correspondentes são geradas por `cmd/contractgen` (invoca `github.com/atombender/go-jsonschema` via `tool` do `go.mod`) para `contracts/gen` — nunca editar à mão:

```bash
go generate ./...        # ou: make generate
```

O pacote `contracts` embute os schemas (`contracts/schemas.go`), `harness.SnapshotSession`/`RestoreSession` usam os tipos gerados e `contracts/conformance_test.go` valida o marshal dos tipos públicos contra os schemas. Para hosts/UI, [`scripts/gen-ts.sh`](scripts/README.md) gera os tipos TypeScript dos mesmos schemas (`contracts/gen-ts/`, gitignored; exige `json2ts` ou `npx`).

## `cmd/harnessctl` (dev/smoke)

CLI de desenvolvimento/smoke (T080/D-13), **não é produto nem superfície estável** — produção é a biblioteca embutida no host. Roda um turno, imprime os eventos no stdout e retoma pelo terminal as confirmações exigidas pela política:

```bash
go run ./cmd/harnessctl -prompt "qual o saldo deste mês?" \
  -model openai/gpt-4o-mini -credential-ref env:OPENROUTER_API_KEY \
  -provider-url https://openrouter.ai/api/v1 \
  -mcp-url https://homolog-api-financeiro.homelab-cloud.com/mcp \
  -mcp-credential-ref env:FINANCEIRO_MCP_KEY \
  -allow-tools 'financeiro.*' -confirm-tools 'financeiro.pagar_fatura'
```

Sem `-allow-tools` vale o default deny (nenhuma tool executa); `-provider-kind anthropic` troca o adaptador; `-pricing-json` informa a premissa de custo. O smoke real com o financeiro tem runbook em [`docs/runbooks/2026-09-15-smoke-homolog-financeiro.md`](docs/runbooks/2026-09-15-smoke-homolog-financeiro.md) (T089, preparado e pendente de execução do operador).

## Limites (constitution §9)

- Biblioteca embutida no processo do host: **sem** serviço/API de rede própria, **sem** gateway MCP, **sem** UI/chat e **sem** autenticação de usuário final (é do host).
- **Sem armazenamento próprio** (banco, vetorial, fila): tudo por porta do host; o repo só traz implementações em memória para dev/teste.
- **Default deny**: sem política explícita para o agente, nenhuma tool executa; confirmação é obrigatória para destrutivas.
- Sem credencial hardcoded; sem tipo de SDK na API pública; erros de domínio com `Code` estável e causa preservada.

## Estrutura

```
harness/          Fachada pública: aliases do contrato + New/Run/ResolveConfirmation/Session/Close
internal/core/    Contrato folha: modelo, Config, portas, eventos e erros
internal/engine/  Motor do turno: orquestração no pacote raiz + regra pura por preocupação (policy, telemetry, session, budget)
adapters/         todo o I/O, injetado por DI (ADR 0008):
  provider/openai|anthropic/   adaptadores de modelo (SSE, tool calls, usage)
  mcpclient/                   ToolSource sobre o SDK MCP oficial (Streamable HTTP, progresso, reconexão)
  session/memory/              SessionStore em memória (dev/teste)
  audit/log|mem/               AuditSink → slog JSON / memória
  memory/inmem/                MemoryStore + Retriever triviais (dev/teste)
  embed/openai/                Embedder OpenAI-compatible
internal/platform/  clock, trace, retry, schema, httpx (infra não pública)
contracts/        JSON Schemas + embed + gen/ (gerado) + teste de conformidade
cmd/contractgen/  gerador dos contratos
cmd/harnessctl/   CLI de dev/smoke (não é produto)
examples/financeiro/  wiring de exemplo do host (documentação executável)
scripts/          gen-ts.sh (tipos TS dos schemas para a UI)
docs/             ADRs, doc da feature e runbooks
specs/            SDD da feature 001 (spec/plan/tasks/use-cases/quickstart)
```

Detalhe arquivo a arquivo e o caminho de um turno: [docs/features/nucleo-harness.md](docs/features/nucleo-harness.md).
