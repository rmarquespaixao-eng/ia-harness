# ADR 0027 — Multi-agente por delegação

**Status**: Accepted
**Data**: 2026-09-17
**Feature**: `specs/nucleo/022-multi-agente`

## Contexto

O host precisa de orquestrador + especialistas sem montar orquestração externa. Publicar cada agente como tool enche o catálogo, e um runner implementado por adaptador público criaria ciclo motor↔adaptador. A recursão sem limite abre DoS.

## Decisão

1. Agentes nomeados em `Config.Agents` (`AgentSpec`: `Description`/`Model`/`SystemPrompt`/`Tools`/`MaxIterations`/`Policy`).
2. Porta `AgentRunner` (`RunSubAgent`) implementada **pelo motor**, mantendo a direção das dependências.
3. Tool sintética única `agents.delegate` (`{agent, task}`), publicada por `internal/engine/agents.Source`.
4. Sub-turno em sessão própria com `ParentSessionID`.
5. `MaxAgentDepth` (default 1) limita a recursão; tools do alvo filtradas por glob; política do alvo (`AgentSpec.Policy` **sobrepõe**).
6. `SubAgentEvent` opcional (type assertion) publica a delegação.

## Alternativas consideradas

- **Uma tool por agente**: enche o catálogo. Rejeitada.
- **Runner como adaptador público**: cria ciclo motor↔adaptador. Rejeitada.
- **Handoff explícito trocando de agente no mesmo loop**: adiado para v2.
- **Sem limite de profundidade**: recursão sem teto = DoS. Rejeitada.

## Consequências

- Orquestrador + N especialistas no próprio harness, sem orquestração externa.
- Cada sub-turno tem budget próprio; o custo pode crescer (documentado).
- `Config.Agents` vazio preserva exatamente o comportamento anterior.
