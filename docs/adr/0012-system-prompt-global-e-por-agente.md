# ADR 0012 — System prompt configurável (global + por agente)

**Status**: Accepted
**Data**: 2026-09-16
**Feature**: `specs/nucleo/005-system-prompt`

## Contexto

O contrato de provider do harness sempre teve `ChatRequest.System` e os três adapters o serializam (OpenAI/Anthropic/Responses), mas o engine nunca o preenchia: `Config` não expunha system prompt e `RunRequest` também não. O host não tinha um ponto de configuração de persona/instruções; só podia injetar `RoleSystem` via memória/RAG e janela. O operador pediu suporte "pronto pra configurar".

## Decisão

Adicionar `Config.SystemPrompt` (global) e `AgentPolicy.SystemPrompt` (override por agente). O engine resolve no turno — **agente vence global** quando não vazio — e envia em `ChatRequest.System`. O prompt fica **separado** das mensagens `RoleSystem` de memória/resumo (não concatena): o adapter emite um bloco de sistema e as mensagens seguem na ordem do contexto. Sem prompt, o campo continua vazio (comportamento anterior). O contrato de arquivo de config (`contracts/config/harness_config.json`) ganhou `system_prompt` no topo e em `agent_policy` (e, de passagem, as capacidades multimodais `vision`/`documents`/`max_media_bytes` da feature 002, mantendo o schema em sincronia).

## Alternativas consideradas

- **Só global**: simples, mas o host que serve múltiplos agentes precisaria reescrever o prompt por turno. Rejeitada.
- **`RunRequest.System` (por turno)**: flexível, porém o prompt é propriedade do agente/config, não do pedido; adiado. Rejeitada no v1.
- **Concatenar com memória/resumo num único bloco**: mistura proveniências e dificulta o controle. Rejeitada (campo dedicado).
- **Nada**: deixa o host sem ponto de configuração. Rejeitada.

## Consequências

- Muda só o núcleo (loop resolve e preenche) e a config; adapters e schemas já suportavam `system`.
- `Config.SystemPrompt`/`AgentPolicy.SystemPrompt` são aditivos; `system_prompt` opcional no schema (sem quebra).
- Testes: `harness/system_prompt_test.go` (global, override, herança, vazio).
