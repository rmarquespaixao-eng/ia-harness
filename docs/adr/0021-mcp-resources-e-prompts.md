# ADR 0021 — MCP: resources e prompts (elicitation/OAuth deferidos)

**Status**: Accepted
**Data**: 2026-09-16
**Feature**: `specs/nucleo/017-mcp-resources-prompts`

## Contexto

O cliente MCP só consumia tools. A spec MCP (2026-07-28) tem resources e prompts como primitivas de servidor, além de elicitation como primitiva de cliente; sampling foi **deprecado**. O SDK oficial v1.8.0 expõe `ListResources`/`ReadResource`/`ListPrompts`/`GetPrompt` no `ClientSession`.

## Decisão

Adicionar ao `mcpclient` os métodos `ListResources`, `ReadResource` (conteúdo **textual**; blobs não são propagados), `ListPrompts` e `GetPrompt`, reusando a sessão preguiçosa e a reconexão única (`withReconnect`). Elicitation e OAuth **ficam fora do v1** (documentados): exigem fluxo de UI e troca de tokens que pertencem ao host; sampling é deprecado e não será adicionado.

## Alternativas consideradas

- **Implementar elicitation/OAuth agora**: superfície alta (UI + tokens) e sem caso no financeiro. Rejeitada no v1.
- **Só tools**: deixa servidores ricos inaproveitados. Rejeitada.

## Consequências

- Recursos/prompts disponíveis para o host; binário de resource fica com o host.
- Deferidos explícitos: elicitation, OAuth, resource templates, notificações de mudança.
- Testes: `adapters/mcpclient/resources_test.go` (servidor em memória).
