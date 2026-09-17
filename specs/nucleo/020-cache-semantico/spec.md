# Feature Specification: Cache semântico de respostas

**Feature Branch**: `nucleo/020-cache-semantico` · **Status**: Draft (aguardando portão)

**Input**: pedido do operador — Fase D (P2): "cache semântico".

## Problema

Perguntas equivalentes repetem a mesma chamada de modelo paga. O harness não tem memória de respostas: cada turno vai ao provedor. Um cache **semântico** (busca por similaridade de embedding, não igualdade exata) evita chamadas repetidas em perguntas reformuladas, sem quebrar determinismo quando não há tools envolvidas.

## Requisitos (EARS)

- **FR-SC-001**: The harness SHALL consultar um cache semântico antes da chamada de modelo quando `Config.Cache.Enabled` e as portas `Embedder` + `SemanticCache` estiverem injetadas.
- **FR-SC-002**: A elegibilidade MUST ser restrita a turnos determinísticos e sem efeitos: **sem tools no turno**, resposta **sem tool calls**, **sem** `OutputSchema` e **sem** parâmetros não-determinísticos (`temperature`/`top_p` em `Params`).
- **FR-SC-003**: A chave do cache MUST ser derivada do conteúdo canônico do contexto (system + mensagens) e do modelo efetivo; a similaridade MUST usar a métrica de cosseno com limiar `Config.Cache.MinScore`.
- **FR-SC-004**: WHEN a resposta vem do cache, o harness MUST NÃO chamar o provedor, MUST emitir `CacheEvent{hit:true, score}` e MUST registrar `Usage` com custo zero e `Estimated=false`; o resultado marca `TurnResult.Cache`.
- **FR-SC-005**: WHEN há miss elegível, após a resposta do provedor o harness MUST armazenar a entrada (com vetor e TTL) e emitir `CacheEvent{hit:false}`.
- **FR-SC-006**: Entradas MUST expirar por TTL (`Config.Cache.TTL`) e o armazenamento MUST respeitar um teto de entradas (`Config.Cache.MaxEntries`), removendo as mais antigas.
- **FR-SC-007**: Falha do `Embedder`/`SemanticCache` MUST NOT derrubar o turno: vira miss e o fluxo segue (log WARN).
- **FR-SC-008**: O cache MUST ser isolado por usuário (`UserID`) — uma resposta nunca é servida a outro usuário.
- **FR-SC-009**: `Config.Cache.Enabled=false` (default) MUST preservar o comportamento anterior byte a byte.

## Critérios (Gherkin)

```gherkin
Cenário: pergunta reformulada acerta o cache
  Dado cache ligado, limiar 0,9 e um embedder determinístico por texto normalizado
  Quando a segunda pergunta é semanticamente idêntica à primeira (mesmo texto canônico)
  Então o provedor é chamado uma única vez e o segundo turno marca Cache.Hit

Cenário: turno com tools não usa cache
  Dado um turno com tool definitions
  Quando o cache está ligado
  Então nenhuma consulta/armazenamento é feito e o provedor é sempre chamado

Cenário: cache por usuário
  Dado uma entrada armazenada para o usuário A
  Quando o usuário B faz a mesma pergunta
  Então o cache não devolve a resposta de A

Cenário: TTL expira
  Dado uma entrada com TTL expirado
  Quando a mesma pergunta chega
  Então é tratada como miss
```

## Casos de borda

- `MinScore` ≤ 0 ⇒ default 0,9; `MaxEntries` ≤ 0 ⇒ default 1000; `TTL` ≤ 0 ⇒ sem expiração.
- Embedder devolve vetor vazio/dimensão divergente: descarta a entrada (nunca compara lixo).
- `SemanticCache` nil mas `Enabled=true`: cache desligado (log WARN no boot).
- Mídia no contexto: entra na chave pelo texto disponível; não é objetivo cachear mídia no v1.

## Não-objetivos (v1)

- Cache de **tool calls**/resultados (efeitos colaterais) — proibido por design.
- Invalidação por evento de escrita do host (o host pode limpar o store).
- Persistência própria: o store é porta do host; o núcleo oferece `adapters/cache/inmem` para dev/teste.

## Dependências e espelho

- Depende de `Embedder` (feature 001, adaptador `embed/openai`) e da fachada (003).
- Host: `financeiro-api-v2` decide o store (ex.: pgvector) e o limiar.
