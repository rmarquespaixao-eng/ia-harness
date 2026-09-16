# ADR 0022 — Módulo público no GitHub com `module path` qualificado

**Status**: Accepted
**Data**: 2026-09-16

## Contexto

O `go.mod` declarava `module rmarquespaixao/ia-harness` — sem ponto no primeiro
elemento do caminho. O Go só resolve um módulo cujo primeiro elemento de path é um
domínio; qualquer tentativa de baixá-lo falha:

```
go: rmarquespaixao/ia-harness@v0.1.0: malformed module path:
    missing dot in first path element
```

O primeiro consumidor (`financeiro-api-v2`, feature 023) usa
`replace rmarquespaixao/ia-harness => ../ia-harness`. Isso funciona no checkout local,
mas o CircleCI da API faz `go mod download` depois do `checkout` e **não** tem o
diretório irmão — logo o `replace` quebra a esteira. Para o consumidor depender do
módulo por **tag**, o harness precisa de um `module path` válido e de um remote que o
`go mod download` do CI alcance. O repositório local não tinha remote algum e
`github.com/rmarquespaixao-eng/ia-harness` ainda não existia. O harness é uma
**biblioteca sem segredos** (não abre banco, não autentica usuário, não guarda chave),
então publicação pública é compatível com a constitution (§3: persistência só por portas
do host).

## Decisão

1. Renomear o `module path` para **`github.com/rmarquespaixao-eng/ia-harness`**,
   atualizando todos os imports e artefatos.
2. Publicar o repositório como **público no GitHub**
   (`github.com/rmarquespaixao-eng/ia-harness`) — o CircleCI o baixa sem credencial.
3. O consumidor passa a `require github.com/rmarquespaixao-eng/ia-harness v0.1.0`,
   **sem** `replace`, após o push da tag.

O `replace` local continua válido durante a transição, agora apontando para o path novo
(`replace github.com/rmarquespaixao-eng/ia-harness => ../ia-harness`).

## Alternativas consideradas

- **Gitea privado (`git.homelab-cloud.com/admin/ia-harness`) + `GOPRIVATE`**: mantém o
  repo no homelab, mas exige credencial de leitura no CircleCI (cloud) e configuração de
  proxy/git para o host — mais superfície de operação e um ponto de falha externo ao CI.
  Rejeitada para o v1.
- **`vendor/`**: commit dos fontes do harness dentro da API; elimina a dependência de
  rede, mas duplica o código, perde a tag SemVer e o `go.sum` deixa de rastrear a versão
  real. Rejeitada.
- **Manter `replace` local para sempre**: a esteira nunca passa a resolver o módulo; o
  build de CI fica impossível. Rejeitada.
- **`git.homelab-cloud.com/...` com cliente já autenticado**: o CI é cloud; depender do
  acesso à VPN/homelab em todo build é frágil. Rejeitada.

## Consequências

- Rename mecânico do module path: `go.mod` + imports em 95 arquivos do harness e 8 da
  API, além de README/CHANGELOG/specs/docs (`docs/features`, contratos). `go.sum` não
  guarda o próprio módulo; nenhum símbolo público muda.
- O ADR 0034 do `financeiro-api-v2` e o doc `features/assistente.md` já citavam o link
  do GitHub; passam a ser a verdade.
- Procedimento de publicação vira runbook de operador (criar repo, push, tag e troca do
  `replace` no consumidor) — `docs/runbooks/2026-09-16-publicar-modulo-github.md`.
- `make verify` deve permanecer verde nos dois repositórios (mudança puramente mecânica).
