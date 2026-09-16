# Implementation Plan: Núcleo de harness de IA reutilizável

**Branch**: `001-harness-ia-reutilizavel` | **Date**: 2026-09-15 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/nucleo/001-harness-ia-reutilizavel/spec.md`

## Summary

Construir o núcleo do harness como **biblioteca Go embutível** (`github.com/rmarquespaixao-eng/ia-harness`) que entrega, in-process, o ciclo completo do agente: cliente MCP (SDK oficial, Streamable HTTP, progresso, reconexão), abstração multi-provedor com fallback, política de permissão com confirmação de destrutivas, sessões persistíveis via portas do host, trilha de auditoria com custo reportado/estimado e memória/RAG por portas. O primeiro consumidor é o `financeiro-api-v2` (integração em feature espelho no repo do host); nenhum serviço, banco ou UI pertence a este repo.

**Arquitetura de pacotes: a mesma do `financeiro-api-v2`** — package-by-feature, regra pura separada de I/O, serviço depende de porta e a implementação mora em subpacote (parede física), **contratos versionados como JSON Schema com structs geradas** (`cmd/contractgen` + `go-jsonschema`, embed e gate de `go generate` sem diff), `internal/platform`, `cmd/` e `Makefile verify`. A única adaptação é a fronteira pública: em biblioteca o host importa os pacotes, então o modelo/portas são públicos e os adaptadores são injetados por DI (ver §3 da constitution v1.1.0 e D-12).

## Technical Context

**Language/Version**: Go 1.27+ (mesma linha do `financeiro-api-v2`)

**Primary Dependencies**: `github.com/modelcontextprotocol/go-sdk` (cliente MCP; linha v1.8.x usada pelo financeiro), `github.com/santhosh-tekuri/jsonschema/v6` (validação de args em runtime), `github.com/stretchr/testify` (testes), `github.com/google/uuid` (IDs), `log/slog` (stdlib). Geração de contratos: `github.com/atombender/go-jsonschema` via **tool directive** (mesmo gerador do financeiro) chamado por `cmd/contractgen`.

**Storage**: nenhum no núcleo — portas (`SessionStore`, `AuditSink`, `MemoryStore`, `Retriever`) fornecidas pelo host; implementações em memória para dev/teste vivem neste repo (`adapters/session/memory`, `adapters/audit/mem`, `adapters/memory/inmem`).

**Testing**: `go test` table-driven + `testify`; provider fake orientado a script, servidor MCP por `mcp.NewInMemoryTransports()`, stores/audit/memória em memória, `httptest` para bordas HTTP dos adaptadores; **testes de contrato** validando o JSON dos tipos públicos contra `contracts/<assunto>/*.json`. **Zero rede real** no núcleo (FR-030).

**Target Platform**: biblioteca Linux (embutida no processo do host); sem binário de produção (o `cmd/harnessctl` é dev/smoke).

**Project Type**: library (módulo Go) + CLI de desenvolvimento.

**Performance Goals**: overhead do núcleo < 50 ms por turno excluindo rede (medido com fakes); primeiro `TextDelta` repassado assim que o adaptador o recebe; catálogo MCP compilado uma vez por sessão.

**Constraints**: API pública pequena e estável; nenhum tipo de SDK de provedor/MCP vaza; `time.Now()` só via `Clock`; segredos só por `CredentialProvider`; default deny de tools; contrato só na forma de JSON Schema gerado.

**Scale/Scope**: single-tenant por instância; ~96 tools no primeiro host (financeiro); sessões curtas; uma execução de turno por sessão (serializada).

**Módulo**: `github.com/rmarquespaixao-eng/ia-harness` (mesma convenção do financeiro; repo Gitea ainda sem remote configurado — ajustar no `go.mod` se o remote mudar).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.* (constitution v1.1.0)

| Seção da constitution | Gate | Status |
|-----------------------|------|--------|
| §2 Stack-base | Go 1.27+, deps mínimas justificadas, biblioteca sem `net/http` público, **contrato = JSON Schema gerado com gate `go generate` sem diff** | PASSA — deps/R10; `contracts/` + `cmd/contractgen` na F0 |
| §3 Arquitetura package-by-feature | Núcleo sem import de adaptador/host; portas em `harness`; adaptador em subpacote com DI; `internal/platform` para infra não pública | PASSA — estrutura abaixo; D-12 |
| §4 Segurança | Sem segredo na config, default deny, validação de schema, redação, confirmação | PASSA — FR-004/016/017/024; política fora do modelo |
| §5 Observabilidade | slog JSON + trace_id + evento por chamada | PASSA — FR-023/026; `AuditSink` |
| §6 Contrato de eventos | Eventos tipados, cancelamento consistente, fallback sem repetir tool | PASSA — `contracts/events.md`; R12 |
| §7 Testes e determinismo | Sem rede real, table-driven, testes de contrato, `make verify` | PASSA — FR-030; R11; D-11 |
| §8 Processo SDD | spec→plan→tasks antes do código; ADRs/runbooks | PASSA — este plan; ADRs 0001–0006 na F0 |

**Pós-Phase 1**: sem violações; nenhuma complexidade a justificar (Complexity Tracking vazio).

## Project Structure

### Documentation (this feature)

```text
specs/nucleo/001-harness-ia-reutilizavel/
├── plan.md              # Este arquivo
├── spec.md              # Especificação (US1–US6, FR-001..031)
├── research.md          # Phase 0 — R1..R13 e resolução de L1..L6
├── data-model.md        # Phase 1 — entidades e invariantes
├── contracts/           # Phase 1 (contrato de design em markdown)
│   ├── library-api.md   # API pública Go (fachada, portas, config, DI)
│   ├── events.md        # Handler/eventos + trilha de auditoria
│   └── financeiro-integration.md
├── quickstart.md        # Cenários de validação
├── checklists/requirements.md
└── tasks.md             # Phase 2 (/speckit-tasks — não criado aqui)
```

### Source Code (repository root)

```text
go.mod                    # module github.com/rmarquespaixao-eng/ia-harness, go 1.27 (+ tool go-jsonschema)
Makefile                  # build, test, vet, fmt-check, staticcheck, generate, verify

harness/                  # API PÚBLICA (package harness) — modelo + portas + fachada + motor puro
  harness.go              # New, Run, ResolveConfirmation, Session, Close
  config.go               # Config + validação (falha rápida, erro nomeado)
  ports.go                # Provider, ToolSource, SessionStore, AuditSink, MemoryStore,
                          # Retriever, Embedder, CredentialProvider, Summarizer, Clock
  events.go               # Handler, eventos do turno, NopHandler, WithTraceID
  session.go              # Session, Message, Part, estado e transições
  policy.go               # motor de política (allowlist, read-only, confirm) — puro
  budget.go  window.go    # orçamento do turno e janela de contexto — puro
  cost.go  redact.go      # custo (reportado×estimado) e redação/truncamento — puro
  loop.go                 # loop do agente (orquestra portas; sem I/O direto)
  errors.go               # ConfigError, PolicyError, ToolError, ProviderError

adapters/                 # TODO o I/O, injetado por DI (pacotes públicos — ADR 0008)
  provider/openai/          # adaptador OpenAI-compatible (chat completions + embeddings)
  provider/anthropic/       # adaptador Messages API
  mcpclient/                # ToolSource: SDK oficial (Streamable HTTP, progresso, reconexão)
  session/memory/           # SessionStore em memória (dev/teste)
  audit/log/  audit/mem/    # AuditSink → slog JSON / memória
  memory/inmem/             # MemoryStore + Retriever triviais (dev/teste)
  embed/openai/             # Embedder via /v1/embeddings compatível

internal/platform/        # infra não pública compartilhada
  clock/                  # SystemClock (única ocorrência de time.Now())
  schema/                 # compilação/validação JSON Schema (santhosh-tekuri)
  retry/                  # backoff/retry reutilizado pelos adaptadores
  trace/                  # extração/propagação de trace_id em context
  httpx/                  # redirect seguro (SEC-01/ADR 0007)
  testutil/               # fakes compartilhados (provider scriptado, ToolSource em memória)

contracts/                # CONTRATOS (fonte única) — espelha o financeiro (ADR 0011)
  schemas.go              # //go:embed */*.json + Schema(path)
  config/  session/  events/  audit/     # JSON Schemas por assunto (.json)
  gen/                    # structs geradas (go-jsonschema) por cmd/contractgen
    doc.go                # //go:generate sh -c "cd ../.. && go run ./cmd/contractgen"

cmd/harnessctl/           # CLI de dev/smoke (subscribe de eventos, run de um turno) — não é serviço
cmd/contractgen/          # gerador: go-jsonschema sobre contracts/<assunto>/*.json → contracts/gen

examples/financeiro/      # wiring de exemplo (DI completa, sem segredos) — documentação executável
docs/
  adr/                    # decisões com trade-off (0001..0006 na F0/F1)
  features/               # doc da feature quando houver código (AGENTS.md item 0)
  runbooks/               # procedimentos (ex.: smoke real com homolog) — quando houver
```

**Structure Decision**: package-by-feature do financeiro adaptado a biblioteca. O pacote `harness` é o núcleo (equivalente ao `internal/core` do financeiro, público por ser contrato de biblioteca) e concentra modelo + portas + fachada + regras puras em arquivos não exportados. Cada adaptador é um pacote público próprio dentro de **`adapters/`** (equivalente ao sufixo de feature + `postgres/repo.go` do financeiro: serviço depende da porta, implementação em pacote separado — parede física; agrupamento pelo eixo I/O — ADR 0008), injetado pelo host via DI (padrão `cmd/api/di.go`). `internal/platform` guarda a infra que não é contrato. `contracts/` é a fonte única dos contratos de fio/persistência, com gerador idêntico em espírito ao `contractgen` do financeiro.

## Decisões técnicas (com trade-off)

| # | Decisão | Alternativas descartadas | Registro |
|---|---------|--------------------------|----------|
| D-01 | Cliente MCP no SDK oficial (Streamable HTTP + progresso + reconexão + transporte em memória) | SDK de terceiro; cliente próprio | ADR 0001 |
| D-02 | Tipos canônicos próprios + adaptadores `openai`/`anthropic` | SDK oficial por provedor; langchaingo | ADR 0001 |
| D-03 | Validação de args com `santhosh-tekuri/jsonschema/v6` | `gojsonschema` (sem manutenção); validação manual | ADR 0001 |
| D-04 | Default **deny** de tools; confirmação síncrona com pausa/retomada da sessão | allow implícito; confirmação assíncrona por fila | ADR 0003 |
| D-05 | Fallback por chamada de modelo; tool executada nunca repete (exceto `idempotent`) | reexecutar turno no fallback | ADR 0002 |
| D-06 | Contexto: truncar do mais antigo (default) com sumarização opt-in via porta | tokenizer exato; falhar ao estourar | ADR 0005 |
| D-07 | Custo híbrido: uso reportado × estimativa por premissa rotulada | só reportado; contagem exata | ADR 0004 |
| D-08 | Memória/RAG só por portas; host dono do índice; embeddings OpenAI-compatible opcional | embutir vector store; exigir pgvector | ADR 0004 |
| D-09 | Redação por allowlist de chaves + truncamento 16 KiB/campo | regex de conteúdo; sem trilha | ADR 0003 |
| D-10 | Sessão = 1 turno por vez; tools executadas **em sequência** (destrutiva/confirmada nunca em paralelo com outra); eventos síncronos no `Handler` | concorrência por sessão; execução paralela de tool calls; fila de eventos | ADR 0002 |
| D-11 | **JSON Schema é a fonte dos contratos** de fio/persistência (config, session, events, audit); structs geradas por `cmd/contractgen` (`go-jsonschema` tool), embed por `contracts/schemas.go`, gate de `go generate` sem diff, teste de conformidade do marshal público contra o schema; os mesmos schemas geram tipos TS para o host | structs à mão; OpenAPI/protobuf; só documentação | ADR 0006 |
| D-12 | **DI explícita de adaptadores**: portas no pacote `harness`; adaptadores em pacotes públicos (`adapters/provider/openai`, `mcpclient`, …) construídos pelo host (`di.go`) e passados no `Config`. `harness.New` só valida e compõe | registry com `init()`/blank-import (como drivers SQL); adaptadores em `internal/features` (ciclo inevitável em biblioteca); Config com `Kind` e construção interna | ADR 0006 |
| D-13 | `cmd/harnessctl` como CLI de dev/smoke (roda um turno, imprime eventos, valida contratos) — sem garantia de produto | binário/serviço de produção (viola D-HAR-1); nenhum binário (dificulta smoke real) | ADR 0006 |

Os ADRs serão criados em `docs/adr/` na F0 (0001–0006), conforme constitution §8 (decisão com trade-off → ADR).

## Fases de implementação (base do `tasks.md`)

| Fase | Escopo | Histórias/FRs | Entregável |
|------|--------|---------------|------------|
| F0 | Fundação: módulo, `Makefile`, tipos canônicos, `Config` + validação, erros, `internal/platform/{clock,trace,schema,retry}`, **infra de contratos** (`contracts/*.json`, `schemas.go`, `cmd/contractgen`, `gen/`, gate de generate), ADRs 0001–0006 | FR-002/004/024 | `make verify` verde com generate idempotente; testes de contrato iniciais |
| F1 | **MVP**: `mcpclient`, loop, eventos, validação de schema das tools, orçamento/iterações, cancelamento, reconexão | US1; FR-005–011 | Cenário 1 do quickstart verde |
| F2 | Providers: `adapters/provider/openai` (streaming + tool calls + usage), `adapters/provider/anthropic`, perfis/roteamento/fallback | US2; FR-012–015 | Cenário 2 verde; `TurnResult.Model` correto |
| F3 | Política: motor, default deny, read-only, confirmação com pausa/retomada, antifraude de injeção | US3; FR-016–019 | Cenário 3 verde |
| F4 | Sessões/contexto: porta + `adapters/session/memory`, snapshot de sessão pelo contrato, retomada, janela truncate/summarize, isolamento | US4; FR-020–022 | Cenário 4 verde |
| F5 | Auditoria/custo: `AuditSink` + `adapters/audit/log`, evento de auditoria pelo contrato, pricing/estimativa, trace_id | US5; FR-023–026 | Cenário 5 verde |
| F6 | Memória/RAG: portas, `adapters/memory/inmem`, embedder opcional | US6; FR-027–029 | Cenário 6 verde |
| F7 | Empacotamento: `cmd/harnessctl`, `examples/financeiro`, geração de TS do host a partir dos schemas, README/doc da feature, revisão OWASP, release `v0.1.0` | FR-030/031; SC-001..009 | Smoke com homolog + `make verify` completo |

Ordem de dependência: F0 → F1 → (F2 ∥ F3) → F4 → (F5 ∥ F6) → F7. O MVP é F0+F1.

## Estratégia de testes

- **Unitários por pacote** (table-driven): política, redação, orçamento, janela de contexto, pricing, parsing de stream de cada adaptador, validação de schema.
- **Integração do loop** com provider fake + MCP em memória: cenários 1–6 do quickstart, incluindo cancelamento, fallback, confirmação e retomada.
- **Contrato (novo, D-11)**: cada tipo público de fio/persistência é marshaled e validado contra `contracts/<assunto>/*.json`; `cmd/contractgen` rodado no gate não pode divergir do `gen/` commitado; snapshots de sessão do F4 e eventos do F5 nascem desse teste.
- **Contrato dos adaptadores** com `httptest`: formato de request, SSE fragmentado, 429/timeout, stream cortado no meio de tool call.
- **Segurança**: iscas de segredo/PII na trilha (SC-005); injeção em resultado de tool (FR-019); default deny (FR-016).
- **Gate (`make verify`)**: `gofmt -l` vazio, `go vet`, `staticcheck`, `go generate ./...` sem diff, `go test ./...`, `go build ./...`, `govulncheck ./...` (advisories do SDK MCP — R1).

## Riscos e mitigação

| Risco (spec) | Mitigação no design |
|--------------|---------------------|
| R1 duplicação de efeito em fallback | D-05 + `idempotent` na política + auditoria da decisão |
| R2 prompt injection | D-04: política decidida fora do modelo; conteúdo nunca eleva privilégio |
| R3 vazamento em trilha | D-09 + teste com iscas; `denied` sem payload |
| R4 custo imprevisível | orçamento por turno (FR-008) + `Budget` + rótulo `estimated` |
| R5 deriva de API dos provedores | adaptadores isolados + testes de contrato com fixtures |
| R6 sessão MCP expira | reconexão do transporte + erro como resultado de tool |
| R7 testes frágeis | fakes determinísticos; zero rede no gate |
| R8 janela de contexto | D-06 + decisão registrada no evento do turno |
| R9 advisory do SDK MCP | pin de versão recente + `govulncheck` no gate |
| R10 drift contrato×código | D-11: schema é a fonte, generate no gate, teste de conformidade |

## Lacunas da spec (resolvidas)

L1→R3 (adaptador OpenAI-compatible cobre OpenAI/OpenRouter/Groq/DeepSeek/Ollama/vLLM + Anthropic nativo) · L2→R12/D-05 · L3→R6/D-08 · L4→R7/D-06 · L5→R4/R13/`contracts/events.md` · L6→R13/D-09.

## Fora de escopo (reafirmado)

Serviço/API própria, gateway MCP, UI, autenticação do usuário final, banco/vector store no núcleo, fine-tuning, billing. `cmd/harnessctl` não é produto. Integração no financeiro (rotas/SSE/UI) é feature espelho no repo do host.

## Próximos passos

`/speckit-tasks` para decompor F0–F7 em tasks rastreáveis (citando FR/CU e este plan) e, aprovado o portão, `/speckit-implement`.
