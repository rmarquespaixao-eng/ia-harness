# Research — 003 Organização (fachada + motor interno)

**Feature**: `specs/nucleo/003-organizacao-pacotes-harness` | **Data**: 2026-09-16
Objetivo: decidir a árvore final e resolver o **ciclo de import** entre fachada e motor, com evidência.

## R1 — O problema do ciclo de import (decisão D-ORG-2)

Se o motor (loop/política) for para `internal/engine` **e** continuar a usar os tipos canônicos, há duas opções ingênuas, ambas inviáveis:

- **Engine importa `harness` (raiz)**: a fachada `harness` precisaria importar `internal/engine` para expor `New/Run` → **ciclo `harness ↔ engine`** (proibido em Go).
- **Fachada não importa engine**: então `New` teria que ser registrado em runtime (factory por `init`), mas o host não pode importar `internal/` para disparar o registro → pacote não linkado.

**Solução adotada (D-ORG-2):** extrair o **contrato** (modelo, portas, `Config`, eventos, erros) para um pacote **folha** `internal/core`. O motor importa `internal/core`; a fachada `harness` importa `internal/core` (para aliasar) e `internal/engine` (para delegar). Direção: `harness → internal/engine → internal/core`, acíclica.

**Aliases e a restrição de `internal/`:** o host importa `harness` e usa `harness.Session`, `harness.Config`, `harness.Provider` etc., que são **aliases de tipo** (`type Session = core.Session`). Não há import de `internal/` pelo host; o alias preserva nome, assinatura, método e literal composto. `harness.New` e os métodos ficam na fachada e delegam a `engine`.

## R2 — Alternativas avaliadas

- **Contrato em pacote público `harness/contract`** com aliases na raiz: funciona e evita o caso de "alias para tipo interno", mas cria um **segundo pacote público** que o host pode importar direto, aumentando a superfície estável. Rejeitada: `internal/core` mantém a superfície pública em um único pacote (`harness`).
- **Manter tudo no pacote raiz `harness`** (status quo): zero risco, mas não resolve a reclamação de organização. Rejeitada.
- **Subpacotes públicos por domínio** (`harness/policy`, `harness/window`…): organiza, porém **quebra a API pública** (imports e símbolos mudam) → exigiria MAJOR. Rejeitada (FR-ORG-001).
- **`internal/engine` único e plano** (sem subpacotes): melhora a fronteira, mas o motor ainda teria ~12 arquivos no mesmo diretório. Rejeitada em favor de subpastas por preocupação (D-ORG-3).

## R3 — Granularidade do motor (D-ORG-3)

Subpastas por preocupação em `internal/engine/`, cada uma com sua regra pura e teste:

| Pasta | Conteúdo atual |
|---|---|
| `internal/engine/` (raiz) | `harness.go` (impl. do `Harness`), `loop.go` (orquestração), `config_validate.go` |
| `internal/engine/policy/` | `policy.go` (motor + override) e `policy_gate.go` (ponte) |
| `internal/engine/routing/` | `provider_route.go` (alias/fallback) |
| `internal/engine/context/` | `window.go` + `memory_context.go` |
| `internal/engine/telemetry/` | `audit_emit.go` + `cost.go` + `redact.go` |
| `internal/engine/session/` | `session_snapshot.go` |
| `internal/engine/media/` | validação multimodal (chega na feature 002) |
| `internal/engine/budget/` | `budget.go` (orçamento do turno) |

Regra: as subpastas expõem **funções/structs pequenas** que o `engine` (raiz) orquestra; não importam umas às outras (compartilham `internal/core`). O `Harness` mantém o estado (`cfg`, portas) e chama cada preocupação.

## R4 — Testes

- Testes de regra pura migram para o pacote que a define (mesmo conteúdo/asserções — FR-ORG-004/005).
- Testes de API/CU (`usecase_cu_harN_test.go`, `loop_test.go`, `nonet_test.go`) ficam na fachada `harness/` (usam só a superfície pública) ou no engine (quando exercitam orquestração não exportada). Decisão de movimento é mecânica, sem reescrita.
- `contracts/conformance_test.go` fica em `contracts/` (verifica contratos contra o schema), como hoje.

## R5 — Documentação e grafo

- `constitution.md` §3 passa a descrever `internal/core` + `internal/engine/...`; emenda classificada como **MINOR** (1.1.0 → 1.2.0) por alterar a estrutura, sem mudar princípio.
- `docs/features/nucleo-harness.md`, `README.md` e `ADRs` com caminhos atualizados; `graphify update .` após a mudança.

## Evidência de viabilidade

Padrão usado por bibliotecas Go que mantêm API na raiz e implementação em `internal/` com **type aliases** exportados; a restrição de `internal/` vale para *importar* o caminho, não para usar um alias exportado. Validado na task T201 por teste de compilação de um consumidor que só importa `harness`.
