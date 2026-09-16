# Tasks: Núcleo de harness de IA reutilizável

**Input**: Design documents from `/specs/nucleo/001-harness-ia-reutilizavel/`

**Prerequisites**: plan.md (F0–F7), spec.md (US1–US6), use-cases.md (CU-HAR-1..6), research.md (R1–R14), data-model.md, contracts/

**Tests**: **Incluídos** (constitution §7 e AGENTS.md item 5 exigem AAA por task; nenhum teste toca rede real). Escrever o teste antes/junto da implementação; o gate `make verify` precisa ficar verde a cada task.

**Organization**: por user story (US1–US6), com referência aos casos de uso (CU-HAR-*) e requisitos (FR-*). MVP = Fase 1–3 (US1).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: pode rodar em paralelo (arquivos diferentes, sem dependência pendente)
- **[Story]**: US1–US6 conforme `spec.md`
- Caminhos exatos em todas as tasks (raiz do repo `ia-harness`)

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: módulo, dependências, gate local, ADRs e a infraestrutura de **contratos JSON Schema** (D-11) — nada disso é story-specific.

- [X] T001 Criar árvore de diretórios do plan (`harness/`, `adapters/`, `internal/platform/`, `contracts/`, `cmd/`, `docs/`) e inicializar `go.mod` com `module rmarquespaixao/ia-harness` e `go 1.27` — plan §Project Structure
- [X] T002 Adicionar dependências diretas e tool do gerador em `go.mod`/`go.sum`: `modelcontextprotocol/go-sdk` v1.8.x, `santhosh-tekuri/jsonschema/v6`, `stretchr/testify`, `google/uuid`, `tool github.com/atombender/go-jsonschema` — plan §Technical Context / R10
- [X] T003 Criar `Makefile` com `build`, `test`, `vet`, `fmt-check`, `staticcheck`, `generate` e `verify` (`fmt-check + vet + staticcheck + generate sem diff + test + build + govulncheck`) — constitution §7
- [X] T004 [P] Criar `docs/adr/0001-mcp-sdk-oficial-e-modelo-canonico.md` (D-01/D-02/D-03) com Status/Contexto/Decisão/Consequências
- [X] T005 [P] Criar `docs/adr/0002-fallback-e-concorrencia-de-sessao.md` (D-05/D-10)
- [X] T006 [P] Criar `docs/adr/0003-politica-default-deny-confirmacao-e-redacao.md` (D-04/D-09)
- [X] T007 [P] Criar `docs/adr/0004-custo-hibrido-e-memoria-por-portas.md` (D-07/D-08)
- [X] T008 [P] Criar `docs/adr/0005-janela-de-contexto.md` (D-06)
- [X] T009 [P] Criar `docs/adr/0006-contratos-json-schema-e-di.md` (D-11/D-12/D-13)
- [X] T010 Criar os JSON Schemas normativos em `contracts/config/harness_config.json`, `contracts/session/session_snapshot.json`, `contracts/events/turn_event_envelope.json` e `contracts/audit/audit_event.json` — data-model/events.md
- [X] T011 Criar `contracts/schemas.go` (`//go:embed */*.json` + `Schema(path)`) e `contracts/gen/doc.go` com `//go:generate sh -c "cd ../.. && go run ./cmd/contractgen"`
- [X] T012 Criar `cmd/contractgen/main.go` (invoca `go-jsonschema` sobre `contracts/<assunto>/*.json` → `contracts/gen/*.go`) e gerar o código; teste `cmd/contractgen/main_test.go` garante idempotência (gerado em diretório limpo == commitado)
- [X] T013 [P] Criar `.gitignore` (`bin/`, `*.out`, `.DS_Store`) e verificar `gofmt -l` vazio no esqueleto

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: tipos canônicos, portas, config/erros, infra de plataforma, redação, fakes de teste, store em memória e testes de conformidade de contrato. **Bloqueia todas as stories.**

**⚠️ CRITICAL**: nenhuma user story começa antes desta fase.

- [X] T014 Definir tipos canônicos de sessão/mensagem/partes (`Session`, `Message`, `Part`, `ToolCall`, `ToolResult`, `PendingConfirmation`, estados) em `harness/session.go` — data-model §Sessão
- [X] T015 [P] Definir eventos + `Handler` + `NopHandler` + `WithTraceID` em `harness/events.go` — contracts/events.md
- [X] T016 [P] Definir portas (`Provider`, `ToolSource`, `SessionStore`, `AuditSink`, `MemoryStore`, `Retriever`, `Embedder`, `CredentialProvider`, `Summarizer`, `Clock`) em `harness/ports.go` — contracts/library-api.md
- [X] T017 [P] Definir `Config`/`RunRequest`/`TurnResult`/`Budget`/`PolicyConfig`/`Pricing`/`ContextPolicy`/`RedactionConfig` e validação de falha rápida em `harness/config.go`; erros nomeados (`ConfigError`, `PolicyError`, `ToolError`, `ProviderError`) em `harness/errors.go`
- [X] T018 [P] Implementar `internal/platform/clock` (`SystemClock`, única ocorrência de `time.Now()`) + `internal/platform/clock/clock_test.go`
- [X] T019 [P] Implementar `internal/platform/trace` (extração/propagação de `trace_id` no contexto) + teste
- [X] T020 [P] Implementar `internal/platform/retry` (backoff exponencial com jitter, configurável) + teste determinístico
- [X] T021 Implementar `internal/platform/schema` (compilar `jsonschema` uma vez, validar argumento e devolver erro detalhado) + testes de borda (tipo, enum, required, additionalProperties) — R2
- [X] T022 [P] Implementar `harness/redact.go` (allowlist de chaves sensíveis `api_key|token|secret|password|authorization|cookie|*_base64`, `[REDACTED]`, truncamento `max_field_bytes`) + `harness/redact_test.go` com valores-isca — FR-024
- [X] T023 Implementar `adapters/session/memory` (`SessionStore` em memória, roundtrip do snapshot) + `adapters/session/memory/store_test.go` — D-HAR-5
- [X] T024 Criar fakes de teste em `internal/testutil/`: `ScriptedProvider` (respostas/streams determinísticos), `MemToolSource` (tools com progresso/erro), `FakeAuditSink`, `FakeMemoryStore`, `FakeSummarizer`, `FakeRetriever` e `FakeCredentialProvider` (refs `env:`/`file:` simuladas) — R11/FR-030
- [X] T025 Criar teste de conformidade de contrato `contracts/conformance_test.go`: marshal de `Session`, `AuditEvent` e `Config` validado contra `contracts/**/*.json`, e checagem estática de que o pacote `harness` não importa biblioteca de banco/armazenamento (`go list -deps`) — D-11/SC-009/FR-003/FR-029
- [X] T026 Implementar fachada `New` (validação: providers vazio = erro; tools vazio ok; portas obrigatórias), `Close` e `Session` em `harness/harness.go` + `harness/config_test.go` (invariantes 1/7 do contracts) — contracts/library-api.md

**Checkpoint**: fundação pronta — stories podem começar.

---

## Phase 3: User Story 1 - Conversar com os dados via tools MCP (Priority: P1) 🎯 MVP

**Goal**: loop de agente com tool-calling, streaming, validação de schema, orçamento, progresso, cancelamento e reconexão (CU-HAR-1).

**Independent Test**: cenário 1 do `quickstart.md` com provider scriptado + MCP em memória: resposta fundamentada, arg inválido sem chamada, progresso antes do resultado, cancelamento limpo.

- [X] T027 [P] [US1] Implementar transporte do `mcpclient` (`StreamableClientTransport`, header de auth resolvido por `CredentialProvider`, timeout) em `adapters/mcpclient/client.go` — R1/FR-004
- [X] T028 [P] [US1] Implementar catálogo `List` com cache por sessão (nomes namespaceados `servidor.tool`, compile do input schema) em `adapters/mcpclient/catalog.go` — FR-005
- [X] T029 [US1] Implementar `Call` com `ProgressNotificationHandler` → `ProgressUpdate` e mapeamento de erro/`IsError` em `adapters/mcpclient/call.go` — FR-009/FR-011
- [X] T030 [US1] Implementar reconexão/erros de transporte com retry do `internal/platform/retry` em `adapters/mcpclient/reconnect.go` + `adapters/mcpclient/client_test.go` (httptest + `mcp.NewInMemoryTransports`) — FR-011
- [X] T031 [P] [US1] Implementar orçamento do turno (iterações, tokens, tempo) em `harness/budget.go` + `harness/budget_test.go` — FR-008
- [X] T032 [US1] Implementar o loop do turno em `harness/loop.go`: montar contexto → `Provider.Chat` (streaming) → validar args por `internal/platform/schema` → executar tool → devolver resultado → repetir até resposta final. Execução de tools **em sequência** (determinística); tool que exige confirmação nunca executa em paralelo com outra (D-10) — FR-005..009
- [X] T033 [US1] Tratar caminhos de erro do loop: tool desconhecida e argumento inválido viram resultado de erro ao modelo **sem** chamar o MCP — FR-006/FR-007
- [X] T034 [US1] Repassar progresso e implementar cancelamento por `ctx` com `StopReason=cancelled` e sessão consistente — FR-009/FR-010
- [X] T035 [US1] Tratar falha de MCP (`ErrorEvent{Scope:mcp}` + resultado de erro) sem derrubar a sessão, com mensagem estável ao consumidor (sem stack trace; causa completa só no log com `trace_id`) — FR-011/constitution §4
- [X] T036 [US1] Implementar `Run` na fachada (`harness/harness.go`) integrando loop + `SessionStore` + orçamento + `TurnResult` — contracts/library-api.md
- [X] T037 [US1] Teste de integração do cenário 1 do quickstart em `harness/loop_test.go` (AAA: resposta com tool, arg inválido, progresso, cancelamento) + regressão de ordem/sequência de tool calls (duas chamadas no mesmo turno executam em ordem; destrutiva nunca em paralelo) e benchmark `harness/loop_bench_test.go` do overhead com fakes (< 50 ms/turno — plan §Performance Goals)
- [X] T038 [US1] Testes dos critérios Gherkin do CU-HAR-1 em `harness/usecase_cu_har1_test.go`
- [X] T039 [US1] Teste de que nenhum acesso à rede real ocorre no pacote `harness` (`harness/nonet_test.go` com `httptest` fechado + fakes) — FR-030

**Checkpoint**: US1 funcional e testável isoladamente — MVP entregável.

---

## Phase 4: User Story 2 - Multi-provedor com rotação e fallback (Priority: P2)

**Goal**: adaptadores `openai`/`anthropic`, roteamento por alias, capacidades e fallback sem repetir tool (CU-HAR-2).

**Independent Test**: cenário 2 do quickstart: troca de alias só por config; fallback após falha com tool executada sem segunda chamada; perfil sem tool-calling falha explícito.

- [X] T040 [P] [US2] Implementar mapeamento de request/response (não-stream) do adapter OpenAI-compatible em `adapters/provider/openai/chat.go` — R3
- [X] T041 [US2] Implementar streaming SSE do OpenAI (`choices[].delta.content`, `tool_calls[].function.arguments` fragmentado, `stream_options.include_usage`) em `adapters/provider/openai/stream.go` — R4
- [X] T042 [P] [US2] Implementar mapeamento + streaming do adapter Anthropic (`content_block_*`, `input_json_delta`, `message_delta.usage`) em `adapters/provider/anthropic/chat.go` — R4
- [X] T043 [US2] Construir adapters por DI (`openai.New`, `anthropic.New` com `Deps{Credentials, Clock, Logger}`) e headers de auth via `CredentialProvider` — contracts/library-api.md
- [X] T044 [US2] Resolver `ModelProfile` por alias (default, capacidades, params) e falhar explícito quando o turno exige tool e o perfil não suporta — FR-012/FR-015
- [X] T045 [US2] Implementar fallback por chamada de modelo (antes/depois de tool executada; confirmação pendente sobrevive) sem repetir tool — FR-013/FR-014/R12
- [X] T046 [US2] Registrar modelo efetivo e motivo do fallback em `TurnResult.Model` + `AuditEvent` (com `FakeAuditSink`) — FR-013
- [X] T047 [P] [US2] Testes de contrato `httptest` do OpenAI: 429/timeout, stream cortado no meio de tool call (encerra limpo, **sem duplicar evento/cobrança**), fragmento de argumento inválido — R11
- [X] T048 [P] [US2] Testes de contrato `httptest` do Anthropic: evento desconhecido ignorado, `tool_use` + `input_json_delta`, usage ausente → estimativa marcada
- [X] T049 [US2] Testes dos critérios Gherkin do CU-HAR-2 em `harness/usecase_cu_har2_test.go` + teste de extensibilidade em `harness/extensibility_test.go` (provider stub novo e `ToolSource` novo plugados só por DI, sem tocar no núcleo) — FR-031/SC-008

**Checkpoint**: US1 + US2 funcionam independentemente.

---

## Phase 5: User Story 3 - Permissões e confirmação de destrutivas (Priority: P3)

**Goal**: motor de política default deny, somente-leitura, confirmação com pausa/retomada e imunidade a conteúdo (CU-HAR-3).

**Independent Test**: cenário 3 do quickstart: deny sem MCP; confirmação pausa/aprova/nega; retomada após recriar o harness; injeção neutralizada.

- [X] T050 [US3] Implementar o motor de política em `harness/policy.go` (default deny, allowlist/deny glob por tool/servidor, `read_only`) + `harness/policy_test.go` — FR-016
- [X] T051 [US3] Implementar overrides de política por tool (`confirm`, `idempotent`, `timeout`) em `harness/policy.go` + testes — FR-016/FR-014
- [X] T052 [US3] Integrar política ao loop: negar sem chamar `ToolSource` e devolver recusa ao modelo — FR-017
- [X] T053 [US3] Implementar pausa em `awaiting_confirmation` com `PendingConfirmation` (args redigidos) persistido via `SessionStore` — FR-018/FR-020
- [X] T054 [US3] Implementar `ResolveConfirmation` na fachada (aprovar executa e retoma; negar devolve recusa) — FR-018
- [X] T055 [US3] Testar retomada de confirmação pendente após recriar o harness (restart simulado) — CU-HAR-3/SC-004
- [X] T056 [US3] Testar imunidade a injeção: resultado de tool com instrução maliciosa não altera política nem executa — FR-019
- [X] T057 [US3] Testar erro do `Handler.Confirmation` (turno aborta, sessão permanece retomável)
- [X] T058 [US3] Testes dos critérios Gherkin do CU-HAR-3 + verificação SC-003 (toda execução/negativa auditada) em `harness/usecase_cu_har3_test.go`

**Checkpoint**: US1–US3 funcionais.

---

## Phase 6: User Story 4 - Sessões persistidas e janela de contexto (Priority: P4)

**Goal**: snapshot de sessão pelo contrato, retomada, janela truncate/summarize e isolamento (CU-HAR-4).

**Independent Test**: cenário 4 do quickstart: retomada com histórico, aplicação da janela registrada, isolamento entre usuários.

- [X] T059 [US4] Implementar `SnapshotSession`/`RestoreSession` (tipo gerado de `contracts/session`) em `harness/session_snapshot.go` + teste de conformidade — D-11/FR-020
- [X] T060 [US4] Ajustar `adapters/session/memory` para persistir o snapshot (roundtrip fiel; estado `awaiting_confirmation` incluso) + testes — FR-020
- [X] T061 [US4] Implementar janela de contexto `truncate_oldest` (preserva sistema + pedido atual; registra o corte no evento/auditoria) em `harness/window.go` + testes de borda — FR-021/R7
- [X] T062 [US4] Implementar sumarização opt-in via `Summarizer` (guarda de 1×/turno; resumo com proveniência) em `harness/window.go` + teste com fake — FR-021
- [X] T063 [US4] Aplicar checagens de isolamento (`user_id` em sessão/memória/permissão) e erro para `SessionID` inexistente/user divergente — FR-022
- [X] T064 [US4] Garantir que falha do `SessionStore.Save` faz o turno falhar antes de responder (sem perda silenciosa) + teste — FR-020
- [X] T065 [US4] Teste de integração do cenário 4 (retomada com histórico + janela + isolamento) — `harness/usecase_cu_har4_test.go`

**Checkpoint**: US1–US4 funcionais.

---

## Phase 7: User Story 5 - Auditoria, custo e redação (Priority: P5)

**Goal**: `AuditEvent` por chamada conforme schema, custo reportado×estimado, trace e redação (CU-HAR-5).

**Independent Test**: cenário 5 do quickstart: eventos por status, iscas redigidas, `estimated` correto.

- [X] T066 [P] [US5] Implementar `adapters/audit/log` (sink → slog JSON) e `adapters/audit/mem` (sink de teste) em `audit/` + testes — FR-023
- [X] T067 [US5] Implementar `harness/cost.go` (preços/moeda/`bytes_per_token`; cálculo reportado e estimado rotulado) + testes de arredondamento — FR-025
- [X] T068 [US5] Emitir `AuditEvent` de `model_call` e `tool_call` (status `ok|error|denied|awaiting_confirmation`) no loop, usando o tipo do contrato — FR-023
- [X] T069 [US5] Aplicar redação/truncamento no pipeline de log e auditoria (`truncated=true` quando cortar; `denied` sem payload) + teste com iscas no evento real — FR-024/SC-005
- [X] T070 [US5] Propagar `trace_id` do contexto para provider, mcpclient, log e eventos (e gerar quando ausente) + teste end-to-end — FR-026
- [X] T071 [US5] Falha do `AuditSink` vira `ErrorEvent` e não derruba o turno + teste
- [X] T072 [US5] Teste de conformidade do `audit_event.json` com um evento real produzido pelo loop — D-11
- [X] T073 [US5] Testes dos critérios Gherkin do CU-HAR-5 em `harness/usecase_cu_har5_test.go`

**Checkpoint**: US1–US5 funcionais.

---

## Phase 8: User Story 6 - Memória de longo prazo e recuperação (Priority: P6)

**Goal**: portas de memória/fato, recuperação com proveniência, esquecer e embedder opcional (CU-HAR-6).

**Independent Test**: cenário 6 do quickstart: fato influencia nova sessão; recuperação cita fonte; delete remove; isolamento.

- [X] T074 [US6] Implementar `adapters/memory/inmem` (`MemoryStore` com isolamento por usuário) + testes — FR-027
- [X] T075 [US6] Implementar `adapters/memory/inmem` `Retriever` determinístico (score por similaridade simples) + testes — FR-028
- [X] T076 [P] [US6] Implementar `adapters/embed/openai` (endpoint `/v1/embeddings` compatível) reutilizando o adapter OpenAI + teste `httptest` — R6
- [X] T077 [US6] Injetar fatos e itens recuperados no contexto com proveniência (sem porta = sem I/O) — FR-028/FR-029
- [X] T078 [US6] Cobrir "esquecer" (delete não é mais recuperado) e isolamento entre usuários + teste — FR-027
- [X] T079 [US6] Testes dos critérios Gherkin do CU-HAR-6 em `harness/usecase_cu_har6_test.go`

**Checkpoint**: todas as stories funcionais.

---

## Phase 9: Polish & Cross-Cutting Concerns

**Purpose**: empacotamento, smoke real, documentação obrigatória, segurança e release.

- [X] T080 Criar `cmd/harnessctl/main.go` (roda um turno com config de exemplo, imprime eventos de forma legível) — D-13
- [X] T081 Criar `examples/financeiro/di.go` (wiring completo sem segredos: mcpclient para `/mcp`, provider real, política do financeiro com confirm em destrutivas) — contracts/financeiro-integration.md
- [X] T082 Criar `scripts/gen-ts.sh` (gera tipos TS dos schemas `contracts/*` para uso do `financeiro-ui-v2`) + nota no README — D-11
- [X] T083 Criar `docs/features/nucleo-harness.md` com a tabela `Arquivo | O que faz` e o fluxo (AGENTS.md item 0) e atualizar conforme o código final
- [X] T084 Criar `README.md` (uso da biblioteca: `New`/`Run`/`ResolveConfirmation`, DI, gate local) — contracts/library-api.md
- [X] T085 Rodar todos os cenários do `quickstart.md` e registrar o resultado (seção Resultado) no próprio arquivo
- [X] T086 Executar revisão OWASP Top 10 (skill `owasp-top10-review`) e registrar cada achado confirmado como ADR — constitution §8
- [X] T087 Rodar `govulncheck ./...` e fixar versão do SDK MCP; registrar exceção (se houver) no ADR 0001 — R9
- [X] T088 Rodar `make verify` completo (fmt/vet/staticcheck/generate sem diff/test/build) e corrigir pendências — constitution §7
- [X] T089 Escrever `docs/runbooks/2026-09-15-smoke-homolog-financeiro.md` (smoke real com `/mcp` de homolog, chave fora do repo, rollback) e marcar como pendente de execução do operador — AGENTS.md item 8
- [ ] T090 Criar tag `v0.1.0` (release inicial do módulo) com `CHANGELOG.md` curto
- [X] T091 Gerar/atualizar o grafo do código (`graphify .` no repo) após o código fechar — AGENTS.md item 0
- [X] T092 Auditoria final de rastreabilidade: cada FR-001..031 e CU-HAR-1..6 com task/teste correspondente; lacunas viram task nova ou ADR — portão `/speckit-analyze`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Fase 1)**: sem dependência; T004–T009 [P]; T010→T011→T012 em sequência.
- **Foundational (Fase 2)**: depende da Fase 1; **bloqueia** todas as stories. T014–T017 [P]; T018–T021 [P] após T017; T022–T026 sequencial ao final.
- **US1 (Fase 3, MVP)**: depende da Fase 2. T027/T028 e T031 [P]; T029–T030 após T027; T032–T036 após T029/T031; T037–T039 fecham.
- **US2 (Fase 4)**: depende de US1 (usa `Provider` no loop). T040/T042 [P]; T047/T048 [P].
- **US3 (Fase 5)**: depende de US1 (loop) e usa `SessionStore` (Fase 2). Pode andar em paralelo com US2.
- **US4 (Fase 6)**: depende de US1/US3 (estado de confirmação) e do contrato de sessão (Fase 1).
- **US5 (Fase 7)**: depende de US1 (loop) e US2 (modelo efetivo); pode andar em paralelo com US4.
- **US6 (Fase 8)**: depende de US1 (contexto) e portas (Fase 2); independente de US5.
- **Polish (Fase 9)**: depende de todas as stories desejadas.

### Parallel Opportunities

- Fase 1: T004–T009 (ADRs) em paralelo; T013 em paralelo.
- Fase 2: T015–T017 em paralelo; T018–T020 em paralelo; T022 em paralelo.
- US1: T027/T028/T031 em paralelo; US2: T040/T042 em paralelo; T047/T048 em paralelo.
- Após a Fase 2 (com US1 pronto), US2 ∥ US3; US4 ∥ US5 ∥ US6 (com cuidado nos arquivos compartilhados `harness/loop.go`, `harness/harness.go` — tasks que tocam esses arquivos **não** são [P]).

### Within Each User Story

1. Testes da story primeiro (quando escritos como regressão do CU) — falham antes da implementação.
2. Tipos/helpers → serviço/loop → integração → Gherkin.
3. Gate `make verify` verde antes de fechar a story.

---

## Parallel Example: User Story 1

```bash
# Em paralelo (arquivos distintos):
Task: "T027 adapters/mcpclient/client.go — transporte + auth"
Task: "T028 adapters/mcpclient/catalog.go — catálogo"
Task: "T031 harness/budget.go — orçamento"

# Sequencial (dependem dos acima):
Task: "T032 harness/loop.go — loop do turno"
Task: "T036 harness/harness.go — fachada Run"
```

---

## Implementation Strategy

### MVP First (US1 only)

1. Fase 1 (Setup) + Fase 2 (Foundational) — obrigatórias.
2. Fase 3 (US1): loop + MCP + streaming + cancelamento.
3. **STOP e VALIDAR**: cenário 1 do quickstart + `make verify`; é a primeira capacidade útil do harness (com `adapters/session/memory`, sem persistência real do host).

### Incremental Delivery

1. US1 → valida → MVP.
2. US2 (fallback/provedores) → valida.
3. US3 (política/confirmação) → valida — habilita uso real no financeiro com segurança.
4. US4 (sessões/contexto) → valida.
5. US5 (auditoria/custo) → valida.
6. US6 (memória) → valida.
7. Fase 9: smoke real no homolog + release `v0.1.0` + feature espelho no `financeiro-api-v2` (repo do host).

### Parallel Team Strategy

1. Fases 1–2 juntas.
2. Depois: Dev A = US1; com US1 fechado, Dev B = US2, Dev C = US3.
3. US4/US5/US6 paralelizam com o cuidado de não editar `harness/loop.go`/`harness/harness.go` ao mesmo tempo.

---

## Notes

- [P] = arquivos diferentes, sem dependência pendente.
- Todo task fecha com teste AAA (constitution §7) e `gofmt`/`go vet` limpos; testes **sem rede real**.
- Divergência de comportamento achada na implementação → reabrir o artefato SDD (spec/plan) e registrar ADR; nunca corrigir só no editor (AGENTS.md item 7).
- `cmd/harnessctl` e `examples/financeiro` são dev/smoke — não são superfície de produção (constitution §9).
- Ordem de release: harness `v0.1.0` antes da feature espelho no financeiro (o host depende do módulo).

---

## Rastreabilidade final (T092)

**Cobertura**: FR-001..031 e CU-HAR-1..6 com task + teste; `make verify` completo verde em 2026-09-15 (gofmt/vet/staticcheck/generate/test/build/govulncheck).

| Requisitos | Tasks | Evidência de teste |
|---|---|---|
| FR-001..004 (biblioteca/DI/segredos) | T001–T026, T043 | `harness/config_test.go`, `contracts/conformance_test.go`, `contracts/gen` |
| FR-005..011 (loop + MCP) | T027–T039 | `harness/loop_test.go`, `harness/usecase_cu_har1_test.go`, `harness/nonet_test.go`, `adapters/mcpclient/*_test.go` |
| FR-012..015 (multi-provedor/fallback) | T040–T049 | `provider/{openai,anthropic}/*_test.go`, `harness/provider_route_test.go`, `harness/usecase_cu_har2_test.go` |
| FR-016..019 (política/confirmação) | T050–T058 | `harness/policy_test.go`, `harness/usecase_cu_har3_test.go` |
| FR-020..022 (sessões/contexto) | T059–T065 | `harness/session_snapshot_test.go`, `harness/window_test.go`, `harness/usecase_cu_har4_test.go`, `adapters/session/memory/store_test.go` |
| FR-023..026 (auditoria/custo/trace) | T066–T073 | `harness/audit_emit_test.go`, `harness/cost_test.go`, `audit/*/sink_test.go`, `harness/usecase_cu_har5_test.go` |
| FR-027..029 (memória/RAG) | T074–T079 | `adapters/memory/inmem/*_test.go`, `adapters/embed/openai/embedder_test.go`, `harness/usecase_cu_har6_test.go` |
| FR-030/031 (teste sem rede/extensão) | T024, T039, T049 | `harness/nonet_test.go`, `harness/usecase_cu_har2_test.go` (provider stub por DI) |
| SC-003/SC-005 | T058, T069 | Gherkin CU-HAR-3/5 + iscas de redação |

**Divergências fechadas neste work**: FR-015 imposto no loop (capability `ToolCalling`); timeout por override aplicado; pausa por confirmação auditada (`awaiting_confirmation`). Divergências restantes documentadas em `docs/features/nucleo-harness.md` (`implementação atual vs plan`): auditoria default nop vs `adapters/audit/log`; `adapters/embed/openai` no lugar de `adapters/provider/openai`; `toolIdempotent` sem chamador (não há retry de tool por design — R12). Revisão OWASP em `docs/adr/0007` (SEC-01 cross-host redirect corrigido; SEC-02 npm pinado). **T090 (tag `v0.1.0`) permanece aberta**: exige commit/release explícito do operador; `CHANGELOG.md` pronto e `make verify` verde.
