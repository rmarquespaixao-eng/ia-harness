# Feature Specification: MCP — resources e prompts

**Feature Branch**: `nucleo/017-mcp-resources-prompts` · **Status**: Aprovado e implementado

## Problema
O cliente MCP só consumia tools; servidores MCP ricos expõem resources e prompts (spec 2025-06-18/2026). Sampling foi **deprecado**; elicitation e OAuth ficam para depois.

## Requisitos (EARS)
- **FR-MCP-001**: O `mcpclient` MUST expor `ListResources`/`ReadResource` (conteúdo textual) e `ListPrompts`/`GetPrompt`.
- **FR-MCP-002**: As operações MUST reusar a sessão preguiçosa e a reconexão única do cliente.
- **FR-MCP-003**: Blobs de resource MUST NOT ser propagados (o host trata binário).
- **FR-MCP-004**: Falhas MUST vir embrulhadas com o nome do servidor, preservando a causa.

## Critérios (Gherkin)
```gherkin
Cenário: recurso e prompt
  Dado um servidor MCP com um resource e um prompt
  Quando o cliente lista e lê
  Então recebe o texto do recurso e as mensagens do prompt
```

## Não-objetivos (v1)
- Elicitation (`elicitation/create`) e OAuth — deferidos.
- Sampling — deprecado na spec.
- Resource templates/notificações de mudança — deferidos.
