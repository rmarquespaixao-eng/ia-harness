# Feature Specification: MCP stdio (servidor MCP como processo local)

**Feature Branch**: `nucleo/023-mcp-stdio` · **Status**: Draft (aguardando portão)

**Input**: dependência declarada pelo host `harness-cli` (spec 002, FR-723/FR-727; ADR 0006 do
`harness-cli`): "MCP stdio é implementado no `ia-harness` e disponibilizado por release".

## Problema

O `adapters/mcpclient` só fala Streamable HTTP (ou um `mcp.Transport` injetado, usado em testes).
Boa parte dos servidores MCP do ecossistema é distribuída como **processo local** que conversa por
stdin/stdout (ex.: servidores de filesystem, git, bancos locais). Hoje cada host precisaria montar o
subprocesso por conta própria e injetar o transporte. Isso contraria o papel do harness (cliente MCP
comum, testado uma vez) e espalha decisões de segurança de subprocesso por vários hosts: argv,
ambiente, credenciais e processos órfãos.

O SDK oficial já em uso (`modelcontextprotocol/go-sdk` v1.8.0) oferece `mcp.CommandTransport`
(JSON delimitado por linha sobre stdin/stdout, com encerramento stdin→SIGTERM→SIGKILL). Falta ao
harness uma **configuração declarativa e segura** para usá-lo, com o mesmo contrato de `ToolSource`,
catálogo, progresso e reconexão do HTTP.

## Requisitos (EARS)

- **FR-STD-001**: The `mcpclient` SHALL aceitar, em `Config`, um servidor stdio descrito por
  `Command` (executável) e `Args` (lista explícita), mutuamente exclusivo com `Endpoint`.
- **FR-STD-002**: The harness MUST iniciar o processo **sem shell**: `Command` é executado
  diretamente com `Args` como argv; metacaracteres de shell não têm significado.
- **FR-STD-003**: WHEN `Command` não é caminho absoluto, the harness MUST resolvê-lo uma única vez
  por `PATH` no momento da conexão e MUST falhar com erro nomeado se não encontrar. Caminho relativo
  com separador (`./x`, `bin/x`) MUST ser recusado.
- **FR-STD-004**: The processo filho MUST NOT herdar o ambiente do host. Ele recebe apenas uma base
  mínima (`PATH`, `HOME`, `LANG`, `TMPDIR` e, no Windows, `SystemRoot`/`ComSpec`/`PATHEXT` quando
  presentes) mais as variáveis declaradas em `Config.Env`.
- **FR-STD-005**: Credenciais MUST entrar no filho somente por `Config.EnvCredentials`
  (nome da variável → `CredentialRef`), resolvidas por `Deps.Credentials` a cada início de processo.
  Credencial MUST NOT aparecer em `Args`, em `Config.Env`, em log, erro ou auditoria.
- **FR-STD-006**: A conexão MUST continuar preguiçosa (processo iniciado na primeira operação
  `List`/`Call`), e o tempo de vida do processo MUST ser ligado ao `Client`, não ao `context` da
  operação que o iniciou. Cancelar uma chamada não mata o servidor.
- **FR-STD-007**: `Client.Close` MUST encerrar o processo e seus descendentes (grupo de processos em
  Unix; job/árvore equivalente quando disponível), seguindo stdin fechado → SIGTERM → SIGKILL com
  `TerminateTimeout` configurável (default 5 s). Nenhum processo órfão após `Close`.
- **FR-STD-008**: WHEN o processo morre ou o pipe fecha, the harness MUST tratar como falha de
  sessão e aplicar a reconexão única existente (reinicia o processo uma vez por operação).
  Falha na inicialização (executável ausente, saída imediata, handshake inválido) MUST NOT entrar em
  laço de reinício.
- **FR-STD-009**: O stderr do filho MUST ser capturado, truncado (limite por linha e total por
  sessão) e encaminhado ao `Deps.Logger` em nível `debug`, com redação das credenciais resolvidas.
  O stderr MUST NOT ir para o stdout/stderr do host.
- **FR-STD-010**: `Config.Dir` opcional define o diretório de trabalho do filho. Vazio usa o
  diretório corrente do host.
- **FR-STD-011**: Tools, catálogo, progresso, resources/prompts (017) e política (default deny,
  confirmação) MUST se comportar como no transporte HTTP. O transporte é invisível para o motor.
- **FR-STD-012**: `Config` sem `Command` MUST preservar o comportamento atual byte a byte.
  `Endpoint` e `Command` juntos MUST ser erro de configuração.
- **FR-STD-013**: Eventos/erros MUST identificar o servidor por `Config.Name` e o executável por
  nome base, sem ecoar `Args` completos (podem carregar caminhos sensíveis) fora do nível `debug`.

## Critérios (Gherkin)

```gherkin
Cenário: servidor stdio publica tools
  Dado um servidor MCP de teste executado como processo local
  Quando o host lista as tools do Client configurado com Command/Args
  Então as tools aparecem no catálogo com o namespace de Config.Name
  E uma chamada devolve o resultado do processo

Cenário: credencial só por ambiente resolvido
  Dado EnvCredentials {"API_TOKEN": "vault:x"} e um CredentialProvider fake
  Quando o processo inicia
  Então o filho enxerga API_TOKEN com o valor resolvido
  E o valor não aparece em argv, em log nem em erro

Cenário: ambiente do host não vaza
  Dado o host com a variável SEGREDO_HOST definida
  Quando o processo inicia
  Então o filho não enxerga SEGREDO_HOST

Cenário: processo morre e reconecta uma vez
  Dado um servidor stdio que encerra após a primeira chamada
  Quando a segunda chamada é feita
  Então o harness reinicia o processo uma única vez e a chamada conclui

Cenário: Close não deixa órfão
  Dado um servidor stdio que cria um processo neto
  Quando Client.Close é chamado
  Então nenhum dos dois processos segue vivo após TerminateTimeout

Cenário: executável ausente
  Dado Command "nao-existe"
  Quando o host lista as tools
  Então recebe erro nomeado de executável não encontrado, sem reinício em laço
```

## Casos de borda

- Handshake que nunca responde: limitado pelo `context` da operação; o processo iniciado é encerrado
  (FR-STD-007) antes de devolver o erro.
- Servidor que escreve lixo no stdout: erro de protocolo, sessão invalidada, sem panic.
- Muitas linhas de stderr: truncamento garante memória limitada (FR-STD-009).
- `Command` com espaços (`"npx -y pkg"`): tratado como nome de executável literal e recusado por não
  existir; a mensagem orienta a usar `Args`.
- Chamadas concorrentes na mesma sessão: mesmo comportamento do HTTP (uma sessão por `Client`).
- Windows: sem grupo de processos POSIX; o kill da árvore é best-effort, documentado.

## Não-objetivos (v1)

- Sandbox do processo (namespaces, seccomp, limites de CPU/memória): responsabilidade do host/SO.
- Allowlist de executáveis dentro do harness: é política do host (o `harness-cli` declara em
  `docs/architecture.md`). O harness só garante execução sem shell e ambiente mínimo.
- Instalar/baixar servidores (`npx`, `uvx`): o host passa o executável e os args.
- Pool de processos ou um processo compartilhado entre vários `Client`s.

## Decisões em aberto (portão)

Recomendação e alternativas. A escolha vira ADR na fase de plan.

1. **Como montar o transporte (recomendado: A)**
   - **A. `mcp.CommandTransport` do SDK com um `exec.Cmd` montado pelo harness** (sem contexto de
     request, `SysProcAttr` com grupo de processos, env mínimo). Reusa encerramento e framing do
     SDK; o harness controla só a criação do processo. Contra: o `Close` do SDK sinaliza só o PID
     do filho, então o kill do grupo fica num complemento nosso.
   - **B. Transporte próprio sobre `mcp.IOTransport`** com pipes e ciclo de vida nossos. Controle
     total do encerramento. Contra: duplica o que o SDK já testa e aumenta a superfície de bug.
   - **C. Deixar o host injetar `Deps.Transport`** (sem feature). Custo zero no harness. Contra:
     cada host reimplementa a parte sensível de segurança; contraria ADR 0006 do `harness-cli`.
2. **Ambiente do filho (recomendado: allowlist mínima, FR-STD-004)**. Alternativas: herdar tudo e
   remover o que parece segredo (frágil, denylist) ou env totalmente vazio (quebra executáveis que
   dependem de `PATH`/`HOME`).
3. **Contrato de config em arquivo**: se `contracts/config` descrever servidores MCP, os campos
   stdio entram no JSON Schema e no gerado (§2 da constitution). Confirmar no plan.

## Dependências e espelho

- Depende de 004/005/009/011 (cliente MCP, catálogo, progresso, reconexão) e 017 (resources/prompts).
- Nenhuma dependência nova: `mcp.CommandTransport` já está no SDK em uso.
- Host: `harness-cli` (spec 002, tasks T2050–T2053) consome a release com esta feature e mapeia
  `mcpServers[].transport = "stdio"` para `Command`/`Args`/`Env`/`EnvCredentials`.
- Release: MINOR (`v0.4.0`), porque os campos novos em `Config` são aditivos.
