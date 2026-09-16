# ADR 0004 — Custo híbrido e memória/RAG apenas por portas

**Status**: Accepted
**Data**: 2026-09-15

## Contexto

O núcleo do harness é uma biblioteca embutível (D-HAR-1) que não fala com billing de provedores nem abre banco de dados. Ainda assim precisa de duas capacidades que, em produtos fechados, costumam vir embutidas: visibilidade de custo por chamada de modelo para a trilha de auditoria (FR-025) e memória/RAG por usuário (FR-027..FR-029, R5/R6).

Nenhum provedor garante reporte de tokens em todos os modos de streaming — a API OpenAI só devolve `usage` no chunk final quando solicitado, e endpoints compatíveis variam. Integrar o billing de cada provedor é escopo e lock-in que a constitution §2/§9 não admite. Para memória, embutir armazenamento violaria a fronteira de biblioteca (constitution §3): o núcleo não pode depender de Postgres, pgvector ou sqlite sem arrastar a escolha do host. O financeiro, primeiro consumidor, já resolveu o mesmo problema pela via híbrida (ADR 0032) e será dono do índice real, o que permite manter a mesma semântica de rótulo e premissa.

## Decisão

1. **Uso reportado tem precedência**: quando o provedor devolve `Usage` (entrada, saída, cache quando houver), esses valores são canônicos e o evento de auditoria sai com `estimated=false`.

2. **Sem reporte, estimativa rotulada**: o núcleo estima tokens por volume de texto medido em bytes **antes** da redação (o truncamento de trilha não deve subestimar a conta) ÷ premissa `bytes_per_token`, e converte em custo via `Pricing` (entrada/saída em micros por 1M tokens, `currency`). O evento sai com `estimated=true` — precisão não é prometida, é rotulada (D-07/R5).

3. **Premissa do host, sem congelamento**: `Pricing` vem da configuração do host; ajustá-la reflete nos turnos seguintes. O núcleo não persiste tabela de preços nem faz backfill — mesma postura "on-read" do ADR 0032.

4. **Extensão `_meta` opcional**: como cliente MCP, o harness pode preencher `_meta["financeiro/usage"]` quando o host habilitar, fechando o ciclo com o servidor MCP do financeiro; sem host habilitado, o campo não é enviado.

5. **Memória e RAG só por portas**: o núcleo define `MemoryStore`, `Retriever` e `Embedder` e entrega apenas implementações em memória para dev/teste (`adapters/memory/inmem`). Persistência, índice e ciclo de vida dos fatos — incluindo eventual pgvector — pertencem ao host: o núcleo não cria armazenamento próprio nem exige vector store (D-08/R6).

6. **Embeddings default OpenAI-compatible**: o `Embedder` default do host fala `/v1/embeddings` compatível com OpenAI, opcional na configuração; sem ele, memória declarativa segue funcionando e RAG semântico fica indisponível sem quebrar o turno.

## Alternativas consideradas

| Alternativa | Motivo da rejeição |
|---|---|
| Só uso reportado (sem estimativa) | Métricas e custo ficariam vazios em parte dos provedores/modos de streaming; a trilha perderia FR-025 verificável. |
| Contagem exata com tokenizer por modelo | Dependência pesada e desatualizada a cada release; ainda heurística para mensagens de sistema — fica como porta `Tokenizer` opcional fora do v1. |
| Embutir vector store (embeddings local) no núcleo | Viola a constitution §3/§9 e decide pelo host o que é decisão do host; inviabiliza a biblioteca como dependência leve. |
| Exigir pgvector no núcleo | Acopla a biblioteca ao Postgres do financeiro; hosts diferentes teriam de adotar a mesma stack de banco. |
| Custo exato via billing do provedor | Escopo não-objetivo: cada provedor expõe API distinta e o núcleo não conversa com eles. |

## Consequências

**Positivas**: a trilha nasce útil (custo estimado rotulado) e melhora sozinha quando o provedor reporta ou quando o cliente preenche `_meta`; o rótulo `estimated` permite à UI separar reportado × estimado; a biblioteca continua leve e sem banco; o host mantém soberania sobre memória (índice, retenção) e premissa de preço.

**Negativas/trade-offs**: a estimativa por `bytes_per_token` (default ~4) é heurística e não é fatura — o rótulo existe justamente para não haver leitura equivocada; sem `Embedder`, o RAG semântico degrada (explícito no `Config`); premissa alterada muda números retroativamente na leitura; o ciclo host→harness→servidor MCP via `_meta` só fecha quando o host habilitar a extensão e a validação server-side (allowlist, teto de 1 KiB) existir.
