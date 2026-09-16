# Feature Specification: Núcleo de harness de IA reutilizável (agente + tools MCP + multi-provedor)

**Feature Branch**: `nucleo/001-harness-ia-reutilizavel`

**Created**: 2026-09-15

**Status**: Draft

**Input**: User description: "Criar um harness de IA reaproveitável para integrar em sistemas — no caso, o financeiro — com suporte a MCP Tools e multi-provider/modelos. Especificar a melhor arquitetura para ser reutilizável."

## 1. Problema

Cada sistema que hoje quer usar IA (primeiro caso: o financeiro) precisa montar do zero o mesmo encanamento: falar com provedores de modelo, descobrir e executar tools de um servidor MCP, manter sessão/histórico, aplicar permissões em operações destrutivas, auditar chamadas e estimar custo. Isso duplica esforço, diverge comportamento e espalha risco (segredo mal redigido, tool destrutiva sem confirmação, teste que depende de rede).

O financeiro já expõe seu domínio como **servidor MCP** (96 tools, SSE, progresso em tempo real, `confirm` para operações destrutivas). Falta o **lado agente**: um núcleo reutilizável que qualquer sistema hospedeiro embuta para conversar com o usuário, decidir quais tools chamar e executá-las com política, auditoria e custo.

Esta feature especifica o **núcleo do harness**: o motor reutilizável. O primeiro consumidor é o `financeiro-api-v2`; outros sistemas vêm depois sem reescrever o núcleo.

## 2. User Scenarios & Testing *(mandatory)*

### User Story 1 - Conversar com os dados do sistema via tools MCP (Priority: P1)

O usuário final de um sistema hospedeiro (ex.: o dono das finanças) escreve em linguagem natural ("qual meu saldo deste mês?", "lança essa compra no cartão"). O harness descobre as tools do servidor MCP configurado, deixa o modelo escolher as chamadas necessárias, executa-as com validação e devolve a resposta fundamentada no resultado das tools — em streaming, com progresso visível nas operações longas.

**Why this priority**: é a razão de existir do harness; sem o loop de agente + tools não há produto. É o MVP.

**Independent Test**: com um servidor MCP de teste (fake) e um provedor simulado, pedir uma informação que exige tool e verificar que o harness descobriu, chamou e respondeu com o dado; pedir uma operação longa e verificar eventos de progresso antes da resposta final.

**Acceptance Scenarios**:

1. **Given** um servidor MCP acessível com tools publicadas e um provedor/modelo configurados, **When** o usuário faz uma pergunta que exige consultar dados, **Then** o harness lista as tools disponíveis, seleciona a correta via modelo, valida os argumentos contra o schema publicado e devolve resposta em linguagem natural baseada no resultado.
2. **Given** que o modelo pediu uma tool com argumentos inválidos, **When** a validação falha, **Then** o erro é devolvido ao modelo como resultado de tool (sem executar nada) e o turno continua até uma resposta válida ou limite atingido.
3. **Given** uma tool longa que emite notificações de progresso, **When** ela é executada, **Then** o consumidor recebe os eventos de progresso antes do resultado final.
4. **Given** o streaming habilitado, **When** o modelo gera texto, **Then** os fragmentos chegam incrementalmente e o turno pode ser cancelado sem corromper o estado da sessão.
5. **Given** que o servidor MCP ficou indisponível no meio de uma conversa, **When** o modelo tenta uma tool, **Then** o harness reporta a falha como resultado de tool, não inventa resposta e mantém a sessão consistente.

---

### User Story 2 - Trocar provedor/modelo sem tocar no sistema hospedeiro (Priority: P2)

O operador configura mais de um provedor/modelo (perfis com credenciais e limites) e escolhe qual cada sistema/agente usa. Quando o provedor primário falha, está limitado (rate limit) ou indisponível, o harness tenta o fallback configurado e registra o motivo.

**Why this priority**: evita lock-in e entrega resiliência operacional com custo baixo de implementação sobre o loop do P1.

**Independent Test**: com dois provedores simulados, trocar a configuração de alias e verificar que a próxima sessão usa o novo modelo sem alteração de código; derrubar o primário e verificar fallback com registro do motivo.

**Acceptance Scenarios**:

1. **Given** dois perfis de provedor configurados, **When** o alias de modelo é trocado na configuração, **Then** a próxima sessão usa o novo modelo e a troca é auditada.
2. **Given** falha de indisponibilidade/limite no provedor primário, **When** o harness decide a chamada, **Then** ele tenta o fallback configurado, registra qual modelo respondeu e não repete tools já executadas sem idempotência garantida.
3. **Given** provedor configurado sem credencial válida, **When** a sessão inicia, **Then** o harness falha de forma explícita e acionável, sem degradar silenciosamente.

---

### User Story 3 - Operações destrutivas exigem política e confirmação (Priority: P3)

O operador define, por usuário/agente, quais tools são permitidas, quais exigem confirmação humana e se o modo é somente-leitura. O harness nunca executa fora da política e devolve a negação ao modelo como recusa — auditada.

**Why this priority**: o domínio-alvo inclui exclusões e transferências irreversíveis; permissão é requisito de segurança, não refinamento.

**Independent Test**: com política que exige confirmação para uma tool destrutiva, verificar que a execução pausa, que aprovar executa, que negar devolve recusa ao modelo e que ambos os caminhos ficam auditados; com modo somente-leitura, verificar que qualquer escrita é negada.

**Acceptance Scenarios**:

1. **Given** uma tool fora da allowlist, **When** o modelo a solicita, **Then** o harness nega a execução, informa o modelo (recusa) e registra a decisão — sem chamar o servidor MCP.
2. **Given** uma tool marcada como destrutiva, **When** o modelo a solicita, **Then** a sessão pausa em "aguardando confirmação" e o host recebe o pedido de decisão.
3. **Given** uma confirmação pendente, **When** o host aprova, **Then** a tool é executada e o turno retoma; **When** o host nega, **Then** a recusa é devolvida ao modelo e o turno segue sem executar.
4. **Given** configuração de somente-leitura, **When** qualquer tool de escrita é solicitada, **Then** ela é negada com o motivo.
5. **Given** conteúdo malicioso vindo de resultado de tool (instrução embutida), **When** o modelo tenta usá-lo para solicitar uma tool fora da política, **Then** a política continua sendo aplicada fora do modelo e a execução é negada.

---

### User Story 4 - Sessões persistidas e retomáveis (Priority: P4)

A conversa de cada usuário é persistida pelo host através do contrato do harness. Se o processo reiniciar, a sessão retoma com histórico e contexto; quando o contexto cresce além da janela do modelo, uma política explícita (truncar com transparência ou sumarizar) mantém a conversa dentro do limite.

**Why this priority**: sem persistência a experiência quebra a cada restart; a política de janela evita falha silenciosa em conversas longas.

**Independent Test**: iniciar conversa com N turnos, recriar o núcleo (simulando restart), retomar a mesma sessão e verificar histórico preservado; forçar estouro de janela e verificar a política aplicada e registrada.

**Acceptance Scenarios**:

1. **Given** uma sessão ativa com histórico, **When** o hospedeiro reinicia e retoma a sessão, **Then** o histórico e o contexto anterior estão disponíveis sem reenvio manual.
2. **Given** histórico maior que a janela do modelo, **When** um novo turno é processado, **Then** a política configurada é aplicada (truncamento com aviso ou sumarização) e o fato fica registrado.
3. **Given** sessões de usuários diferentes, **When** qualquer operação ocorre, **Then** dados de uma sessão nunca vazam para outra (isolamento por usuário).

---

### User Story 5 - Trilha de auditoria e custo por chamada (Priority: P5)

Cada chamada de modelo e de tool gera um evento estruturado (provedor, modelo, tokens, latência, status, tool, custo estimado), redigido por allowlist antes de sair do núcleo. O host decide onde persistir. Quando o provedor não reporta tokens, o harness estima por volume × premissa de preço configurável — e marca o evento como "estimado".

**Why this priority**: sem trilha não há operação responsável nem controle de gasto; depende dos eventos do loop do P1.

**Independent Test**: executar turnos com tool (sucesso, erro e negação) e verificar os eventos emitidos, com segredo/PII ausentes e estimativa marcada quando não houver uso reportado.

**Acceptance Scenarios**:

1. **Given** um turno com chamada de modelo e de tool, **When** o turno termina, **Then** eventos de auditoria são emitidos com provedor, modelo, tokens (ou marcação de estimativa), latência e status.
2. **Given** argumentos/resultados contendo credencial ou dado sensível, **When** o evento é emitido, **Then** o valor é redigido/truncado por allowlist.
3. **Given** provedor que não reporta uso, **When** o evento é emitido, **Then** o custo aparece como estimado (premissa configurável), com rótulo distinguindo do reportado.

---

### User Story 6 - Memória de longo prazo e recuperação semântica (Priority: P6)

O harness mantém fatos e preferências duráveis do usuário e permite recuperar conteúdo indexado fornecido pelo host (ex.: transações, documentos) por similaridade, injetando no contexto com proveniência — sempre escopado por usuário.

**Why this priority**: melhora a personalização e o acerto das respostas, sem bloquear o valor central das histórias anteriores.

**Independent Test**: gravar um fato do usuário, iniciar nova sessão e verificar que o fato influencia a resposta; indexar um item no host, perguntar sobre ele e verificar a recuperação com identificação da fonte.

**Acceptance Scenarios**:

1. **Given** um fato/preferência gravado para o usuário, **When** uma nova sessão é criada, **Then** o fato pode ser injetado no contexto com sua proveniência.
2. **Given** conteúdo indexado pelo host (via contrato do harness), **When** a pergunta do usuário é semanticamente relacionada, **Then** os itens relevantes são recuperados e citados como fonte.
3. **Given** memórias de usuários distintos, **When** há recuperação, **Then** somente memórias do usuário da sessão são consideradas.
4. **Given** que o usuário pede para esquecer um fato, **When** a operação é executada, **Then** o fato deixa de ser recuperado em sessões futuras.

---

### Edge Cases

- **Servidor MCP indisponível na descoberta**: sessão inicia sem tools? Falha explícita? (Decisão: degradar com aviso claro e operar sem tools é aceitável; executar tool sem catálogo, não.)
- **Sessão MCP expira no meio da conversa**: o harness deve reconectar de forma transparente antes de reportar falha.
- **Duas tool calls no mesmo turno com dependência de ordem**: execução deve respeitar a ordem/paralelismo seguros; tool destrutiva nunca executa em paralelo com outra do mesmo turno.
- **Cancelamento durante tool em execução**: o efeito no servidor não é revertido; o resultado deve ser reportado e a sessão permanecer consistente.
- **Queda do stream de resposta**: retomar ou encerrar o turno de forma limpa, sem duplicar a cobrança/evento.
- **Modelo retorna texto achando que chamou tool** (tool call alucinada): executar apenas chamadas estruturadas no protocolo; ignorar tool inexistente com erro devolvido.
- **Estouro de orçamento do turno** (iterações, tokens ou tempo): encerrar com mensagem clara e evento de auditoria.
- **Prompt injection vindo de dado do usuário ou de resultado de tool**: política e permissões são aplicadas fora do modelo; conteúdo não eleva privilégio.
- **Provedor sem suporte a tool-calling**: rotear/usar perfil compatível ou falhar explícito, nunca fingir suporte.
- **Resposta duplicada em retry de provedor**: chamadas de modelo são idempotentes por natureza; tools só repetem se idempotentes.
- **Falha ao persistir sessão/memória pelo host**: o turno deve falhar de forma observável antes de perder dados silenciosamente.

## 3. Requirements *(mandatory)*

### Functional Requirements

**Núcleo e integração**

- **FR-001**: O harness MUST operar embutido no processo do sistema hospedeiro, exposto como biblioteca — nesta fase não há serviço separado nem API de rede própria.
- **FR-002**: O harness MUST ser configurável por um único ponto de composição do host (perfis de provedor, servidores MCP, política de permissão, portas de persistência), sem exigir alteração do núcleo para novos hosts.
- **FR-003**: O harness MUST NOT depender de banco, fila ou serviço específico do financeiro; sessões, auditoria e memória são acessadas através de portas/contratos fornecidos pelo host.
- **FR-004**: O host MUST poder injetar credenciais de provedores e tokens de servidores MCP em runtime; nenhum segredo é embutido, logado ou persistido pelo núcleo.

**Agente e tools MCP**

- **FR-005**: O harness MUST ser cliente de um ou mais servidores MCP, descobrindo tools dinamicamente e mantendo o catálogo atualizado por sessão.
- **FR-006**: O harness MUST validar argumentos de tool contra o schema publicado pelo servidor antes de executar; argumento inválido vira resultado de erro entregue ao modelo, nunca execução.
- **FR-007**: WHEN o modelo solicita tool inexistente ou com nome desconhecido, o harness SHALL devolver erro ao modelo como resultado de tool, sem executar nada.
- **FR-008**: O loop de agente MUST ter limites configuráveis (iterações, tokens, tempo) e encerrar com mensagem explícita ao atingi-los.
- **FR-009**: O harness MUST emitir streaming incremental de texto e de eventos de progresso de tools longas, no mesmo canal de eventos consumido pelo host.
- **FR-010**: O host MUST poder cancelar um turno em andamento; o harness encerra eventos pendentes e mantém o estado da sessão consistente.
- **FR-011**: IF um servidor MCP ficar indisponível THEN o harness SHALL reportar falha ao modelo como resultado de tool, manter a sessão utilizável e tentar reconectar em chamadas futuras.

**Multi-provedor e modelos**

- **FR-012**: O harness MUST suportar múltiplos provedores e modelos por configuração do host (adaptador de provedor injetado + perfil de modelo com capacidades), selecionados por alias, sem alteração de código do host para trocar de modelo.
- **FR-013**: WHEN o provedor primário falha por indisponibilidade, limite ou erro transitório, o harness SHALL tentar o fallback configurado e registrar o motivo e o modelo efetivamente usado.
- **FR-014**: O harness MUST NOT repetir tool já executada em retry/fallback, exceto quando a tool for declaradamente idempotente.
- **FR-015**: IF o perfil de provedor não suportar as capacidades exigidas pelo turno (ex.: tool-calling) THEN o harness SHALL falhar de forma explícita, sem simular suporte.

**Permissões e confirmação**

- **FR-016**: O harness MUST aplicar política de permissão por usuário/agente: allowlist de tools, modo somente-leitura e marcação de tools que exigem confirmação humana.
- **FR-017**: WHEN o modelo solicita tool negada pela política, o harness SHALL recusar a execução sem chamar o servidor MCP, devolver a recusa ao modelo e registrar a decisão.
- **FR-018**: WHEN uma tool marcada exige confirmação, o harness SHALL pausar a sessão em estado "aguardando decisão", expor o pedido ao host e retomar o turno após aprovação; na negação, a recusa é devolvida ao modelo.
- **FR-019**: O harness MUST tratar conteúdo de tools e do usuário como dados, não instruções: nenhuma decisão de política pode ser alterada por conteúdo de conversa ou de resultado de tool.

**Sessões e contexto**

- **FR-020**: O harness MUST manter sessões conversacionais persistidas via porta fornecida pelo host, com histórico retomável após reinício do processo.
- **FR-021**: WHEN o contexto excede a janela do modelo, o harness SHALL aplicar política explícita (truncamento com transparência ou sumarização) e registrar a decisão no evento do turno.
- **FR-022**: O harness MUST isolar sessões, memórias e permissões por usuário do host; nenhuma leitura cruza usuários.

**Auditoria, custo e observabilidade**

- **FR-023**: O harness MUST emitir evento estruturado por chamada de modelo e por execução/negativa de tool, com provedor, modelo, tool, latência, status e tokens (reportados ou estimados), redigido por allowlist.
- **FR-024**: O harness MUST redigir/truncar segredos e dados sensíveis de mensagens, argumentos e resultados antes de log, auditoria ou telemetria; o evento marcado como negado não carrega payload.
- **FR-025**: WHEN o provedor não reporta uso, o harness SHALL estimar custo por volume × premissa(s) de preço configurável(is), rotulando explicitamente como estimativa.
- **FR-026**: O harness MUST propagar identificadores de correlação fornecidos pelo host em todos os eventos, logs e chamadas externas.

**Memória e recuperação**

- **FR-027**: O harness MUST permitir gravar, listar, esquecer e recuperar fatos/preferências duráveis do usuário, sempre escopados por usuário, via portas do host.
- **FR-028**: O harness MUST recuperar conteúdo semanticamente relevante fornecido pelo host (índice de documentos/transações) e injetá-lo no contexto com identificação de proveniência.
- **FR-029**: O harness MUST NOT indexar ou persistir conteúdo sem que o host forneça a porta correspondente; o núcleo não cria armazenamento próprio.

**Testabilidade e extensibilidade**

- **FR-030**: O harness MUST ser testável de ponta a ponta sem rede real, com provedor e servidor MCP substituíveis por simulações determinísticas.
- **FR-031**: O harness MUST permitir adicionar um novo provedor ou servidor MCP como adaptador, sem alterar as regras do núcleo do agente (loop, política, auditoria).

### Key Entities

- **Sessão**: conversa de um usuário do host; provedor/modelo, histórico, estado (ativa, aguardando confirmação, encerrada), orçamento consumido.
- **Turno/Mensagem**: unidade de interação; papéis, conteúdo, chamadas de tool solicitadas e resultados.
- **Servidor MCP**: origem das tools; identificação, endpoint/transporte, estado de conexão.
- **Tool**: operação publicada por um servidor MCP; nome, schema de entrada, marcação de destrutiva/idempotente.
- **Política de permissão**: regras por usuário/agente; allowlist, modo somente-leitura, exigência de confirmação.
- **Decisão de confirmação**: pedido pendente de aprovação/negação com contexto da tool e argumentos.
- **Evento de auditoria**: registro de uma chamada (modelo ou tool); tipo, provedor/modelo, tokens, latência, status, custo reportado/estimado, correlação.
- **Perfil de provedor**: credencial, capacidades, modelos disponíveis, limites e fallback.
- **Fato de memória**: informação durável do usuário; conteúdo, escopo, proveniência, data.
- **Item indexado**: conteúdo fornecido pelo host para recuperação semântica; identificador, texto, metadados.

### Decisões desta spec (D-HAR)

- **D-HAR-1**: consumo como **biblioteca embutida no processo do host** (financeiro-api-v2 primeiro); serviço separado e gateway MCP ficam fora do v1.
- **D-HAR-2**: papel **cliente MCP** — o harness consome servidores MCP (o financeiro já é servidor); não publica tools próprias via MCP no v1.
- **D-HAR-3**: o núcleo v1 inclui as seis capacidades do escopo aprovado: loop de agente com tool-calling/streaming, multi-provedor com fallback, permissões/confirmação, sessões persistidas, auditoria/custo e memória/RAG.
- **D-HAR-4**: stack do núcleo: **Go** — alinhada ao financeiro-api-v2 e ao SDK MCP já usado; detalhes de arquitetura ficam no `plan.md`.
- **D-HAR-5**: persistência é do host via portas (sessões, auditoria, memória/índice); o núcleo não impõe armazenamento.

## 4. Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Um sistema hospedeiro entrega a primeira capacidade de agente conversacional integrando o núcleo **sem alterar o núcleo** e sem serviço intermediário (primeiro caso: financeiro-api-v2).
- **SC-002**: Trocar o provedor/modelo de um host é **somente configuração**: nenhuma linha de código do host muda; a troca fica auditada.
- **SC-003**: **100%** das execuções e negativas de tool passam pela política de permissão e geram evento de auditoria verificável em teste.
- **SC-004**: Uma conversa retomada após reiniciar o processo do host mantém histórico e contexto (verificado em teste de retomada).
- **SC-005**: **Zero** credenciais ou dados sensíveis de argumentos/resultados aparecem em logs/trilha (verificado por teste de redação com valores-isca).
- **SC-006**: Com provedor remoto típico, o usuário vê o início da resposta **sem esperar o turno completo** (primeiros fragmentos chegam enquanto o modelo ainda gera).
- **SC-007**: Com fallback configurado e injeção de falha no primário, **≥ 90%** dos turnos sem tool destrutiva concluem sem erro visível ao usuário, registrando o modelo usado.
- **SC-008**: Um provedor novo ou servidor MCP novo entra em operação **sem alteração das regras do núcleo** — apenas adaptador + configuração.
- **SC-009**: A suíte de testes do núcleo roda **sem chamadas de rede reais** (100% simulado) e cobre os fluxos principais de todas as histórias P1–P6.

## 5. Não-objetivos (v1)

- Serviço independente/API de rede própria do harness (D-HAR-1).
- Gateway ou servidor MCP próprio; agregação de servidores MCP publicada a clientes externos (D-HAR-2).
- Interface de chat/UI do agente (responsabilidade do host).
- Autenticação/autorização dos usuários finais (já resolvida pelo host antes de chamar o harness).
- Fine-tuning, treino ou hospedagem de modelos.
- Agentes autônomos sem supervisão humana; execução de planos multi-passo sem confirmação humana quando a política exigir.
- Armazenamento próprio (banco, vetorial, fila): vem do host por portas.
- Cobrança/faturamento do uso do harness por terceiros.

## 6. Riscos

- **R1 — Duplicação de efeito em fallback**: repetir tool não idempotente após falha de provedor. Mitigação: FR-014 + marcação de idempotência e auditoria de decisão.
- **R2 — Injeção de prompt via resultado de tool/usuário**: conteúdo tentar alterar política. Mitigação: FR-019 (decisão de política fora do modelo) + allowlist estrita.
- **R3 — Vazamento de segredo em trilha/telemetria**: Mitigação: FR-024 (redação por allowlist; `denied` sem payload) + teste com valores-isca (SC-005).
- **R4 — Custo imprevisível**: uso sem limite estourar orçamento. Mitigação: FR-008 (orçamento por turno) + FR-025 (estimativa rotulada) + evento por chamada (FR-023).
- **R5 — Deriva de API dos provedores**: mudanças que quebram adaptadores. Mitigação: adaptadores isolados (FR-031) + testes de contrato com simulações.
- **R6 — Estado MCP expira/muda no meio da conversa**: falha de tool em cascata. Mitigação: FR-011 reconexão + catálogo por sessão (FR-005).
- **R7 — Testes frágeis/não determinísticos**: dependência de rede/modelo real. Mitigação: FR-030 (simulações determinísticas; SC-009).
- **R8 — Janela de contexto estourar em conversas longas**: falha silenciosa. Mitigação: FR-021 (política explícita e auditada).

## 7. Lacunas (resolvidas no plan/research — R1–R14)

- **L1**: lista inicial de provedores/modelos suportados e critério de entrada de novos (decidir no `research.md`).
- **L2**: política default de fallback × tools destrutivas (fallback pode ocorrer após confirmação humana? decidir no `plan.md` com ADR se necessário).
- **L3**: forma da recuperação semântica (quem gera embeddings — host ou adaptador do harness; decidir no `plan.md`).
- **L4**: política default de sumarização/truncamento de contexto (decidir no `plan.md`).
- **L5**: contrato de eventos para o host (streaming/assinatura) e formato mínimo de auditoria (decidir no `plan.md`/`contracts`).
- **L6**: telemetria (formato de logs e correlação) alinhada às diretrizes do operador (decidir no `plan.md`).

## 8. Assumptions

- O primeiro host é o `financeiro-api-v2`; a integração é in-process (D-HAR-1), aproveitando que o financeiro já é servidor MCP com SSE, progresso e `confirm`.
- A identidade do usuário final chega autenticada pelo host; o harness nunca autentica o usuário.
- Provedores reportam (ou não) tokens conforme sua API; a ausência é tratada como estimativa rotulada.
- O host fornece as portas de persistência (sessão, auditoria, memória/índice) e a injeção de credenciais; o núcleo não abre banco.
- Confirmação humana de operações destrutivas segue o padrão já existente no domínio (`confirm: true`).
- O transporte até a UI do host é responsabilidade do host; o núcleo emite eventos e fragmentos.
- Go é a stack do núcleo (D-HAR-4); testes e convenções seguem a constitution do repo (ratificada v1.1.0 em 2026-09-15).
- O uso inicial é single-tenant por instância (um dono por host), mas o isolamento por usuário é exigido desde já (FR-022).
