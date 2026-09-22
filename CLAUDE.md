# CLAUDE.md — diretrizes do repositório ia-harness (Claude Code)

> **Fonte única de verdade canônica (global):** `~/workspace/AGENTS.md`
> (aplica-se a todos os repos deste ambiente). Se houver divergência,
> **o global vence** e este deve ser corrigido.

## 1. Boot obrigatório (a cada sessão)
1. Leia a fonte canônica global `~/workspace/AGENTS.md` (SDD/TDD, `model_routing`,
   segurança OWASP, convenções por stack, remote de repos).
2. Leia o índice de memória `/mnt/c/Users/rafael/ObsidianVault/agent-memory/MEMORY.md`
   quando precisar de contexto arquitetural/histórico.

## 2. Este repositório
**ia-harness** — github.com/rmarquespaixao-eng/ia-harness (stack: `go`).

### Build / teste
- `go build ./...`
- `go test ./...`
- `go vet ./...`

## 3. Todo o restante do processo
Siga o canônico global `~/workspace/AGENTS.md`:
- **TDD estrito**: teste em Red antes do código; nunca alterar teste sem `"UPDATE SPECIFICATION"`.
- **SDD**: não escrever código de feature (Track A/B) sem artefatos aprovados; Chore/Track C dispensa.
- **Racional**: propor solução + 2 alternativas com trade-offs antes de codar.
- Diagramas de arquitetura/fluxo em **ASCII**, nunca Mermaid.
- **Git Hygiene**: commit atômico por task, Conventional Commits.
- **Remote**: **todos** os repositórios no **GitHub** (`github.com/rmarquespaixao-eng`).

## 4. ARQUIVOS DE DIRETRIZES POR CLI
- **codex / opencode / Cursor:** `AGENTS.md` (raiz)
- **Claude Code:** `CLAUDE.md`
- **GitHub Copilot (VS Code):** `.github/copilot-instructions.md` e `.github/AGENTS.md`

