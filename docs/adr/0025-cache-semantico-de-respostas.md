# ADR 0025 — Cache semântico de respostas

**Status**: Accepted
**Data**: 2026-09-17
**Feature**: `specs/nucleo/020-cache-semantico`

## Contexto

Turnos idempotentes e sem efeito (prompts repetidos ou reformulados) pagam o provedor de novo. Um cache **exato** por hash não cobre a reescrita — que é o caso comum. Em sentido oposto, cachear turnos com tool calls devolveria efeitos já executados ou estado obsoleto. O cache precisa ser seguro por construção, não por disciplina do host.

## Decisão

Cache **só para turnos determinísticos e sem efeito**: elegível apenas sem tools, sem tool calls na resposta, sem `OutputSchema` e sem `temperature`/`top_p`/`top_k`/`seed`. Porta `SemanticCache` (`Lookup`/`Store`) do host, reusando o `Embedder` existente — o núcleo **não** embedda nem mantém store próprio (constitution §2/§9). A chave é SHA-256 canônico do sistema + mensagens + modelo; a busca é por **cosseno** com `MinScore` (default 0,9) e isolamento por `UserID`; TTL e eviction (FIFO por `MaxEntries`) no adapter `adapters/cache/inmem`. Toda falha de embedder/store vira **miss** (WARN), nunca erro do turno. Hit **não chama o provedor**: `Usage` zero com `Estimated=false`, `TurnResult.Cache` e `CacheEvent` publicados. Regra pura em `internal/engine/semcache/cache.go`; wiring em `internal/engine/cache.go`.

## Alternativas consideradas

- **Cache exato por hash apenas**: barato, mas não pega reformulação — o valor está na similaridade. Mantido o hash **como chave**, com vetor para busca. Rejeitada a versão pura.
- **Cachear turnos com tools**: devolveria efeitos/estado obsoleto. Proibido (FR-SC-002). Rejeitada.
- **Store próprio no núcleo**: violaria "sem banco" (§2/§9). Porta do host + adapter inmem. Rejeitada.
- **Embeddar dentro do núcleo**: o `Embedder` é porta; o núcleo só orquestra. Rejeitada.

## Consequências

- Economia de chamadas pagas em turnos elegíveis; `Enabled=false` preserva exatamente o comportamento anterior.
- Custo de um embedding por turno elegível — mitigado por só rodar com cache ligado e sem tools.
- `MinScore` alto (0,9) evita falso positivo semântico; o isolamento por `UserID` evita vazamento entre contas.
- Release MINOR (portas/campos aditivos); testes `semcache/cache_test.go`, `adapters/cache/inmem/store_test.go` e wiring com fakes.
