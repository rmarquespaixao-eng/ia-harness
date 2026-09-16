# ADR 0020 — Execução paralela de tools independentes

**Status**: Accepted
**Data**: 2026-09-16
**Feature**: `specs/nucleo/015-tools-paralelo`

## Contexto

Tool calls independentes do mesmo turno executavam em sequência. Paralelizar sempre é perigoso: tools destrutivas/confirmáveis exigem decisão humana e ordem determinística.

## Decisão

`Config.ParallelTools` (default desligado). Com ele ligado e >1 tool call, o harness **pré-resolve, valida e autoriza todas** (`planParallel`); só paraleliza se **todas** forem permitidas e nenhuma exigir confirmação. Caso contrário, cai no caminho sequencial. No lote seguro, as tools rodam em goroutines e os **eventos/resultados/histórico saem na ordem original** (a coleta é indexada; a emissão é posterior e sequencial). Progresso pode ser emitido concorrentemente — o `Handler` precisa ser seguro.

## Alternativas consideradas

- **Paralelizar sempre**: violaria confirmação/ordem. Rejeitada.
- **Paralelizar por tool, intercalando com sequencial**: complexidade alta e eventos fora de ordem. Rejeitada.
- **Limite de concorrência configurável**: adiado (lote típico é pequeno).

## Consequências

- Latência menor em lotes de leitura; semântica preservada (ordem, auditoria, materialização).
- `MemToolSource` recebeu mutex (testutil) para suportar concorrência; testes com `-race` verdes.
- Testes: `harness/parallel_test.go` (paralelo em ordem + confirmação cai no sequencial).
