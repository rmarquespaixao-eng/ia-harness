# Feature Specification: Organização do repositório — fachada pública `harness` + motor em `internal/`

**Feature Branch**: `nucleo/003-organizacao-pacotes-harness`

**Created**: 2026-09-16

**Status**: Draft (**aguardando aprovação no portão SDD** — refactor arquitetural)

**Input**: "a estrutura dentro do harness tá tudo solta, organiza isso também" (decisão do operador após revisão). Evolução da decisão do **ADR 0008** (que agrupou `adapters/` e deixou o split do núcleo como "evolução futura").

## 1. Problema

Hoje o pacote raiz `harness/` concentra **19 arquivos `.go`** (fora testes) com papéis heterogêneos no mesmo diretório: contrato público (`session.go`, `ports.go`, `config.go`, `events.go`, `errors.go`), orquestração (`harness.go`, `loop.go`, `provider_route.go`, `policy_gate.go`) e regras puras (`policy.go`, `budget.go`, `window.go`, `cost.go`, `redact.go`, `audit_emit.go`, `memory_context.go`, `session_snapshot.go`, `config_validate.go`). Consequências:

- **Leitura lenta**: não há separação física entre o que é **contrato estável** (o que o host importa) e o que é **motor interno** (o que pode evoluir sem quebrar host).
- **Blast radius opaco**: qualquer arquivo pode tocar qualquer outro; a fronteira público×interno só existe por convenção, não por pasta.
- **Reincidência**: o ADR 0008 resolveu `adapters/`, mas o núcleo ficou plano; revisões seguintes já apontaram de novo o "layout solto".

O objetivo **não** é mudar comportamento: é dar **parede física** entre contrato e motor, espelhando o `internal/core` do `financeiro-api-v2` (ADR 0012), preservando **100% da API pública atual** (`harness.New/Run/ResolveConfirmation/Session/Close`, tipos, portas, eventos, erros).

## 2. User Scenarios & Testing *(mandatory)*

### User Story 1 - Host inalterado consome a mesma API (Priority: P1)

O `financeiro-api-v2` (e os exemplos deste repo) continuam importando `rmarquespaixao/ia-harness/harness` e usando os mesmos símbolos (`harness.New`, `harness.Config`, `harness.Session`, `harness.Provider`…) sem uma única alteração.

**Independent Test**: compilar `examples/financeiro` e `cmd/harnessctl` sem mudar imports/símbolos; `go build ./...` verde; suíte existente sem alteração de expectativa.

**Acceptance Scenarios**:

1. **Given** o código atual de `examples/financeiro/di.go`, **When** o refactor termina, **Then** ele compila sem trocar nenhum identificador público.
2. **Given** a API pública documentada no `README`, **When** o refactor termina, **Then** os mesmos símbolos existem (nomes e assinaturas).

### User Story 2 - Fronteira física entre contrato e motor (Priority: P1)

Um leitor entende o repositório pela árvore: `harness/` = contrato/fachada; `internal/core` = modelo/portas/config/eventos/erros; `internal/engine/...` = motor por preocupação. A dependência aponta para dentro: fachada → engine → core; adaptador → fachada (contrato).

**Independent Test**: inspeção da árvore + checagem de que `internal/core` não importa `internal/engine`/`harness`, e que `harness` não importa nenhum `adapters/`.

**Acceptance Scenarios**:

1. **Given** a árvore final, **When** um arquivo de `internal/core` é lido, **Then** ele não importa engine nem adaptadores.
2. **Given** um erro de compilação introduzido de propósito (engine importando fachada), **When** o gate roda, **Then** falha (prova da direção da dependência).

### User Story 3 - Comportamento idêntico e gate verde (Priority: P1)

Nenhuma regra muda: mesmo comportamento observável, mesmos testes verdes, `make verify` completo verde e grafo regenerado.

**Independent Test**: `make verify` e `go test ./...` verdes **sem** alterar expectativas de teste (só movendo arquivos/pacotes).

**Acceptance Scenarios**:

1. **Given** a suíte antes do refactor, **When** roda depois, **Then** passa com as mesmas asserções (mover testes, não reescrever).
2. **Given** `go generate ./...`, **When** roda, **Then** nenhum diff.

### Edge Cases

- **Aliases de tipo e a restrição de `internal/`**: expor tipos de `internal/core` pela fachada via **alias** (`type Session = core.Session`) mantém o host sem importar `internal/` (decisão registrada no ADR 0010; validado por teste de compilação de um pacote externo).
- **Ciclo de import**: fachada→engine→core é acíclico; o motor **não** importa a fachada.
- **Testes internos** que exercitam funções não exportadas (`evaluatePolicy`, `truncateOldest`, `estimateWindowTokens`) passam a viver no pacote que as define (`internal/engine/...`).
- **Docs e grafo**: tabelas `Arquivo | O que faz` e `graphify-out` precisam ser atualizados no mesmo trabalho.

## 3. Requirements *(mandatory)*

### Functional Requirements (EARS)

- **FR-ORG-001**: A API pública da biblioteca MUST permanecer **idêntica** (nomes e assinaturas de `New`, `Harness.Run`, `ResolveConfirmation`, `Session`, `Close`, tipos/portas/eventos/erros) — refactor sem quebra (semver MINOR de estrutura, sem MAJOR de contrato).
- **FR-ORG-002**: O repositório MUST separar fisicamente **contrato** (`internal/core`), **motor** (`internal/engine` + subpacotes por preocupação) e **fachada** (`harness/`).
- **FR-ORG-003**: A direção de dependência MUST ser acíclica: `harness` → `internal/engine` → `internal/core`; `adapters/*` → `harness` (contrato); `internal/core` MUST NOT importar `internal/engine`, `harness` ou `adapters`.
- **FR-ORG-004**: Nenhum comportamento observável MUST mudar: mesmos eventos, ordens, erros e códigos; suíte existente verde sem reescrita de expectativa.
- **FR-ORG-005**: Todo teste MUST acompanhar o código que cobre: testes de regra pura → pacote do motor; testes de API/CU → fachada; testes de contrato→ `contracts/`.
- **FR-ORG-006**: O contrato JSON Schema e o `go generate` MUST permanecer idempotentes (sem mudança de schema nesta feature).
- **FR-ORG-007**: `docs/features/nucleo-harness.md`, `README.md`, `constitution.md` (§3) e o grafo MUST refletir a nova árvore no mesmo trabalho.
- **FR-ORG-008**: `make verify` completo MUST ficar verde (fmt/vet/staticcheck/generate sem diff/test/build/govulncheck).

### Key Entities

- **Contrato (`internal/core`)**: modelo canônico, portas, `Config`, eventos, erros.
- **Motor (`internal/engine`)**: `Harness` (orquestração) + regras por preocupação (policy, window/context, telemetry, routing, session, media, budget).
- **Fachada (`harness`)**: aliases do contrato + construtor/métodos públicos.

### Decisões desta spec (D-ORG)

- **D-ORG-1**: fachada pública fina + motor interno (escolha do operador).
- **D-ORG-2**: contrato em `internal/core` com **aliases** na fachada para preservar a API e quebrar o ciclo de import.
- **D-ORG-3**: motor subdividido por preocupação (pastas visíveis), não um pacote plano.
- **D-ORG-4**: refactor **puro** — zero mudança de comportamento; qualquer ajuste de regra vai para a feature 002 (multimodal) ou nova spec.

## 4. Success Criteria *(mandatory)*

- **SC-ORG-001**: `examples/financeiro` e `cmd/harnessctl` compilam **sem** trocar identificadores públicos.
- **SC-ORG-002**: `internal/core` sem import de `internal/engine`/`harness`/`adapters` (verificado por teste/gate).
- **SC-ORG-003**: `make verify` verde; suíte com as mesmas asserções.
- **SC-ORG-004**: árvore final documentada (tabelas de arquivo e grafo) e sem arquivos órfãos.

## 5. Não-objetivos

- Mudança de comportamento/API pública (isso seria MAJOR + ADR).
- Alterar `adapters/` (já organizado pelo ADR 0008), salvo ajuste de import para a fachada.
- Novos recursos (ver feature 002).

## 6. Riscos

- **R-ORG-1 — Quebra acidental da API**: mitigação FR-ORG-001/SC-ORG-001 (compilação de consumidores) + ADR 0010.
- **R-ORG-2 — Ciclo de import**: mitigação D-ORG-2 + teste de direção (SC-ORG-002).
- **R-ORG-3 — Refactor misturado com feature**: mitigação D-ORG-4 (002 primeiro ou depois, nunca junto).
- **R-ORG-4 — Docs/grafo defasados**: mitigação FR-ORG-007.
- **R-ORG-5 — Regressão silenciosa**: mitigação FR-ORG-004/SC-ORG-003 (suíte intacta).
