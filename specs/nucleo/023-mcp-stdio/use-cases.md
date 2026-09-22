# Use Cases: MCP stdio — servidor MCP como processo local (023)

Formato fully-dressed (Cockburn) + Gherkin. O operador implementa; o agente audita.

## CU-STD-1 — Host usa servidor MCP local como ToolSource

- **Identificador**: CU-STD-1 · **Escopo**: adaptador `mcpclient` · **Nível**: subfunção
- **Ator primário**: host (biblioteca embarcada, ex.: `harness-cli`)
- **Partes interessadas e interesses**: operador (plugar servidores MCP locais sem código de
  subprocesso no host); usuário (tools do servidor disponíveis ao agente)
- **Pré-condições**: `Config.Command` definido, `Endpoint` vazio; executável acessível
- **Gatilho**: primeira operação `List` ou `Call` do `Client`
- **Garantia de sucesso**: o processo roda sem shell, com ambiente mínimo, e publica as tools sob
  `Config.Name`
- **Garantia mínima**: nenhuma credencial exposta; nenhum processo órfão; erro nomeado em caso de
  falha
- **Mapeamento técnico**: `Client.ensureSession` → `transport()` → `exec.Cmd` (sem ctx de request)
  → `mcp.CommandTransport` → `sdk.Connect`
- **Fluxo principal de sucesso**:
  1. O host cria o `Client` com `Command`/`Args` e as portas; nada é iniciado.
  2. Na primeira operação, o harness resolve o executável e monta o ambiente mínimo com `Env`.
  3. O harness resolve cada `EnvCredentials` por `Deps.Credentials` e injeta só no env do filho.
  4. O processo inicia em grupo próprio, com stderr ligado ao log truncado e redigido.
  5. O handshake MCP conclui e o catálogo é publicado com o namespace de `Config.Name`.
- **Fluxos alternativos e de exceção**:
  - `2a` Executável não encontrado ou caminho relativo com separador: erro nomeado, sem processo.
  - `2b` `Endpoint` e `Command` juntos: erro de configuração.
  - `3a` Credencial não resolvida: erro nomeado sem o valor; processo não inicia.
  - `5a` Processo sai antes do handshake: erro nomeado com o código de saída; sem reinício em laço.
- **Regras de negócio**: RN-1 sem shell; RN-2 ambiente do host não herdado; RN-3 credencial só em
  env do filho, resolvida por início de processo.
- **Critérios de aceite (Gherkin)**:
```gherkin
Cenário: tools do processo local
  Dado um servidor MCP de teste compilado como executável temporário
  Quando o host chama List
  Então as tools aparecem com o namespace do Client
```
- **Rastreabilidade**: FR-STD-001..005, 010, 011, 012.

## CU-STD-2 — Queda do processo e reconexão

- **Identificador**: CU-STD-2 · **Escopo**: adaptador · **Nível**: subfunção
- **Pré-condições**: sessão stdio ativa
- **Gatilho**: o processo termina ou o pipe fecha durante ou entre operações
- **Garantia de sucesso**: a operação corrente é repetida uma vez sobre um processo novo
- **Garantia mínima**: no máximo um reinício por operação; o processo anterior é reaproveitado
  pelo `Wait`, sem zumbi
- **Fluxo principal**:
  1. A operação falha com conexão fechada.
  2. O harness invalida a sessão (encerra o processo antigo) e inicia outro.
  3. A operação é repetida e conclui.
- **Fluxos alternativos**: `3a` falha de novo: erro devolvido ao motor como falha de tool.
- **Rastreabilidade**: FR-STD-008.

## CU-STD-3 — Encerramento sem órfãos

- **Identificador**: CU-STD-3 · **Escopo**: adaptador · **Nível**: subfunção
- **Gatilho**: `Client.Close` (ou fim do host)
- **Garantia de sucesso**: processo e descendentes encerrados dentro de `TerminateTimeout`
  (+ margem do SIGKILL)
- **Fluxo principal**: fechar stdin → aguardar → SIGTERM ao grupo → aguardar → SIGKILL ao grupo.
- **Fluxos alternativos**: `1a` Windows: kill best-effort da árvore, documentado.
- **Rastreabilidade**: FR-STD-006, 007.

## Requisitos especiais (NFR)

- Determinismo: o servidor de teste é um binário Go compilado no `TestMain` (ou o padrão de
  re-execução do próprio binário de teste); nenhum teste toca rede nem depende de `npx`/`uvx`.
- Segurança: teste com valor-isca de credencial verificando argv, log, erro e stderr redigido;
  teste de não herança de env do host.
- Sem breaking change: `Config` sem `Command` mantém o comportamento HTTP inalterado.
