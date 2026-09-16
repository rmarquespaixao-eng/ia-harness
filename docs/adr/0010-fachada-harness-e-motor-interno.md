# ADR 0010 — Fachada pública `harness` + motor em `internal/` (pacote folha `internal/core`)

**Status**: Accepted
**Data**: 2026-09-16
**Feature**: `specs/nucleo/003-organizacao-pacotes-harness`
**Relaciona-se com**: ADR 0008 (agrupamento de `adapters/`) — esta é a evolução do núcleo que o ADR 0008 havia adiado.

## Contexto

Após o ADR 0008, `adapters/` ficou organizado, mas o pacote raiz `harness/` permaneceu **plano**: contrato público, orquestração e regras puras nos mesmos ~19 arquivos. Não há parede física entre o que o host importa (contrato estável) e o que pode evoluir (motor). O operador pediu organização explícita. Restrições: **preservar 100% da API pública** (o harness ainda não tem consumidor em produção, mas a estabilidade é princípio da §6) e manter a direção de dependência do package-by-feature (ADR 0012 do financeiro).

## Decisão

Separar em três camadas:

```
harness/            fachada pública: aliases de tipo (Session = core.Session, ...) + New/Run/ResolveConfirmation/Session/Close
internal/core/      contrato folha: modelo canônico, portas, Config, eventos, erros
internal/engine/    motor: orquestração no pacote raiz (engine.go, loop.go, config_validate.go, policy_gate.go,
                    provider_route.go, window.go, memory_context.go, audit_emit.go) + regra pura por preocupação
                    em subpacotes: policy/ (autorização), telemetry/ (redação + custo), session/ (snapshot), budget/ (orçamento)
```

**Escopo da extração (revisão pós-implementação):** só as preocupações **puras e sem estado do `Harness`** viraram subpacotes (`policy`, `telemetry`, `session`, `budget`). As que são **métodos de `Harness`** (roteamento de modelo, janela de contexto, injeção de memória, emissão de auditoria) permanecem no pacote `engine`: movê-las exigiria converter métodos em funções e reescrever a suíte correspondente, sem ganho que justificasse o risco (decisão registrada aqui; o plan da feature 003 listava subpacotes também para `routing`/`context`). A direção de dependência segue: `harness → engine/{policy,telemetry,session,budget} → core`.

O **ciclo de import** é resolvido extraindo o contrato para o pacote folha `internal/core`: `harness → internal/engine → internal/core`, e `adapters/* → harness`. Como o host não pode importar `internal/`, a fachada expõe o contrato por **type aliases exportados** (`type Session = core.Session`), que preservam nome, método e literal composto sem que o host importe o caminho interno.

## Alternativas consideradas

- **Contrato em pacote público `harness/contract`**: evita o alias para tipo interno, mas cria um segundo pacote público estável. Rejeitada (superfície menor com `internal/core`).
- **Manter tudo no pacote raiz**: zero risco, mas não resolve a organização. Rejeitada.
- **Subpacotes públicos por domínio** (`harness/policy`, …): organiza, mas **quebra a API** (MAJOR). Rejeitada.
- **`internal/engine` plano**: melhora a fronteira mas mantém muitos arquivos no mesmo diretório. Parcialmente adotada: o pacote `engine` mantém a orquestração (inclusive o que é método de `Harness`) e só as regras puras foram para subpacotes.

## Consequências

- **Sem mudança de API**: `harness.New/Run/...` e todos os tipos/portas permanecem; consumidores (`examples/financeiro`, `cmd/harnessctl`) compilam sem trocar identificador.
- **Constitution §3 emendada** (MINOR 1.1.0 → 1.2.0) para refletir a árvore; princípios inalterados.
- **Direção de dependência verificável** por teste/`go list -deps`: `internal/core` não importa `engine`/`harness`/`adapters`; `harness` não importa `adapters`.
- Testes de regra pura migram para o pacote que as define; asserções não mudam.
- Docs (`docs/features/nucleo-harness.md`, `README`) e grafo atualizados no mesmo trabalho.
- Custo: um alias-mapa a manter quando a API pública crescer; mitigado por teste de compilação de consumidor.
