# ADR 0015 — Loader do arquivo de configuração

**Status**: Accepted
**Data**: 2026-09-16
**Feature**: `specs/nucleo/008-config-loader`

## Contexto

O schema `contracts/config/harness_config.json` (JSON Schema → `contracts/gen/config.go`) era a fonte do contrato do arquivo de configuração, mas nenhum código o lia. O host precisava montar `harness.Config` inteiramente à mão, contrariando o "pronto pra configurar".

## Decisão

Criar o pacote público `adapters/config` (I/O de arquivo mora em `adapters/`, ADR 0008) com `Load(path)`/`Parse(raw)` → `File`, validando contra o schema embutido (`contracts.Schema` + `internal/platform/schema.Compile/Validate`) e interpretando no tipo gerado (`gen.HarnessConfigFile`). `File.Apply(*harness.Config)` mescla os campos de configuração; `File.MCPServers()` expõe os descritores de servidor para o host montar o `mcpclient`. Providers, Tools, portas e credenciais **não** vêm do arquivo (DI + `CredentialProvider`), preservando a constitution §2/§4.

## Alternativas consideradas

- **Carregar direto para `harness.Config`**: impossível — `Config` exige portas/adapters (DI) que o arquivo não fornece. Rejeitada.
- **YAML**: exigiria dependência nova e um schema paralelo. Rejeitada no v1 (JSON já é o contrato).
- **Pacote `internal/config`**: o host precisa consumir; precisa ser público. Rejeitada.
- **Validar só com `json.Unmarshal`**: não checa enums/required/`additionalProperties` como o schema. Rejeitada (reusa a infra de schema).

## Consequências

- Configuração de modelos/política/pricing/contexto/redação/system prompt via arquivo validado; `max_media_bytes`/`prompt_caching` e overrides por tool (com `timeout_seconds`) mapeados.
- Reuso do schema canônico (uma verdade); `go generate` mantém o gerado.
- Testes: `adapters/config/file_test.go` (válido + inválido).
