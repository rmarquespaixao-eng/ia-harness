# Use Cases: Rate limiting de provider (019)

Formato fully-dressed (Cockburn) + Gherkin. O operador implementa; o agente audita.

## CU-RL-1 — Rajada acima do limite

- **Identificador**: CU-RL-1 · **Escopo**: motor do harness · **Nível**: subfunção
- **Ator primário**: host (biblioteca embarcada)
- **Partes interessadas e interesses**: operador (não ser bloqueado pelo provedor); provedor (vazão respeitada)
- **Pré-condições**: provider com `RequestsPerMinute`/`Burst` configurados; `Waiter` injetado
- **Gatilho**: chamada de modelo (`chat`) no turno
- **Garantia de sucesso**: as chamadas são espaçadas conforme o limite e o turno conclui
- **Garantia mínima**: nenhuma chamada excede o bucket; o host é notificado da espera
- **Mapeamento técnico**: `Bucket.Reserve` → `Waiter.Wait` → `Provider.Chat`
- **Fluxo principal de sucesso**:
  1. O motor resolve o perfil e o alias do provider.
  2. O bucket do alias devolve a espera necessária (`Reserve(now)`).
  3. Se a espera cabe em `MaxWait`, o motor aguarda (cancelável) e chama o provedor.
  4. O evento `RateLimitEvent` é emitido quando o handler o observa.
- **Fluxos alternativos e de exceção**:
  - `2a` Sem limite configurado: chamada segue sem espera.
  - `3a` Espera > `MaxWait`: erro retryável `ratelimit/espera-excedida` (não bloqueia).
  - `3b` Contexto cancelado na espera: devolve `context.Canceled`, provedor não é chamado.
- **Regras de negócio**: RN-1 bucket por alias de provider; RN-2 limite específico vence o global; RN-3 refill pelo `Clock`.
- **Critérios de aceite (Gherkin)**:
```gherkin
Cenário: espaçamento
  Dado RequestsPerMinute=60 e Burst=1
  Quando duas chamadas ocorrem em sequência imediata
  Então a segunda espera ~1s (relógio fake) antes do I/O
```
- **Rastreabilidade**: FR-RL-001..003, 005.

## CU-RL-2 — Falha rápida quando a espera é longa

- **Identificador**: CU-RL-2 · **Escopo**: motor · **Nível**: subfunção
- **Ator primário**: host
- **Pré-condições**: `MaxWait` definido
- **Garantia de sucesso**: o turno não fica preso; o erro é classificado retryável
- **Critérios (Gherkin)**:
```gherkin
Cenário: teto de espera
  Dado RequestsPerMinute=1 e MaxWait=100ms
  Quando a espera necessária é 1s
  Então a chamada falha com ratelimit/espera-excedida retryável
```
- **Rastreabilidade**: FR-RL-004.

## CU-RL-3 — Cancelamento durante a espera

- **Identificador**: CU-RL-3 · **Escopo**: motor · **Nível**: subfunção
- **Ator primário**: host
- **Garantia mínima**: cancelar não gera chamada ao provedor nem efeito
- **Fluxo principal**: a espera observa `ctx.Done()` e retorna o erro de contexto.
- **Rastreabilidade**: FR-RL-008.

## Requisitos especiais (NFR)

- Determinismo: nenhum teste dorme de verdade; `Waiter` e `Clock` são fakes.
- Sem breaking change: `Handler`/`Clock`/`Config` permanecem compatíveis (portas opcionais).
