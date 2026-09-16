package contracts

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gen "rmarquespaixao/ia-harness/contracts/gen"
	"rmarquespaixao/ia-harness/harness"
	"rmarquespaixao/ia-harness/internal/platform/schema"
)

// fixedAt é o instante determinístico usado nos fixtures (SC-009: sem relógio,
// rede ou modelo reais no teste).
var fixedAt = time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)

// genericDoc serializa um valor Go e devolve o documento genérico (map/[]any)
// que o validador recebe, reproduzindo o caminho de persistência do host.
func genericDoc(t *testing.T, value any) any {
	t.Helper()

	// Act — marshal do tipo público seguido de unmarshal para a forma genérica.
	raw, err := json.Marshal(value)
	require.NoError(t, err, "valor deve serializar para JSON")

	var doc any
	require.NoError(t, json.Unmarshal(raw, &doc), "JSON deve virar documento genérico")

	return doc
}

// assertConforms compila o schema embutido e valida o documento genérico,
// preservando a causa raiz do erro de validação (D-11).
func assertConforms(t *testing.T, schemaPath string, doc any) {
	t.Helper()

	raw, err := Schema(schemaPath)
	require.NoError(t, err, "schema %q deve estar embutido", schemaPath)

	validator, err := schema.Compile(raw)
	require.NoError(t, err, "schema %q deve compilar", schemaPath)

	encoded, err := json.Marshal(doc)
	require.NoError(t, err, "documento genérico deve serializar")

	require.NoError(t, validator.Validate(encoded), "marshal deve satisfazer %s", schemaPath)
}

// TestSessionSnapshotConformance prova que o snapshot da sessão produzido pelo
// núcleo satisfaz contracts/session/session_snapshot.json (FR-003/D-11).
func TestSessionSnapshotConformance(t *testing.T) {
	// Arrange — sessão completa: 2 mensagens com partes text/tool_call/tool_result,
	// confirmação pendente e consumo com moeda.
	session := harness.Session{
		ID:      "sess-001",
		UserID:  "user-42",
		AgentID: "agent-financeiro",
		Model:   "gpt-5.6-luna",
		State:   harness.SessionAwaitingConfirmation,
		Messages: []harness.Message{
			{
				ID:        "msg-001",
				Role:      harness.RoleUser,
				CreatedAt: fixedAt,
				Parts: []harness.Part{
					{Kind: harness.PartText, Text: "quanto gastei em setembro?"},
				},
			},
			{
				ID:        "msg-002",
				Role:      harness.RoleAssistant,
				CreatedAt: fixedAt.Add(time.Second),
				Parts: []harness.Part{
					{Kind: harness.PartText, Text: "vou consultar o extrato."},
					{
						Kind: harness.PartToolCall,
						Call: &harness.ToolCall{
							ID:        "call-001",
							Name:      "listar_transacoes",
							Namespace: "financeiro",
							Args:      json.RawMessage(`{"month":9,"year":2026}`),
						},
					},
					{
						Kind: harness.PartToolResult,
						Result: &harness.ToolResult{
							CallID: "call-001",
							Content: []harness.ResultContent{
								{Kind: harness.ResultText, Text: "3 transações"},
								{Kind: harness.ResultJSON, JSON: json.RawMessage(`{"total_cents":12345}`)},
							},
							Truncated: true,
						},
					},
				},
			},
		},
		Pending: &harness.PendingConfirmation{
			CallID:       "call-002",
			ToolName:     "transferir_transacoes",
			ArgsRedacted: `{"ids":["***"]}`,
			Reason:       "transferência exige confirmação humana",
			RequestedAt:  fixedAt.Add(2 * time.Second),
		},
		Usage: harness.Usage{
			InputTokens:  1200,
			OutputTokens: 340,
			Estimated:    false,
			CostMicros:   1500,
			Currency:     "BRL",
		},
		CreatedAt: fixedAt,
		UpdatedAt: fixedAt.Add(2 * time.Second),
	}

	// Act
	doc := genericDoc(t, session)

	// Assert — schema canônico e presença dos campos exigidos.
	assertConforms(t, "session/session_snapshot.json", doc)

	snapshot, ok := doc.(map[string]any)
	require.True(t, ok, "snapshot deve serializar como objeto JSON")
	for _, field := range []string{
		"id", "user_id", "agent_id", "model", "state",
		"messages", "usage", "created_at", "updated_at",
	} {
		assert.Contains(t, snapshot, field, "campo exigido pelo schema ausente: %s", field)
	}
	assert.Len(t, snapshot["messages"], 2)
}

// TestAuditEventConformance prova que o evento de auditoria do núcleo (alias do
// tipo gerado) satisfaz contracts/audit/audit_event.json (D-11).
func TestAuditEventConformance(t *testing.T) {
	// Arrange — execução de tool com tokens, custo e campos redigidos preenchidos.
	event := harness.AuditEvent{
		Id:             "audit-001",
		Kind:           gen.AuditEventKindToolCall,
		TraceId:        "trace-abc123",
		UserId:         "user-42",
		SessionId:      "sess-001",
		AgentId:        "agent-financeiro",
		Provider:       ptr("openai"),
		Model:          ptr("gpt-5.6-luna"),
		Tool:           ptr("listar_transacoes"),
		ToolCallId:     ptr("call-001"),
		Status:         gen.AuditEventStatusOk,
		InputTokens:    ptr(1200),
		OutputTokens:   ptr(340),
		Estimated:      false,
		CostMicros:     ptr(1500),
		Currency:       ptr("BRL"),
		LatencyMs:      42,
		ArgsRedacted:   ptr(`{"month":9,"year":2026}`),
		ResultRedacted: ptr(`{"count":3}`),
		OccurredAt:     fixedAt,
	}

	// Act
	doc := genericDoc(t, event)

	// Assert
	assertConforms(t, "audit/audit_event.json", doc)

	auditEvent, ok := doc.(map[string]any)
	require.True(t, ok, "evento deve serializar como objeto JSON")
	for _, field := range []string{
		"id", "kind", "trace_id", "user_id", "session_id",
		"agent_id", "status", "estimated", "latency_ms", "occurred_at",
	} {
		assert.Contains(t, auditEvent, field, "campo exigido pelo schema ausente: %s", field)
	}
}

// TestTurnEventEnvelopeConformance prova que os envelopes de fio do turno
// satisfazem contracts/events/turn_event_envelope.json (D-11).
func TestTurnEventEnvelopeConformance(t *testing.T) {
	// Arrange — um fragmento de texto e um desfecho de tool com métricas.
	at := fixedAt.Format(time.RFC3339)
	envelopes := map[string]map[string]any{
		"text_delta": {
			"type":       "text_delta",
			"session_id": "sess-001",
			"message_id": "msg-002",
			"text":       "Olá",
			"at":         at,
		},
		"tool_result": {
			"type":           "tool_result",
			"session_id":     "sess-001",
			"call_id":        "call-001",
			"tool":           "listar_transacoes",
			"namespace":      "financeiro",
			"status":         harness.StatusOK,
			"latency_ms":     123,
			"result_summary": "3 transações",
			"at":             at,
		},
	}

	// Act + Assert — cada envelope é serializado e validado isoladamente.
	for name, envelope := range envelopes {
		t.Run(name, func(t *testing.T) {
			doc := genericDoc(t, envelope)
			assertConforms(t, "events/turn_event_envelope.json", doc)
		})
	}
}

// TestHarnessConfigConformance prova o round-trip do arquivo de configuração:
// JSON de exemplo → gen.HarnessConfigFile → marshal de volta → schema (D-11).
func TestHarnessConfigConformance(t *testing.T) {
	// Arrange — arquivo de exemplo com modelos/capabilities/fallbacks, política
	// default deny com agente, pricing, context, redaction e servidor MCP. Nenhuma
	// credencial real: apenas referência resolvida pelo CredentialProvider.
	const configJSON = `{
  "default_model": "default",
  "models": {
    "default": {
      "provider": "openai",
      "model": "gpt-5.6-luna",
      "capabilities": {
        "tool_calling": true,
        "streaming": true,
        "max_context_tokens": 128000,
        "max_output_tokens": 8192
      },
      "params": {"temperature": 0.2},
      "fallbacks": ["backup"]
    },
    "backup": {
      "provider": "anthropic",
      "model": "claude-sonnet-5",
      "capabilities": {
        "tool_calling": true,
        "streaming": true,
        "max_context_tokens": 200000,
        "max_output_tokens": 8192
      }
    }
  },
  "policy": {
    "default": "deny",
    "agents": {
      "financeiro": {
        "mode": "read_only",
        "allow_tools": ["listar_*"],
        "deny_tools": ["excluir_*"],
        "confirm_tools": ["transferir_*"],
        "overrides": {
          "listar_contas": {"confirm": false, "read_only": true, "idempotent": true, "timeout_seconds": 30}
        }
      }
    }
  },
  "pricing": {
    "currency": "BRL",
    "input_price_micros_per_million": 1000000,
    "output_price_micros_per_million": 4000000,
    "bytes_per_token": 4
  },
  "context": {"max_tokens": 32000, "strategy": "summarize"},
  "redaction": {"sensitive_keys": ["authorization", "x-api-key"], "max_field_bytes": 512},
  "mcp_servers": [
    {
      "name": "financeiro",
      "endpoint": "https://mcp.local/financeiro",
      "credential_ref": "env:FINANCEIRO_API_KEY",
      "tool_timeout_seconds": 60
    }
  ]
}`

	var cfg gen.HarnessConfigFile
	require.NoError(t, json.Unmarshal([]byte(configJSON), &cfg), "config de exemplo deve desserializar no tipo gerado")

	// Act — re-serializa o que o host carregou (contrato de ida e volta).
	doc := genericDoc(t, cfg)

	// Assert — schema canônico e campos representativos preservados.
	assertConforms(t, "config/harness_config.json", doc)

	config, ok := doc.(map[string]any)
	require.True(t, ok, "config deve serializar como objeto JSON")
	assert.Equal(t, "default", config["default_model"])

	models, ok := config["models"].(map[string]any)
	require.True(t, ok, "models deve ser objeto")
	require.Len(t, models, 2)
	profile, ok := models["default"].(map[string]any)
	require.True(t, ok, "modelo default deve ser objeto")
	assert.Equal(t, []any{"backup"}, profile["fallbacks"], "fallbacks não podem ser perdidos no round-trip")
	capabilities, ok := profile["capabilities"].(map[string]any)
	require.True(t, ok, "capabilities deve ser objeto")
	assert.Equal(t, true, capabilities["tool_calling"])

	policy, ok := config["policy"].(map[string]any)
	require.True(t, ok, "policy deve ser objeto")
	assert.Equal(t, "deny", policy["default"])
	assert.Contains(t, policy["agents"], "financeiro")

	for _, field := range []string{"pricing", "context", "redaction", "mcp_servers"} {
		assert.Contains(t, config, field, "campo opcional preenchido ausente: %s", field)
	}
}

// forbiddenStorageTerms são os indícios de biblioteca/engine de armazenamento
// que o núcleo não pode carregar (FR-029).
var forbiddenStorageTerms = []string{"database", "pgx", "sqlite", "redis", "mongo"}

// stdlibStorageExemptions são pacotes da stdlib que apenas expõem as interfaces
// de driver de database/sql. Chegam indiretamente por github.com/google/uuid
// (internal/platform/trace) e não representam engine de armazenamento.
var stdlibStorageExemptions = map[string]bool{
	"database/sql/driver":   true,
	"database/sql/internal": true,
}

// TestHarnessPackageHasNoStorageDeps prova, sobre o grafo de imports real, que o
// núcleo não depende de armazenamento (FR-029/SC-009).
func TestHarnessPackageHasNoStorageDeps(t *testing.T) {
	// Arrange — o go lista as dependências transitivas do pacote público.
	cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", "./harness")
	cmd.Dir = ".." // raiz do repo: o teste roda com cwd em contracts/.

	// Act
	output, err := cmd.CombinedOutput()

	// Assert — o comando precisa rodar e o próprio harness aparecer no grafo.
	require.NoError(t, err, "go list -deps ./harness deve executar: %s", output)
	deps := strings.Split(strings.TrimSpace(string(output)), "\n")
	require.Contains(t, deps, "rmarquespaixao/ia-harness/harness")

	var offenders []string
	for _, dep := range deps {
		if stdlibStorageExemptions[dep] {
			continue
		}
		lower := strings.ToLower(dep)
		for _, term := range forbiddenStorageTerms {
			if strings.Contains(lower, term) {
				offenders = append(offenders, dep)
				break
			}
		}
	}
	assert.Empty(t, offenders, "núcleo não pode carregar biblioteca de armazenamento (FR-029)")
}

// ptr devolve o ponteiro do valor, usado nos campos opcionais dos tipos gerados.
func ptr[T any](v T) *T {
	return &v
}
