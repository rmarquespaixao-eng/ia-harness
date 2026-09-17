# Use Cases: Execução durável (021)

Formato fully-dressed (Cockburn) + Gherkin. O operador implementa; o agente audita.

## CU-DU-1 — Queda no meio do turno e retomada

- **Identificador**: CU-DU-1 · **Escopo**: motor do harness · **Nível**: subfunção
- **Ator primário**: host (biblioteca embarcada)
- **Partes interessadas e interesses**: operador (não perder trabalho); usuário (não repetir efeito)
- **Pré-condições**: `Durable=true`; `SessionStore` persistente; turno com tools
- **Gatilho**: novo `Run` com o mesmo `SessionID` após interrupção
- **Garantia de sucesso**: o turno continua do ponto persistido e conclui
- **Garantia mínima**: nenhuma tool já executada é repetida
- **Mapeamento técnico**: `checkpoint.go` → `Session.Checkpoint` → retomada em `Run`
- **Fluxo principal de sucesso**:
  1. O turno grava o checkpoint `running` e o write-ahead de cada tool.
  2. O processo interrompe.
  3. Um novo harness carrega a sessão, vê `running` e retoma sem re-anexar input.
  4. Os passos já no histórico são reaproveitados; o turno conclui e o checkpoint vira `completed`.
- **Fluxos alternativos e de exceção**:
  - `3a` Checkpoint de outro usuário: erro nomeado.
  - `3b` `Save` do checkpoint falha: turno falha explícito.
- **Regras de negócio**: RN-1 write-ahead antes de efeito; RN-2 resultado é a verdade; RN-3 usage acumulado no snapshot.
- **Critérios de aceite (Gherkin)**:
```gherkin
Cenário: retomada não repete tool
  Dado um turno que executou t1 e foi interrompido
  Quando a retomada roda
  Então o contador de chamadas de t1 continua 1
```
- **Rastreabilidade**: FR-DU-001..003, 006.

## CU-DU-2 — Interrupção entre write-ahead e execução

- **Identificador**: CU-DU-2 · **Escopo**: motor · **Nível**: subfunção
- **Garantia de sucesso**: a retomada não repete efeito de tool não-idempotente
- **Fluxo principal**:
  1. O checkpoint registra `pending_call` e o processo cai antes do resultado.
  2. Na retomada, não há resultado para o `call_id`.
  3. Tool não-idempotente ⇒ resultado de erro "resultado ambíguo após interrupção".
  4. Tool idempotente ⇒ pode reexecutar.
- **Rastreabilidade**: FR-DU-004/005.

## CU-DU-3 — Durabilidade desligada

- **Identificador**: CU-DU-3 · **Escopo**: motor · **Nível**: subfunção
- **Garantia mínima**: `Durable=false` mantém o comportamento anterior
- **Fluxo principal**: sem flag, o save acontece só no fim/pausa; nenhum checkpoint intermediário é gravado.
- **Rastreabilidade**: FR-DU-007.

## Requisitos especiais (NFR)

- Determinismo: store inmem + clock fake; nenhum teste toca rede.
- Retrocompatibilidade: `checkpoint` é opcional no schema; snapshot antigo continua válido.
- Observabilidade: `CheckpointEvent` opcional + `TurnResult.Resumed`.
