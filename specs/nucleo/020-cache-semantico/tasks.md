# Tasks: Cache semântico de respostas (020)

**Input**: `specs/nucleo/020-cache-semantico/` (spec · plan · use-cases)
**Pré-requisitos**: plan aprovado. Cada task fecha com teste AAA e `make verify` verde.

## Fase 1 — Contratos aditivos

- [x] T2001 `internal/core/ports.go`: `SemanticCache`, `CacheQuery`, `CacheEntry` — FR-SC-001
- [x] T2002 `internal/core/config.go`: `SemanticCacheConfig` + `Config.Cache`/`Config.CacheStore` — FR-SC-001/009
- [x] T2003 `internal/core/events.go`: `CacheEvent` + `CacheHandler` — FR-SC-004/005
- [x] T2004 `internal/core/config.go`: `TurnResult.Cache`/`CacheInfo` — FR-SC-004
- [x] T2005 `harness/alias.go`: aliases públicos — API pública

## Fase 2 — Regra pura e adaptador

- [x] T2006 `internal/engine/semcache/cache.go`: `Eligible`, `Key`, `Cosine`, `Valid` — FR-SC-002/003
- [x] T2007 `internal/engine/semcache/cache_test.go`: elegibilidade, chave canônica, cosseno — SC-020
- [x] T2008 `adapters/cache/inmem/store.go`: store com TTL/eviction/por usuário — FR-SC-006/008
- [x] T2009 `adapters/cache/inmem/store_test.go`: TTL, eviction, isolamento por usuário — CU-SC-3/4

## Fase 3 — Wiring

- [x] T2010 `internal/engine/loop.go`: consulta/armazenamento elegível, hit sem provedor, falha ⇒ miss — FR-SC-001/004/005/007
- [x] T2011 `internal/engine/config_validate.go`: defaults do cache (MinScore/MaxEntries) + aviso quando Enabled sem store — FR-SC-006/009
- [x] T2012 `internal/engine/cache.go`: `emitCache` + `cacheInfo` — FR-SC-004/005

## Fase 4 — Testes e fechamento

- [x] T2013 `harness` teste de turno: segundo turno idêntico não chama o provedor e marca `Cache.Hit` — CU-SC-1
- [x] T2014 `harness` teste: turno com tools ignora o cache; usuário distinto não compartilha — CU-SC-2/3
- [x] T2015 `make verify` verde + ADR 0025 + CHANGELOG + docs de feature — Constitution §7

## Dependências

- T2001→T2002→T2003→T2004→T2005; T2006/T2007→T2008/T2009; T2006→T2010→T2012; T2010→T2013/T2014; tudo→T2015.
- Depende de `Embedder` (001) e 003 (motor).

## Rastreabilidade — tasks × requisitos

| Requisito | Tasks |
|---|---|
| FR-SC-001 | T2001, T2010 |
| FR-SC-002 | T2006, T2010 |
| FR-SC-003 | T2006 |
| FR-SC-004 | T2003, T2004, T2010, T2012 |
| FR-SC-005 | T2003, T2010, T2012 |
| FR-SC-006 | T2008, T2009, T2011 |
| FR-SC-007 | T2010 |
| FR-SC-008 | T2008, T2009, T2014 |
| FR-SC-009 | T2002, T2011 |
| CU-SC-1..4 | T2013, T2014 |
