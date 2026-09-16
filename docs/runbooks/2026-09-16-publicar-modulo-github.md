# Runbook — Publicar o módulo `ia-harness` no GitHub (ADR 0022)

- **Data:** 2026-09-16
- **Repo:** `ia-harness`
- **Decisão:** `docs/adr/0022-modulo-publico-github-e-path-qualificado.md`
- **Status:** preparado pelo agente; **pendente de execução do operador**

## Objetivo

Publicar o harness como módulo público `github.com/rmarquespaixao-eng/ia-harness`
(repo + tag `v0.1.0`) e trocar o `replace` local do `financeiro-api-v2` pela tag,
destravando o commit da feature 023 na esteira do CircleCI.

## Contexto

O `module path` antigo (`rmarquespaixao/ia-harness`) não tem domínio e o Go não o
resolve (`missing dot in first path element`). O rename para o path qualificado já está
feito na árvore de trabalho dos dois repositórios, mas **não commitado**. Enquanto o
módulo não estiver publicado, o `replace ../ia-harness` mantém o build local; o CI da
API faz `go mod download` e precisa da tag pública.

## Pré-condições

- `gh` autenticado como `rmarquespaixao-eng` com escopo `repo` (`gh auth status`).
- Identidade Git configurada no `ia-harness` (`user.name`/`user.email`).
- Árvores limpas exceto pelo rename em andamento e pela feature 023 no `financeiro-api-v2`.
- Nenhum segredo no diff.

## Passos

1. **Gate do harness no commit do rename.**
   ```bash
   cd ~/workspace/Gitea/ia-harness
   make verify
   ```
   O que faz: gofmt, vet, staticcheck, `go generate` sem diff, testes, build e
   govulncheck. Saída esperada: exit 0, sem diff em `contracts/gen`.

2. **Revisar e commitar o rename.**
   ```bash
   git status --short
   git diff --stat
   git add -A
   git commit -m "refactor(module): migra para github.com/rmarquespaixao-eng/ia-harness (ADR 0022)"
   ```
   Como verificar: `git show --stat HEAD` deve listar `go.mod` + imports + docs, sem
   arquivos de segredo.

3. **Recriar a tag no commit do rename.** A tag `v0.1.0` atual aponta para `af05370`
   (path antigo) e **nunca foi publicada**; movê-la é seguro.
   ```bash
   git tag -d v0.1.0
   git tag -a v0.1.0 -m "release v0.1.0"
   git tag -n1
   ```
   Saída esperada: `v0.1.0 release v0.1.0` apontando para o commit do rename.

4. **Criar o repositório público e publicar.**
   ```bash
   gh repo create rmarquespaixao-eng/ia-harness --public --source=. --remote=origin \
     --description "Harness de IA em Go: loop de agente com tool-calling, MCP, multi-provedor e sessões (biblioteca embutível)."
   git push -u origin master
   git push origin v0.1.0
   ```
   O que faz: cria o repo público, adiciona `origin` e publica branch + tag. Saída
   esperada: URL do repo e os refs aceitos. Como verificar:
   ```bash
   gh repo view rmarquespaixao-eng/ia-harness --json visibility,defaultBranchRef
   ```

5. **Conferir a resolução do módulo** (o proxy pode levar alguns minutos).
   ```bash
   cd /tmp && rm -rf gochk && mkdir gochk && cd gochk && go mod init chk
   GONOSUMCHECK=1 go list -m github.com/rmarquespaixao-eng/ia-harness@v0.1.0
   ```
   Saída esperada: `github.com/rmarquespaixao-eng/ia-harness v0.1.0`. Se falhar com
   "not found", aguardar 1–2 min e repetir; se persistir, `GOPROXY=direct`.

6. **Trocar o `replace` pela tag no `financeiro-api-v2`.**
   ```bash
   cd ~/workspace/Gitea/financeiro-api-v2
   go mod edit -dropreplace github.com/rmarquespaixao-eng/ia-harness
   go mod edit -require=github.com/rmarquespaixao-eng/ia-harness@v0.1.0
   go mod tidy
   make verify
   go generate ./...
   git status --short   # contracts/gen não deve mudar
   ```
   Saída esperada: `make verify` verde e `go generate` sem diff. Como verificar:
   `grep -n ia-harness go.mod` mostra `require ... v0.1.0` e **sem** `replace`.

7. **Commit da feature 023 pela esteira normal.**
   ```bash
   cd ~/workspace/Gitea/financeiro-api-v2
   git add -A
   git commit -m "feat(assistente): integra o harness de IA in-process (spec 023)"
   git push          # a branch main trackeia github/main; use `git push github main` explícito
   git push origin main   # espelha no Gitea do homelab
   ```
   O que faz: envia a feature e dispara o pipeline (CircleCI está ligado ao GitHub). Como
   verificar: CircleCI `test` + `build` verdes; `deploy_homolog` só em tag.

## Rollback

1. **Antes de publicar (passos 1–3):** descartar o rename com
   `git checkout -- . && git clean -fd` e recriar a tag com
   `git tag -f -a v0.1.0 -m "release v0.1.0" af05370`.
2. **Depois de publicar (passo 4 em diante):** tornar o repo privado ou removê-lo
   (`gh repo delete rmarquespaixao-eng/ia-harness --yes`); no consumidor, restaurar o
   `replace`: `go mod edit -replace github.com/rmarquespaixao-eng/ia-harness=../ia-harness`
   e `git checkout -- go.mod go.sum`.
3. **Não** mova a tag depois de publicada — se precisar de outra versão, corte `v0.1.1`.

## Riscos e o que NÃO fazer

- **NÃO** commitar credencial/segredo; o harness não guarda chave (chave do modelo vive
  no env do consumidor).
- **NÃO** publicar antes de `make verify` verde.
- **NÃO** deixar `replace` e `require@v0.1.0` no mesmo `go.mod` — o CI ignora a tag e usa
  o path local (inexistente no checkout do pipeline).
- O proxy pode demorar a indexar a tag; não é falha do módulo.
- A tag `v0.1.0` só pode ser movida porque nunca foi publicada.

## Registro

| Item | O que muda |
|---|---|
| Repositórios | Novo repo público `github.com/rmarquespaixao-eng/ia-harness` |
| Arquivos/configs | `go.mod` + imports dos dois repos (module path); `financeiro-api-v2/go.mod` (require por tag, sem `replace`) |
| Pacotes/versões | Harness publicado como `v0.1.0` |
| Serviços/portas | Nenhum |
| Dados | Nenhum |
| Credenciais/ENV | Nenhuma credencial nova; `gh` já autenticado |

## Resultado

**Executado em 2026-09-16 (agente, autorizado pelo operador).**

- Passo 1: `make verify` verde no harness (exit 0, `No vulnerabilities found`).
- Passo 2: commit `b8e5902` — `refactor(module): migra para github.com/rmarquespaixao-eng/ia-harness (ADR 0022)` (108 arquivos).
- Passo 3: tag `v0.1.0` recriada no commit do rename (`git tag -n1` → `v0.1.0 release v0.1.0`).
- Passo 4: repo criado em `https://github.com/rmarquespaixao-eng/ia-harness`; push de `master` e da tag. Refs remotas: `refs/heads/master` e `refs/tags/v0.1.0^{}` = `b8e5902`.
- Passo 5: `go list -m github.com/rmarquespaixao-eng/ia-harness@v0.1.0` → `v0.1.0` (proxy OK na 1ª tentativa).
- Passo 6: no `financeiro-api-v2`, `dropreplace` + `require v0.1.0` + `go mod tidy` (baixou o módulo; `go.sum` com `h1:` e `/go.mod`); `make verify` verde e `go generate ./...` sem diff.
- Passo 7: commits `b595dd0` (chore gitignore) e `5780730` (`feat(assistente)`), publicados em `github/main` e `origin/main` (Gitea).

**Desvios:** (a) o passo 4 usou `gh repo create` + `git push -u origin master` + `git push origin v0.1.0` — sem `--push` no `create`; (b) o passo 7 tinha `git push origin main`, mas a `main` do financeiro trackeia `github/main`; corrigido acima e publicado nos dois remotes. **Pendente:** conferir o pipeline do CircleCI e o smoke real do assistente.
