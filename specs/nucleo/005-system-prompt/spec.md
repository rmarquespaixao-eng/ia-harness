# Feature Specification: System prompt configurável (global e por agente)

**Feature Branch**: `nucleo/005-system-prompt`

**Created**: 2026-09-16

**Status**: Aprovado e implementado (portão liberado pelo operador)

**Input**: "suporta system pronto pra configurar?" — o contrato do provider (`ChatRequest.System`) e os três adapters já honram um prompt de sistema, mas o engine nunca o preenchia e a `Config` não tinha campo. Decisão: campo dedicado, global + override por agente, separado das mensagens de sistema de memória/resumo.

## 1. Problema

Hoje o host não consegue configurar persona/instruções do agente: `Config` não tem campo de system prompt e `RunRequest` também não; o `ChatRequest.System` fica sempre vazio (só há mensagens `RoleSystem` injetadas por memória/RAG e janela). Isso obriga o host a moldar o comportamento só por mensagens, sem um ponto de configuração claro.

## 2. Requisitos (EARS)

- **FR-SP-001**: WHEN `AgentPolicy.SystemPrompt` do agente da sessão não está vazio, o harness SHALL usá-lo como system prompt do turno; senão SHALL usar `Config.SystemPrompt`.
- **FR-SP-002**: O harness MUST enviar o system prompt resolvido em `ChatRequest.System`, separado das mensagens `RoleSystem` de memória/resumo (sem concatenar).
- **FR-SP-003**: Sem prompt configurado, `ChatRequest.System` MUST ficar vazio (comportamento anterior preservado).
- **FR-SP-004**: O prompt de sistema MUST NOT conter segredo; entra por configuração do host, como os demais campos (constitution §4).
- **FR-SP-005**: O contrato do arquivo de configuração (`contracts/config/harness_config.json`) MUST expor `system_prompt` no topo e em `agent_policy`, além das capacidades multimodais já implementadas (`vision`/`documents`/`max_media_bytes`).

### Decisões (D-SP)

- **D-SP-1**: precedência `agente > global`; sem `RunRequest.System` no v1.
- **D-SP-2**: campo dedicado (`ChatRequest.System`), separado das mensagens de sistema injetadas.

## 3. Critérios de aceite (Gherkin)

```gherkin
Cenário: prompt global chega ao provider
  Dado Config.SystemPrompt = "Você é o assistente financeiro."
  Quando o turno roda
  Então ChatRequest.System é o prompt global

Cenário: override do agente vence o global
  Dado Config.SystemPrompt = "global" e AgentPolicy.SystemPrompt = "do agente"
  Quando o turno do agente roda
  Então ChatRequest.System = "do agente"

Cenário: sem configuração
  Dado nenhum prompt configurado
  Quando o turno roda
  Então ChatRequest.System é vazio
```

## 4. Não-objetivos

- Template/variáveis no prompt, versionamento de prompt, biblioteca de prompts (evolução).
- Aplicar prompt por mensagem (o system é do turno).
