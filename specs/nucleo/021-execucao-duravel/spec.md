# Feature Specification: Execução durável (checkpoint e retomada)

**Feature Branch**: `nucleo/021-execucao-duravel` · **Status**: Draft (aguardando portão)

**Input**: pedido do operador — Fase D (P2): "execução durável".

## Problema

Hoje o turno só é persistido **no fim** (`Run` → `Sessions.Save`) ou ao pausar por confirmação. Se o processo do host cair/gozar no meio de um turno com tools, o trabalho é perdido e — pior — **não há registro de qual efeito já aconteceu**. Retomar às cegas pode repetir uma tool não-idempotente (transferência, exclusão). Falta o contrato de *write-ahead* + retomada que o núcleo já promete na constitution §6 ("tool já executada nunca é repetida").

## Requisitos (EARS)

- **FR-DU-001**: WHEN `Config.Durable=true`, o harness MUST persistir a sessão (checkpoint) após cada passo durável: mensagem do assistente, write-ahead **antes** de executar cada tool e resultado de cada tool.
- **FR-DU-002**: The checkpoint MUST registrar `turn_id`, `status` (`running`/`awaiting_confirmation`/`completed`), `step` e, no write-ahead, `pending_call` (call_id, tool, args redigidos) — persistido no contrato de sessão.
- **FR-DU-003**: WHEN `Run` encontra uma sessão com checkpoint `running` do mesmo dono, o harness MUST retomar o turno a partir do histórico persistido, **sem** re-anexar a entrada.
- **FR-DU-004**: WHEN há `pending_call` sem resultado correspondente no histórico (interrupção entre write-ahead e execução), o harness MUST NOT reexecutar automaticamente tool **não-idempotente**; MUST devolver resultado de erro "resultado ambíguo após interrupção" ao modelo.
- **FR-DU-005**: WHEN a tool pendente é declarada idempotente (`Tool.Idempotent` ou política), o harness MAY reexecutá-la na retomada.
- **FR-DU-006**: Um turno retomado MUST NOT duplicar custo/usage já contabilizado: o `Usage` da sessão é carregado do checkpoint e continua a partir dele.
- **FR-DU-007**: `Config.Durable=false` (default) MUST preservar o comportamento anterior (save só no fim/pausa).
- **FR-DU-008**: A retomada MUST ser observável (`TurnResult.Resumed`, `CheckpointEvent` opcional) e MUST concluir normalmente quando não há interrupção.
- **FR-DU-009**: Checkpoint após conclusão MUST marcar `completed`; o próximo `Run` com o mesmo `SessionID` inicia turno novo (a menos que seja retomada explícita).

## Critérios (Gherkin)

```gherkin
Cenário: queda no meio do turno e retomada
  Dado Durable=true e um turno que executa a tool t1 e é interrompido antes de concluir
  Quando um novo harness carrega a mesma sessão e chama Run sem Input
  Então o turno continua do histórico, t1 não é reexecutada e o turno conclui

Cenário: interrupção entre write-ahead e execução
  Dado um pending_call persistido sem resultado no histórico
  E a tool não é idempotente
  Quando a retomada ocorre
  Então o modelo recebe resultado de erro "resultado ambíguo" e o turno segue

Cenário: Durable desligado
  Dado Durable=false
  Quando o turno interrompe antes do fim
  Então nenhum checkpoint intermediário é gravado (comportamento anterior)
```

## Casos de borda

- Checkpoint de sessão de outro usuário: rejeitado (`run/sessao-de-outro-usuario`).
- `SessionStore.Save` falha durante o checkpoint: o turno falha explícito (durabilidade não é best-effort quando ligada).
- Confirmação pendente já existia: continua usando o caminho `ResolveConfirmation` (não é retomada por `Run`).
- Checkpoint `running` de turno já encerrado por cancelamento: retomada é válida (o cancelamento salvou o ponto).

## Não-objetivos (v1)

- Fila/worker próprio de execução assíncrona (o host decide).
- Coordenação distribuída/lock entre instâncias — porta do host.
- Compensação (rollback) de efeitos: apenas evita repetição; a decisão é do modelo/host.

## Dependências e espelho

- Estende o contrato `contracts/session/session_snapshot.json` (`checkpoint`) — regenerar `contracts/gen`.
- Respeita a constitution §6 (tool executada nunca repetida) e dialoga com a reconciliação do financeiro (028: resultado ambíguo bloqueia retry).
