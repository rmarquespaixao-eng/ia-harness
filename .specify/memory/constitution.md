<!--
Sync Impact Report
- Version change: 1.1.0 → 1.2.0 (MINOR)
  - §2: contrato de fio/persistência = JSON Schema versionado + structs geradas (`cmd/contractgen`, go-jsonschema) + gate de `go generate` sem diff.
  - §3: fachada pública `harness`, contrato folha `internal/core` e motor `internal/engine`, mantendo adaptadores públicos por DI, `internal/platform`, `contracts/` e `cmd/harnessctl`.
  - §7: gate `make verify` inclui `go generate` sem diff, `govulncheck` e testes de contrato contra os schemas.
  - §9: `cmd/harnessctl` explicitamente fora de produção.
- Ratificação inicial: 1.0.0 (template → constitution).
- Seções: 1 Objetivo e escopo · 2 Stack-base · 3 Arquitetura (package-by-feature adaptado) · 4 Segurança ·
  5 Observabilidade · 6 Contrato da biblioteca e eventos · 7 Testes e determinismo · 8 Processo (SDD) ·
  9 Fora de escopo · Governança.
- Base: decisões D-HAR-1..5 da spec + arquitetura do `financeiro-api-v2` (ADR 0011/0012) + AGENTS.md do operador.
  - Feature 003: refactor puro sem alteração de API ou comportamento; dependência `harness → internal/engine → internal/core`.
-->

# Constitution — ia-harness

> Princípios invioláveis do harness. Toda spec, plan, task e código respeita este documento. Mudança aqui é decisão consciente registrada em ADR.

## 1. Objetivo e escopo

Harness de IA reutilizável, **embutível como biblioteca Go no processo de sistemas hospedeiros** (primeiro: `financeiro-api-v2`). Entrega o **lado agente**: loop com tool-calling e streaming, cliente MCP (consome servidores MCP existentes — ex.: `/mcp` do financeiro), abstração multi-provedor/modelo com fallback, política de permissão e confirmação de operações destrutivas, sessões persistidas, auditoria/custo e memória de longo prazo com recuperação semântica — sempre através de **portas fornecidas pelo host**.

Referências obrigatórias: spec `specs/nucleo/001-harness-ia-reutilizavel/spec.md` (D-HAR-1..5). O harness **não** é serviço, não é UI e não é gateway MCP no v1.

## 2. Stack-base

- **Linguagem:** Go (versão corrente do ecossistema, alinhada ao `financeiro-api-v2`; `go 1.27+`). `gofmt`/`gofumpt` obrigatório; `go vet` + `staticcheck` limpos.
- **É biblioteca, não servidor:** a API pública não expõe `net/http`. HTTP existe apenas dentro de adaptadores (provedor, MCP) e é interno.
- **Dependências mínimas e justificadas.** SDK oficial `modelcontextprotocol/go-sdk` para MCP; biblioteca dedicada para validação de **JSON Schema** das tools. Proibido adotar framework de orquestração de IA de terceiros (o loop é nosso, testável e versionável). Dependência nova se justifica na task/PR com trade-off explícito.
- **Contrato = fonte única gerada (espelha o ADR 0011 do financeiro):** todo contrato de **fio ou persistência** (sessão persistida, evento do turno, trilha de auditoria, config de arquivo) vive como JSON Schema versionado em `contracts/<assunto>/*.json` e é a verdade. As structs Go correspondentes são **geradas** por `cmd/contractgen` (invocando `github.com/atombender/go-jsonschema` via `tool` no `go.mod`) para `contracts/gen/*.go`, embutido por `contracts/schemas.go` (`//go:embed */*.json`) — nunca escritas à mão. `go generate ./...` no gate **falha se o gerado divergir** do commitado. Os mesmos schemas podem gerar tipos do host (ex.: TypeScript da UI do financeiro) — sem contrato paralelo.
- **Sem banco, sem ORM, sem fila:** persistência é porta do host.
- **Erros:** tipos de domínio que implementam `error` com `Code` estável; propagação com `fmt.Errorf("...: %w", err)`; inspeção com `errors.Is`/`errors.As`. Erro nunca é engolido.
- **Contexto:** `context.Context` em toda operação de I/O; cancelamento se propaga por todo o turno.

## 3. Arquitetura — package-by-feature adaptado a biblioteca (espelha o financeiro)

Mesma disciplina do `financeiro-api-v2` (ADR 0012): dependência aponta para dentro; feature não importa feature; serviço depende de porta (nunca da implementação concreta); regra pura separada de I/O; contrato gerado de schema. A adaptação necessária: **em biblioteca, a API pública é o núcleo model + regras** — o host importa os pacotes, então o modelo canônico e as portas não podem viver em `internal/`.

```text
harness/                  FACHADA PÚBLICA (package harness): aliases do contrato + New/Run/
                           ResolveConfirmation/Session/Close.
internal/core/            CONTRATO FOLHA: modelo canônico, Config, portas, eventos e erros.
internal/engine/          MOTOR: orquestração do turno no pacote raiz (engine, loop, config_validate,
                           policy_gate, provider_route, window, memory_context, audit_emit) + regra pura
                           por preocupação em subpacotes (policy, telemetry, session, budget).
adapters/                 TODO o I/O, injetado por DI (pacotes públicos):
  provider/openai/          adaptador OpenAI-compatible (chat completions + embeddings)
  provider/anthropic/       adaptador Messages API
  mcpclient/                ToolSource sobre o SDK oficial (Streamable HTTP, progresso, reconexão)
  session/memory/           SessionStore em memória (dev/teste) — a impl. real é do host
  audit/log/  audit/mem/    AuditSink → slog JSON / memória
  memory/inmem/             MemoryStore + Retriever triviais (dev/teste)
  embed/openai/             Embedder via /v1/embeddings compatível
internal/platform/        infra não pública reutilizada: clock (SystemClock), schema (JSON Schema),
                          retry/backoff, trace, httpx (redirect seguro)
contracts/                JSON Schemas por assunto (config, session, events, audit) = fonte da
                          verdade + schemas.go (embed) + gen/ (structs geradas por cmd/contractgen)
cmd/harnessctl/           CLI de desenvolvimento/smoke (NÃO é serviço nem superfície de produção)
```

- **Núcleo não importa adaptador; adaptador importa o núcleo.** Adaptadores são **pacotes públicos** porque o host os injeta explicitamente (DI — padrão `cmd/api/di.go` do financeiro); `harness.New` recebe as portas já montadas. Resolve o ciclo inevitável de `internal/features` em biblioteca e mantém a parede física do subpacote.
- **Portas mínimas:** `Provider` (modelo), `ToolSource`/cliente MCP, `SessionStore`, `AuditSink`, `MemoryStore`, `Retriever`/`Embedder`, `CredentialProvider`, `Clock`. Tempo entra só pela porta `Clock` — `time.Now()` é proibido fora de `internal/platform/clock`.
- **Turno síncrono, eventos por handler:** `Run` bloqueia até concluir; streaming e progresso chegam por `Handler` (interface com no-op embutível). Confirmação é request/response síncrono no handler.
- **Sem import de código do host.** O host importa o harness; jamais o contrário. Nada de tipos do financeiro na API pública.
- **Tipos canônicos próprios** para mensagem, tool call, evento e erro — tipos de SDK de provedor/MCP não vazam para a API pública.
- **Contratos de fio/persistência gerados** de JSON Schema (§2): a API pública usa tipos ergonômicos; o que cruza processo/banco sai de `contracts/gen` e é verificado por teste de conformidade contra o schema.
- **API pública pequena e estável** (semver); campos/opções aditivos; quebra de contrato exige MAJOR + ADR.

## 4. Segurança (invariantes)

- **Zero credencial hardcoded.** Credenciais de provedor e tokens de MCP entram por `CredentialProvider`/config do host; o núcleo nunca as serializa, loga ou persiste.
- **Política fora do modelo.** Allowlist, modo somente-leitura e confirmação são decididas no núcleo, à prova de conteúdo — prompt injection (conteúdo do usuário ou de resultado de tool) **nunca** eleva privilégio.
- **Default deny:** sem política explícita, nenhuma tool executa.
- **Validação sempre:** argumentos de tool são validados contra o JSON Schema publicado antes de qualquer chamada; args inválidos nunca chegam ao servidor.
- **Redação por allowlist + truncamento** antes de log, auditoria e telemetria (segredo, PII, binário); evento de execução negada não carrega payload.
- **Erro nunca vaza stack trace** ao consumidor; mensagem estável + cadeia completa só no log.
- **Operações destrutivas exigem confirmação explícita** do host (espelha o `confirm: true` do domínio financeiro).

## 5. Observabilidade

- Log JSON estruturado via `log/slog` (`slog.JSONHandler`): `level`, `time`, `msg`, `trace_id` e `error` quando aplicável. Sem `fmt.Print*`/`log.Print*`.
- `trace_id`/correlação chegam do host pelo `context` e são propagados a provedor, servidor MCP e eventos de auditoria.
- **Evento por chamada de modelo e de tool** (FR-023): provedor, modelo, tool, latência, status, tokens (reportados ou estimados), custo com rótulo `reportado`×`estimado`, sempre redigido.
- Estimativa de custo por premissa configurável (`Pricing`), recalculável sem reescrever evento passado.
- Erro nunca engolido: tratado com decisão explícita ou propagado com `%w`.

## 6. Contrato da biblioteca e eventos

- API pública no pacote raiz `harness` + subpacotes de adaptadores; documentação de uso no `quickstart.md` da feature.
- Eventos **tipados e versionáveis**: texto incremental, tool call/resultado, progresso, pedido de confirmação, uso, erro, fim de turno. O host decide transporte e UI.
- Cancelamento via `context` encerra o turno de forma consistente: sem duplicar eventos, custo ou efeitos; tool já executada é reportada, não revertida.
- Confirmação pendente pausa a sessão em estado explícito; aprovar executa, negar devolve recusa ao modelo — ambos auditados.
- Semântica de fallback: o modelo pode ser trocado no meio do turno, mas **tool já executada nunca é repetida** (exceto se declarada idempotente).

## 7. Testes e determinismo

- `testing` table-driven (`t.Run`) + `testify` (`require`/`assert`); AAA visível; nomes semânticos; asserções específicas; mock só de portas.
- **Nenhum teste do núcleo toca rede real:** provedores e MCP por fakes, transporte em memória ou `httptest`; sessão/auditoria/memória em implementações de memória.
- Testes de contrato por adaptador (provedor/MCP) com fixtures; validação de schema testada nos casos de borda (tipo, enum, obrigatório, adicional).
- Teste de redação com valores-isca (SC-005); teste de política com conteúdo malicioso embutido (FR-019).
- **Testes de contrato:** marshal dos tipos públicos de fio/persistência validado contra `contracts/<assunto>/*.json`; divergência quebra o teste.
- **Gate local pré-merge (`make verify`):** `gofmt -l` vazio + `go vet` + `staticcheck` + `go generate ./...` **sem diff no gerado** + `go test ./...` + `go build ./...` + `govulncheck ./...` — todos verdes.

## 8. Processo (SDD)

- Ordem: **Constitution → spec → plan → tasks → implementação**, com portão de aprovação em cada fase (AGENTS.md item 7). Nenhum código de feature sem spec/plan/tasks aprovados.
- Requisitos de comportamento em `use-cases.md` (caso de uso fully-dressed Cockburn + Gherkin) no diretório da feature; `tasks.md` cita os `CU-*`.
- Decisão técnica com alternativa/trade-off vira **ADR** em `docs/adr/` (Status/Contexto/Decisão/Consequências; imutável após `Accepted`); procedimento de ambiente/instalação vira **runbook** em `docs/runbooks/AAAA-MM-DD-<slug>.md` antes de executar (AGENTS.md itens 8–9).
- Docs de código por feature em `docs/features/<feature>.md` quando houver código; spec/plan ancoram a mudança na base existente.
- **O operador implementa; o agente audita** e escreve de forma incremental e revisável (zero caixa-preta).

## 9. Fora de escopo (sempre)

- Serviço/API de rede própria do harness; gateway MCP ou agregação publicada a clientes externos (D-HAR-1/2).
- `cmd/harnessctl` como produto: é CLI de desenvolvimento/smoke, sem garantia de superfície estável.
- UI/chat do agente e autenticação dos usuários finais (pertencem ao host).
- Armazenamento próprio (banco, vetorial, fila): vem do host por portas.
- Fine-tuning, treino e hospedagem de modelos.
- Coleta/retenção de credenciais pelo harness: quem entrega é o host.

## Governança

- Esta constitution está acima de qualquer prática ad-hoc; specs, plans, tasks e código devem respeitar seus princípios.
- Todo `plan.md` declara explicitamente que não viola nenhuma seção desta constitution; divergência vira ADR (§8).
- Emenda: proposta registrada em Sync Impact Report no topo deste arquivo e aplicada por edição direta, com revisão do operador do repositório.
- Versionamento semântico: MAJOR para remoção/redefinição incompatível de princípio; MINOR para princípio/seção nova; PATCH para clarificação/redação sem mudança de regra.

**Version**: 1.2.0 | **Ratified**: 2026-09-15 | **Last Amended**: 2026-09-16
