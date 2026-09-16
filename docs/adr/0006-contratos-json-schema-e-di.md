# ADR 0006 — Contratos em JSON Schema gerado e injeção explícita de adaptadores

**Status**: Accepted
**Data**: 2026-09-15

## Contexto

O harness nasce espelhando a arquitetura do financeiro-api-v2 (R14/D-11..D-13): package-by-feature, regra pura separada de I/O e contrato versionado. Duas fronteiras, porém, mudam quando o produto é biblioteca e não serviço.

Primeira: os contratos de fio/persistência (config, session, events, audit) precisam de fonte única; escrever structs à mão em paralelo a schemas/documentação é a receita clássica de drift entre núcleo, host e testes, e o host (financeiro-ui) quer gerar tipos TS a partir da mesma fonte.

Segunda: `internal/features` — o padrão do financeiro para adaptadores — não funciona em biblioteca: um pacote `internal` é inimportável por outro módulo, e adaptadores públicos importando o núcleo que os orquestra criariam ciclo. O host precisa montar os adaptadores (DI) e o núcleo só validar e compor.

## Decisão

1. **JSON Schema é a fonte única dos contratos**: `contracts/<assunto>/*.json` (assunto = config, session, events, audit) define os contratos de fio/persistência; nenhum tipo público serializável é definido primeiro em Go. A evolução do schema acompanha a evolução da API pública da biblioteca.

2. **Structs geradas, nunca à mão**: `cmd/contractgen` roda `github.com/atombender/go-jsonschema` (declarado como tool directive no `go.mod`) sobre os schemas e grava `contracts/gen`, commitado. O embed dos schemas fica em `contracts/schemas.go`, consultado em runtime pela validação.

3. **Gate de geração sem diff**: `go generate ./...` entra no `make verify`; se o gerado divergir do commitado, o gate quebra. O mesmo gate exige `gofmt` limpo, `go vet`, `staticcheck`, testes, build e `govulncheck`.

4. **Teste de conformidade do marshal**: cada tipo público de fio/persistência é serializado em teste e validado contra o schema correspondente; snapshot de sessão e evento de auditoria nascem desse contrato — drift entre Go e schema vira teste vermelho, não bug de produção.

5. **TS do host a partir dos mesmos schemas**: o financeiro-ui pode gerar seus tipos TypeScript dos mesmos JSON Schemas; a UI não redescobre o formato de eventos/sessão.

6. **Modelo e portas públicos no pacote `harness`**: o pacote `harness` faz o papel do `internal/core` do financeiro (modelo canônico, portas, fachada, motor puro), público por ser o contrato de importação da biblioteca.

7. **Adaptadores públicos e DI explícita**: cada adaptador é um pacote próprio (`adapters/provider/openai`, `adapters/provider/anthropic`, `mcpclient`, stores/audit em memória), construído pelo host no padrão `cmd/api/di.go` e injetado em `Config.Providers`/`Config.Tools`; `harness.New` só valida (porta obrigatória, perfil órfão, schema inválido) e compõe — nunca instancia adaptador por `kind` (D-12).

8. **`internal/platform` para infra não pública** (clock, schema, retry, trace, testutil) e **`cmd/harnessctl` como CLI de dev/smoke** — roda um turno, imprime eventos, valida contratos; sem garantia de produto (D-13).

## Alternativas consideradas

| Alternativa | Motivo da rejeição |
|---|---|
| Structs à mão + documentação | Drift garantido entre núcleo, host e testes; sem gate mecânico para detectar. |
| OpenAPI ou protobuf como fonte | OpenAPI não cobre persistência/eventos internos; protobuf adiciona toolchain e geração binária sem ganho para JSON de fio/persistência. |
| Só documentação em `contracts/*.md` | Serve ao design, mas não gera código nem sustenta teste de conformidade. |
| Registry com `init()`/blank-import (padrão drivers SQL) | Acoplamento global invisível, difícil de testar e de ler na DI do host; conflita com falha rápida. |
| `Config` com `Kind` e construção interna | O núcleo passaria a importar cada adaptador — ciclo e dependência pesada na biblioteca; trocar adapter exigiria mudança no núcleo. |
| Adaptadores em `internal/features` (como no financeiro) | `internal` é inimportável por outro módulo; e o adaptador importando o núcleo que o orquestra gera ciclo. |
| Sem `cmd/harnessctl` | Smoke real e diagnóstico ficariam por conta de cada host; um CLI de dev barato reduz atrito sem virar produto. |

## Consequências

**Positivas**: fonte única elimina drift núcleo × host × UI; geração com gate torna a violação erro de CI; DI explícita mantém o núcleo ignorante de SDKs e permite trocar provedor por configuração; a API pública permanece pequena e estável; `cmd/harnessctl` dá smoke sem serviço.

**Negativas/trade-offs**: exige etapa de geração e disciplina de commit do `contracts/gen` (o gate compensa); `go-jsonschema` é dependência de toolchain, com pin e atualização deliberada; mudança de schema quebra hosts — evolução pede aditividade/versionamento; DI explícita transfere ao host um wiring repetitivo, mitigado pelo `examples/financeiro`; o CLI de dev pode acumular escopo se não for tratado como não-produto.
