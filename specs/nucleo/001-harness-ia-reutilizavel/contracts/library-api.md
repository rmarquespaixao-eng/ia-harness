# Contract — API pública da biblioteca (`harness`)

**Feature**: `specs/nucleo/001-harness-ia-reutilizavel` | **Data**: 2026-09-15
Contrato de fachada em Go. Assinaturas são o contrato; corpos ficam para a implementação. Tipos canônicos em `data-model.md`.

## Fachada

```go
package harness // import "rmarquespaixao/ia-harness/harness"

// New valida a configuração (falha rápida: porta obrigatória ausente, perfil órfão,
// schema inválido) e monta o harness. Não abre rede, não lê credencial.
func New(cfg Config) (*Harness, error)

// Run executa um turno do agente de ponta a ponta e bloqueia até concluir
// (resposta final, pausa por confirmação ou erro). Eventos são entregues
// incrementalmente pelo Handler durante a execução.
// - SessionID vazio cria sessão nova.
// - ctx cancelado encerra o turno de forma consistente (FR-010).
func (h *Harness) Run(ctx context.Context, req RunRequest, handler Handler) (TurnResult, error)

// ResolveConfirmation retoma uma sessão em awaiting_confirmation (FR-018).
// Retorna o TurnResult do turno que havia pausado (o turno é retomado de onde pausou).
func (h *Harness) ResolveConfirmation(ctx context.Context, sessionID, callID string, d Decision, handler Handler) (TurnResult, error)

// Session devolve o estado atual da sessão (para inspeção pelo host).
func (h *Harness) Session(ctx context.Context, sessionID string) (*Session, error)

// Close libera conexões MCP e recursos dos adaptadores. Idempotente.
func (h *Harness) Close() error
```

```go
type RunRequest struct {
    SessionID string        // vazio = nova sessão
    UserID    string        // obrigatório (isolamento)
    AgentID   string        // escopo de política
    Model     string        // alias; vazio = default da configuração
    Input     []Part        // partes do usuário (hoje: texto)
    MaxIterations int       // default da config; 0 = default do núcleo
    Budget    Budget        // opcional
}

type Budget struct { MaxTokens int64; MaxWall time.Duration }

type TurnResult struct {
    SessionID string
    State     SessionState        // active | awaiting_confirmation | closed
    Output    []Part              // partes finais do assistente
    Pending   *PendingConfirmation
    Usage     Usage
    ToolCalls []ToolExecution     // resumo do turno (tool, status, latência)
    StopReason StopReason         // completed | max_iterations | budget | cancelled | error
    Model     string              // alias efetivamente usado (fallback aplicado)
}

type Decision struct { Approve bool; Reason string }
```

## Portas (interfaces fornecidas ou implementadas pelo host)

```go
type Provider interface {
    // Chat faz uma chamada de modelo com streaming de texto opcional.
    // O adaptador converte o formato nativo para o canônico (research R4).
    Chat(ctx context.Context, req ChatRequest, onText func(string)) (ChatResponse, error)
    Close() error
}

type ToolSource interface { // cliente MCP por trás
    List(ctx context.Context) ([]Tool, error)                       // FR-005
    Call(ctx context.Context, name string, args json.RawMessage,
         onProgress func(ProgressUpdate)) (ToolResult, error)       // FR-009/FR-011
    Close() error
}

type SessionStore interface { // FR-020 (host persiste; núcleo fornece impl. em memória)
    Load(ctx context.Context, sessionID string) (*Session, error)
    Save(ctx context.Context, s *Session) error
}

type AuditSink interface { Emit(ctx context.Context, ev AuditEvent) error } // FR-023

type MemoryStore interface { // FR-027
    Put(ctx context.Context, f Fact) error
    List(ctx context.Context, userID string) ([]Fact, error)
    Delete(ctx context.Context, userID, factID string) error
}

type Retriever interface { // FR-028
    Retrieve(ctx context.Context, userID, query string, k int) ([]RetrievedItem, error)
}

type Embedder interface { Embed(ctx context.Context, texts []string) ([][]float32, error) }

type CredentialProvider interface { // segredos nunca entram na Config (constitution §4)
    Resolve(ctx context.Context, ref string) (string, error) // refs: "env:VAR", "file:/path"
}

type Summarizer interface { Summarize(ctx context.Context, msgs []Message, maxTokens int) (string, error) }

type Clock interface { Now() time.Time } // time.Now() proibido fora da impl. de sistema
```

## Configuração e injeção de dependência (DI)

Adaptadores são **construídos pelo host** e injetados no `Config` (padrão `cmd/api/di.go` do financeiro). `New` não instancia provedor/MCP a partir de um `kind`; só valida e compõe — mantém cada adaptador como pacote próprio, sem ciclo com o núcleo (D-12).

```go
// No host (ex.: financeiro-api-v2/cmd/api/di.go):
providers := map[string]harness.Provider{
    "openrouter": openai.New(openai.Config{BaseURL: "https://openrouter.ai/api/v1", CredentialRef: "env:OPENROUTER_API_KEY"},
        openai.Deps{Credentials: creds, Clock: clk, Logger: log}),
    "claude": anthropic.New(anthropic.Config{Model: "claude-..."},
        anthropic.Deps{Credentials: creds, Clock: clk, Logger: log}),
}
tools := []harness.ToolSource{
    mcpclient.New(mcpclient.Config{Name: "financeiro", Endpoint: "https://<host>/mcp",
        CredentialRef: "env:FINANCEIRO_MCP_KEY"}, mcpclient.Deps{HTTPClient: httpc}),
}

cfg := harness.Config{
    Providers: providers,
    Tools:     tools,
    Models: map[string]harness.ModelProfile{
        "default": {Provider: "openrouter", Model: "...", Capabilities: caps, Fallbacks: []string{"claude"}},
    },
    // Policy, Pricing, Context, Redaction, credenciais/portas...
}
```

```go
type Config struct {
    Providers map[string]Provider // alias do provedor → adapter instanciado (DI)
    Models    map[string]ModelProfile
    Tools     []ToolSource        // um por servidor MCP instanciado (mcpclient.New)
    Policy    PolicyConfig
    Pricing   Pricing
    Context   ContextPolicy
    Redaction RedactionConfig
    DefaultModel string

    // Portas (obrigatórias marcadas com *)
    Credentials CredentialProvider // *
    Sessions    SessionStore       // *
    Logger      *slog.Logger       // *
    Clock       Clock              // default: clock do sistema (internal/platform/clock)
    Audit       AuditSink          // default: adapters/audit/log
    Memory      MemoryStore        // opcional
    Retriever   Retriever          // opcional
    Embedder    Embedder           // opcional
}
```

## Contratos gerados (JSON Schema)

Os contratos de fio/persistência vivem como JSON Schema em `contracts/{config,session,events,audit}/*.json` (fonte única) e geram structs em `contracts/gen` via `cmd/contractgen` + `go-jsonschema` (`go generate ./...` no gate, sem diff). A API pública usa tipos ergonômicos (`Session`, `AuditEvent`, …); o pacote `harness` expõe helpers de snapshot (`SnapshotSession`/`RestoreSession`) e o **teste de contrato** garante que o marshal dos tipos públicos satisfaz o schema. Hosts podem gerar tipos TS dos mesmos schemas.

## Invariantes do contrato (testáveis)

1. `New` sem `Credentials`, `Sessions` ou `Logger` → erro nomeado, sem panic.
2. `Run` sem política para o `AgentID` → **default deny**; nenhuma tool executa.
3. Nenhum tipo de SDK de provedor/MCP aparece na API pública.
4. `TurnResult.Model` reflete o alias efetivo após fallback; `StopReason` é sempre preenchido.
5. `Run` cancelado retorna `StopReason=cancelled` com a sessão consistente e persistida.
6. Erros são tipos do pacote `harness` (`*ConfigError`, `*PolicyError`, `*ToolError`, `*ProviderError`), com `errors.As`.
7. `New` **não** constrói adaptadores (D-12): `Providers` vazio → erro nomeado; `Tools` vazio é válido (agente sem tools); adapter nulo em `Providers`/`Tools` → erro nomeado.
8. `go generate ./...` não altera `contracts/gen` (idempotente) e o marshal dos tipos públicos valida contra os schemas de `contracts/`.
