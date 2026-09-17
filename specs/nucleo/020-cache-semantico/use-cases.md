# Use Cases: Cache semântico de respostas (020)

Formato fully-dressed (Cockburn) + Gherkin. O operador implementa; o agente audita.

## CU-SC-1 — Pergunta reformulada acerta o cache

- **Identificador**: CU-SC-1 · **Escopo**: motor do harness · **Nível**: subfunção
- **Ator primário**: host (biblioteca embarcada)
- **Partes interessadas e interesses**: operador (economia de chamadas pagas); usuário (resposta rápida e consistente)
- **Pré-condições**: `Cache.Enabled=true`; `Embedder` e `SemanticCache` injetados; turno elegível (sem tools, sem output schema)
- **Gatilho**: chamada de `Run`
- **Garantia de sucesso**: pergunta semanticamente equivalente não chama o provedor
- **Garantia mínima**: sem hit, o fluxo normal do provedor permanece
- **Mapeamento técnico**: `semcache.Eligible` → `Embedder.Embed` → `SemanticCache.Lookup` → resposta cacheada
- **Fluxo principal de sucesso**:
  1. O motor avalia a elegibilidade do turno.
  2. Gera a chave canônica e o vetor do contexto.
  3. O store devolve a melhor entrada com cosseno ≥ `MinScore`.
  4. O motor conclui o turno com a resposta cacheada, custo zero e `CacheEvent{hit:true}`.
- **Fluxos alternativos e de exceção**:
  - `1a` Turno com tools/output schema/params não-determinísticos: cache ignorado.
  - `3a` Nenhuma entrada acima do limiar: miss; após a resposta do provedor, armazena.
  - `3b` `Embedder`/store falha: miss, log WARN, turno segue.
- **Regras de negócio**: RN-1 isolamento por `UserID`; RN-2 TTL e teto de entradas; RN-3 só turnos determinísticos-sem-efeito.
- **Critérios de aceite (Gherkin)**:
```gherkin
Cenário: segundo turno idêntico
  Dado um embedder fake e o cache ligado
  Quando dois turnos idênticos rodam em sequência
  Então o provedor é chamado uma vez e o segundo marca Cache.Hit=true
```
- **Rastreabilidade**: FR-SC-001..005.

## CU-SC-2 — Turno com tools não usa cache

- **Identificador**: CU-SC-2 · **Escopo**: motor · **Nível**: subfunção
- **Ator primário**: host
- **Garantia de sucesso**: nunca servir resposta que dependa de tool calls/estado
- **Fluxo principal**: com tools no catálogo do turno, `Eligible=false`; provedor chamado sempre.
- **Rastreabilidade**: FR-SC-002.

## CU-SC-3 — Isolamento por usuário

- **Identificador**: CU-SC-3 · **Escopo**: motor/store · **Nível**: subfunção
- **Garantia mínima**: resposta de A nunca é servida a B
- **Fluxo principal**: chave de índice inclui `UserID`; lookup filtra pelo usuário.
- **Rastreabilidade**: FR-SC-008.

## CU-SC-4 — Expiração e teto

- **Identificador**: CU-SC-4 · **Escopo**: adaptador inmem · **Nível**: subfunção
- **Garantia de sucesso**: entradas expiradas não são servidas; o store não cresce sem limite
- **Fluxo principal**: no lookup/armazenamento, entradas com `now > CreatedAt+TTL` são descartadas; acima de `MaxEntries`, a mais antiga sai.
- **Rastreabilidade**: FR-SC-006.

## Requisitos especiais (NFR)

- Determinismo: embedder e store fake nos testes; nenhum teste toca rede.
- Observabilidade: `CacheEvent` + `TurnResult.Cache` com chave/score/hit.
- Sem breaking change: campos/portas aditivos; `Enabled=false` preserva o comportamento.
