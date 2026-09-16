# ADR 0008 — Agrupamento dos adaptadores sob `adapters/`

**Status**: Accepted
**Data**: 2026-09-15

## Contexto

Após a implementação do núcleo, a raiz do repositório acumulou 8 diretórios de pacote sem critério visível: `harness/`, `provider/`, `mcpclient/`, `store/`, `memory/`, `embed/`, `audit/` e `internal/`. O operador apontou a estrutura como "solta e confusa". Problemas concretos identificados: (a) nada no layout distinguia o núcleo dos adaptadores de I/O e das ferramentas; (b) o token "memory" aparecia em três lugares com papéis diferentes (`store/memory` = sessão; `memory/inmem` = fatos de longo prazo; `audit/memory` = sink em memória; e ainda `harness/memory_context.go`); (c) `provider/` estava agrupado, mas os demais adaptadores ficavam soltos na raiz.

## Decisão

Agrupar **todo o I/O** sob `adapters/` e renomear para eliminar ambiguidade:

```
adapters/
  provider/{openai,anthropic}/   # Provider (modelo)
  mcpclient/                     # ToolSource (MCP)
  session/memory/                # SessionStore (antes store/memory)
  memory/inmem/                  # MemoryStore + Retriever (fatos)
  embed/openai/                  # Embedder
  audit/{log,mem}/               # AuditSink (antes audit/log e audit/memory)
```

Regra de leitura única: `harness/` = contrato + núcleo (modelo, portas, loop, política, janelas); `adapters/` = tudo que fala com o mundo externo, injetado por DI; `internal/platform/` = infra compartilhada não pública; `contracts/ cmd/ examples/ docs/ specs/ scripts/` = artefatos de engenharia. Os **nomes de pacote permanecem** (`memory`, `openai`, `anthropic`, `mcpclient`); muda só o caminho de import. A decisão foi tomada antes de qualquer commit/tag, então sem custo de compatibilidade. Regeneração do grafo (`graphify`) e atualização dos artefatos SDD (constitution §3, plan §Project Structure, `docs/features`, README) fazem parte da mudança.

## Alternativas consideradas

- **Manter o layout plano**: menos movimento agora, mas mantém a ambiguidade e a percepção de desorganização (rejeitada pelo operador).
- **`internal/adapters/…`**: impossível — o host importa os adaptadores para DI; `internal/` é inimportável por outro módulo (mesma restrição registrada em ADR 0006).
- **Quebrar também `harness/` em `internal/engine`**: refactor significativo (métodos do `Harness` e testes internos), sem ganho imediato de clareza; fica como evolução futura se necessário.
- **Nome `components/`/`integrations/`**: `adapters` preserva o vocabulário de portas/adaptadores já usado na constitution.

## Consequências

- `import "rmarquespaixao/ia-harness/adapters/..."` em hosts e exemplos; nenhum símbolo público muda.
- `docs/features/nucleo-harness.md`, README, constitution §3 e plan §Project Structure passam a apontar os novos caminhos; grafo regenerado.
- `make verify` deve permanecer verde sem qualquer alteração de comportamento (mudança puramente mecânica).
