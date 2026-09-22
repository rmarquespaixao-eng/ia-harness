---
name: "ia-harness — System Directives & Operating Guidelines"
description: "Instruções de operação para agentes de IA neste repositório (lido por codex, opencode, Cursor e também pelo GitHub Copilot via .github/AGENTS.md)."
version: "1.0.0"
author: "rafael"
---

# ia-harness — AGENTS.md (repositório)

> **Fonte única de verdade canônica (global):** `~/workspace/AGENTS.md`
> (aplica-se a todos os repos deste ambiente). Se houver divergência,
> **o global vence** e este deve ser corrigido.

## Boot (agente, antes de agir)
1. Leia a fonte canônica global `~/workspace/AGENTS.md` (SDD/TDD, `model_routing`,
   segurança OWASP, convenções por stack, remote de repos).
2. Leia o índice de memória `/mnt/c/Users/rafael/ObsidianVault/agent-memory/MEMORY.md`
   quando precisar de contexto arquitetural/histórico.

## Este repositório
**ia-harness** — github.com/rmarquespaixao-eng/ia-harness (stack: `go`).

### Build / teste
- `go build ./...`
- `go test ./...`
- `go vet ./...`

### Guideline de stack (resumo do canônico global)
- TDD estrito: teste em Red antes do código; nunca afrouxar contratos/tests sem a
  ordem `"UPDATE SPECIFICATION"`.
- SDD: não escrever código de feature (Track A/B) sem `constitution`/`spec`/`plan`/`tasks`
  aprovados. Chore/Track C dispensa.
- Racional: propor solução + 2 alternativas com trade-offs antes de codar.
- Diagramas de arquitetura/fluxo em **ASCII**, nunca Mermaid.
- Segurança OWASP: subprocessos com argumentos explícitos, input schema estrito.

## Remote e git
- **Todos** os repositórios — públicos e privados — vão para o **GitHub**
  (`github.com/rmarquespaixao-eng`); Gitea homelab não é mais destino.
- Commit atômico por task, Conventional Commits, sem quebrar build.

## ARQUIVOS DE DIRETRIZES POR CLI
- **codex / opencode / Cursor:** `AGENTS.md` (raiz)
- **Claude Code:** `CLAUDE.md`
- **GitHub Copilot (VS Code):** `.github/copilot-instructions.md` e `.github/AGENTS.md`

Todos apontam para a fonte canônica global `~/workspace/AGENTS.md`.
