# Plan: Cache semântico de respostas (020)

**Feature**: `specs/nucleo/020-cache-semantico/` · **Status**: Aprovado

Constitution: **não viola** nenhuma seção. Aditivo na API pública; nenhum JSON Schema de fio/persistência muda (o cache é porta do host, não persistência do núcleo).

## Arquitetura

1. **Portas** (`internal/core/ports.go`): `SemanticCache interface { Lookup(ctx, q CacheQuery) (*CacheEntry, bool, error); Store(ctx, q CacheQuery, e CacheEntry) error }`; `CacheQuery{UserID, Model, Key string, Vector []float32}`; `CacheEntry{Response ChatResponse, CreatedAt time.Time}`.
2. **Config** (`internal/core/config.go`): `SemanticCacheConfig{Enabled bool; MinScore float64; TTL time.Duration; MaxEntries int}`; `Config.Cache SemanticCacheConfig`; `Config.CacheStore SemanticCache`.
3. **Regra pura** (`internal/engine/semcache`): `Eligible(req ChatRequest, tools []Tool) bool`; `Key(req ChatRequest, model string) string` (hash SHA-256 de system+mensagens ordenadas); `Cosine(a, b []float32) float64`; `Better(a, b) bool`; `Valid(vector) bool`.
4. **Adaptador de memória** (`adapters/cache/inmem`): store com `sync.Mutex`, índice por usuário, TTL e eviction FIFO por `MaxEntries`; `Lookup` faz a busca por cosseno e devolve o melhor acima do limiar. Testável sem rede.
5. **Wiring** (`internal/engine/loop.go`): antes de `chat`, se elegível: `Embedder.Embed(texto canônico)` → `Lookup`; hit ⇒ responde sem provedor; miss ⇒ após a resposta, `Store`. Toda falha vira miss (WARN).
6. **Observabilidade** (`internal/core/events.go`): `CacheEvent{SessionID, Model, Hit, Score, Key, SavedMicros}` + `CacheHandler` opcional. `TurnResult.Cache *CacheInfo{Key, Score, Hit}`; `Usage` de hit = zeros com `Estimated=false`.

## Contratos (aditivos)

- `SemanticCache`, `CacheQuery`, `CacheEntry`, `SemanticCacheConfig`, `CacheInfo`, `CacheEvent`, `CacheHandler`; aliases em `harness/alias.go`.

## Alternativas consideradas

- **Cache exato por hash**: barato, mas não pega reformulação; o valor do "semântico" está em similaridade. Mantido hash **como chave** + vetor para busca.
- **Cachear turnos com tools**: poderia devolver tool calls já executadas/estado obsoleto. Proibido (FR-SC-002).
- **Store próprio no núcleo**: viola "sem banco" (constitution §2/§9); porta do host + adapter inmem.
- **Embeddar no núcleo**: o `Embedder` é porta; o núcleo só orquestra.

## Riscos

- Custo do embedding a cada turno elegível; mitigado por só rodar quando `Enabled` e sem tools.
- Falso positivo semântico: limiar alto default (0,9) e isolamento por usuário.
- Respostas com mídia/não-determinismo: barradas pela elegibilidade.
