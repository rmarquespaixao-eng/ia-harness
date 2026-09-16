# Feature Specification: Loader do arquivo de configuração

**Feature Branch**: `nucleo/008-config-loader`

**Status**: Aprovado e implementado

**Input**: lacuna P0 — o schema `contracts/config/harness_config.json` existia, mas não havia código que carregasse um arquivo; o host montava tudo à mão.

## 1. Problema

"Pronto pra configurar" exige ler a configuração de um arquivo validado, não só montar structs em Go. O contrato do arquivo já existia (JSON Schema) e era gerado, mas nenhum pacote o consumia.

## 2. Requisitos (EARS)

- **FR-CFG-001**: O harness MUST oferecer um loader que leia um arquivo de configuração, valide contra o schema canônico e o interprete.
- **FR-CFG-002**: O loader MUST aplicar os campos ao `harness.Config` (models, default_model, system_prompt, policy com overrides, pricing, context, redaction) **sem** tocar em Providers/Tools/portas (DI).
- **FR-CFG-003**: O loader MUST expor os **servidores MCP** declarados (`name`, `endpoint`, `credential_ref`, `tool_timeout`) para o host montar o `mcpclient`.
- **FR-CFG-004**: Credenciais MUST NOT entrar no arquivo (só `credential_ref`); schema inválido MUST falhar com erro nomeado antes de aplicar.
- **FR-CFG-005**: O loader MUST ser testável sem rede, com fixture.

## 3. Critérios de aceite (Gherkin)

```gherkin
Cenário: arquivo válido vira Config
  Dado um arquivo com models, policy, pricing, context, system_prompt e mcp_servers
  Quando o loader interpreta e aplica
  Então o Config tem os modelos e a política com overrides (timeout_seconds → Duration)
  E os servidores MCP são expostos

Cenário: schema inválido é rejeitado
  Dado um arquivo sem "policy.default"
  Quando o loader interpreta
  Então falha com erro de schema
  E nada é aplicado
```

## 4. Não-objetivos

- YAML (JSON no v1) ou hot-reload/hot-watch.
- Montar Providers/Tools/portas (DI do host).
- Resolver credenciais (o `CredentialProvider` faz).
