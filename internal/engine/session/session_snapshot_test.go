package session

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gen "rmarquespaixao/ia-harness/contracts/gen"
)

// snapshotAt é o instante determinístico dos fixtures de snapshot.
var snapshotAt = time.Date(2026, time.September, 15, 9, 30, 0, 0, time.UTC)

// snapshotSessionFixture devolve uma sessão com todas as variantes do contrato:
// texto, tool call (args JSON), tool result (texto + JSON), pending, usage e
// timestamps.
func snapshotSessionFixture() *Session {
	return &Session{
		ID:      "sess-1",
		UserID:  "user-1",
		AgentID: "agent-1",
		Model:   "default",
		State:   SessionAwaitingConfirmation,
		Messages: []Message{
			{
				ID:        "msg-1",
				Role:      RoleSystem,
				Parts:     []Part{{Kind: PartText, Text: "prompt de sistema"}},
				CreatedAt: snapshotAt,
			},
			{
				ID:   "msg-2",
				Role: RoleAssistant,
				Parts: []Part{
					{Kind: PartText, Text: "vou chamar a tool"},
					{
						Kind: PartToolCall,
						Call: &ToolCall{
							ID:        "call-1",
							Name:      "criar_transacao",
							Namespace: "financeiro",
							Args:      json.RawMessage(`{"amount_cents":1500,"description":"café"}`),
						},
					},
				},
				CreatedAt: snapshotAt.Add(time.Minute),
			},
			{
				ID:   "msg-3",
				Role: RoleTool,
				Parts: []Part{
					{
						Kind: PartToolResult,
						Result: &ToolResult{
							CallID: "call-1",
							Content: []ResultContent{
								{Kind: ResultText, Text: "ok"},
								{Kind: ResultJSON, JSON: json.RawMessage(`{"id":"tx-1"}`)},
							},
							IsError:   true,
							Truncated: true,
						},
					},
				},
				CreatedAt: snapshotAt.Add(2 * time.Minute),
			},
		},
		Pending: &PendingConfirmation{
			CallID:       "call-1",
			ToolName:     "criar_transacao",
			ArgsRedacted: `{"amount_cents":"[REDACTED]"}`,
			Reason:       "tool destrutiva",
			RequestedAt:  snapshotAt,
		},
		Usage: UsageTotals{
			InputTokens:  123,
			OutputTokens: 45,
			Estimated:    true,
			CostMicros:   678,
			Currency:     "BRL",
		},
		CreatedAt: snapshotAt,
		UpdatedAt: snapshotAt.Add(3 * time.Minute),
	}
}

func TestSnapshotSession_RoundtripFielDeTodosOsCampos(t *testing.T) {
	// Arrange
	original := snapshotSessionFixture()

	// Act
	snapshot, err := SnapshotSession(original)
	require.NoError(t, err)

	// Assert — campos do tipo gerado do contrato mapeados um a um.
	assert.Equal(t, "sess-1", snapshot.Id)
	assert.Equal(t, "user-1", snapshot.UserId)
	assert.Equal(t, "agent-1", snapshot.AgentId)
	assert.Equal(t, "default", snapshot.Model)
	assert.Equal(t, gen.SessionSnapshotStateAwaitingConfirmation, snapshot.State)
	assert.Equal(t, original.CreatedAt, snapshot.CreatedAt)
	assert.Equal(t, original.UpdatedAt, snapshot.UpdatedAt)
	require.Len(t, snapshot.Messages, 3)
	assert.Equal(t, gen.MessageRoleSystem, snapshot.Messages[0].Role)
	assert.Equal(t, gen.MessageRoleAssistant, snapshot.Messages[1].Role)
	assert.Equal(t, gen.MessageRoleTool, snapshot.Messages[2].Role)
	require.NotNil(t, snapshot.Messages[1].Parts[1].Call)
	assert.Equal(t, "call-1", snapshot.Messages[1].Parts[1].Call.Id)
	assert.Equal(t, "criar_transacao", snapshot.Messages[1].Parts[1].Call.Name)
	assert.Equal(t, "financeiro", snapshot.Messages[1].Parts[1].Call.Namespace)
	assert.True(t, boolValue(snapshot.Messages[2].Parts[0].Result.IsError))
	assert.Nil(t, snapshot.Messages[2].Parts[0].Result.Denied)
	assert.True(t, boolValue(snapshot.Messages[2].Parts[0].Result.Truncated))
	require.NotNil(t, snapshot.Pending)
	assert.Equal(t, "call-1", snapshot.Pending.CallId)
	assert.Equal(t, "tool destrutiva", snapshot.Pending.Reason)
	assert.Equal(t, 123, snapshot.Usage.InputTokens)
	assert.Equal(t, 45, snapshot.Usage.OutputTokens)
	assert.True(t, snapshot.Usage.Estimated)
	require.NotNil(t, snapshot.Usage.CostMicros)
	assert.Equal(t, 678, *snapshot.Usage.CostMicros)
	require.NotNil(t, snapshot.Usage.Currency)
	assert.Equal(t, "BRL", *snapshot.Usage.Currency)

	// Act — volta para o tipo canônico.
	restored, err := RestoreSession(snapshot)

	// Assert — fidelidade total (inclusive bytes de args/JSON e timestamps).
	require.NoError(t, err)
	assert.Equal(t, original, restored)
}

func TestSnapshotSession_JSONPreservaArgsEJsonCrus(t *testing.T) {
	// Arrange + Act
	snapshot, err := SnapshotSession(snapshotSessionFixture())
	require.NoError(t, err)
	raw, err := json.Marshal(snapshot)
	require.NoError(t, err)

	// Assert — campos exigidos pelo contrato e JSON embutido, não string/base64.
	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &doc))
	for _, field := range []string{"id", "user_id", "agent_id", "model", "state", "messages", "usage", "created_at", "updated_at"} {
		assert.Contains(t, doc, field, "campo exigido pelo schema ausente: %s", field)
	}
	assert.Contains(t, string(raw), `"args":{"amount_cents":1500,"description":"café"}`)
	assert.Contains(t, string(raw), `"json":{"id":"tx-1"}`)
}

func TestSnapshotSession_ListasNulasViramArraysVazios(t *testing.T) {
	// Arrange — sessão recém-criada, sem histórico.
	snapshot, err := SnapshotSession(&Session{ID: "sess-1", UserID: "user-1", State: SessionActive})
	require.NoError(t, err)

	// Act
	raw, err := json.Marshal(snapshot)

	// Assert — o schema exige array; null quebraria o contrato.
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"messages":[]`)
	assert.NotContains(t, string(raw), `"messages":null`)
}

func TestRestoreSession_AposJSONDoContratoPreservaConteudo(t *testing.T) {
	// Arrange — caminho real do host: snapshot → JSON → tipo gerado decodificado.
	snapshot, err := SnapshotSession(snapshotSessionFixture())
	require.NoError(t, err)
	raw, err := json.Marshal(snapshot)
	require.NoError(t, err)
	var loaded gen.SessionSnapshot
	require.NoError(t, json.Unmarshal(raw, &loaded))

	// Act
	restored, err := RestoreSession(loaded)
	require.NoError(t, err)

	// Assert — args/JSON decodificados voltam ao mesmo valor JSON.
	original := snapshotSessionFixture()
	assert.JSONEq(t, string(original.Messages[1].Parts[1].Call.Args), string(restored.Messages[1].Parts[1].Call.Args))
	assert.JSONEq(t, string(original.Messages[2].Parts[0].Result.Content[1].JSON), string(restored.Messages[2].Parts[0].Result.Content[1].JSON))

	// E o snapshot reemitido é semanticamente idêntico ao primeiro.
	reenviado, err := SnapshotSession(restored)
	require.NoError(t, err)
	rawReenviado, err := json.Marshal(reenviado)
	require.NoError(t, err)
	assert.JSONEq(t, string(raw), string(rawReenviado))
}

func TestRestoreSession_EstadoInvalidoDevolveErro(t *testing.T) {
	// Act
	_, err := RestoreSession(gen.SessionSnapshot{
		Id:     "sess-1",
		UserId: "user-1",
		State:  gen.SessionSnapshotState("paused"),
	})

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "estado inválido")
	assert.Contains(t, err.Error(), "paused")
}

func TestSnapshotSession_EstadoInvalidoDevolveErro(t *testing.T) {
	// Act
	_, err := SnapshotSession(&Session{ID: "sess-1", UserID: "user-1", State: SessionState("expirada")})

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "estado inválido")
}

func TestSnapshotSession_SessaoNulaDevolveErro(t *testing.T) {
	// Act
	_, err := SnapshotSession(nil)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nula")
}

func TestSnapshotSession_MidiaRoundtrip(t *testing.T) {
	// Arrange — uma imagem inline e um documento por referência (feature 002).
	original := &Session{
		ID:     "sess-mm",
		UserID: "user-1",
		State:  SessionActive,
		Messages: []Message{
			{ID: "m1", Role: RoleUser, Parts: []Part{
				{Kind: PartText, Text: "olha"},
				{Kind: PartImage, Media: &Media{MIME: "image/png", Name: "f.png", SizeBytes: 4, Bytes: []byte("PNG!")}},
				{Kind: PartDocument, Media: &Media{MIME: "application/pdf", Name: "f.pdf", SizeBytes: 1, Reference: "https://x/f.pdf"}},
			}},
		},
	}

	// Act
	snapshot, err := SnapshotSession(original)
	require.NoError(t, err)

	// Assert — mapeamento para os tipos gerados.
	require.Len(t, snapshot.Messages, 1)
	require.Len(t, snapshot.Messages[0].Parts, 3)
	img := snapshot.Messages[0].Parts[1].Media
	require.NotNil(t, img)
	assert.Equal(t, "image/png", img.Mime)
	require.NotNil(t, img.Bytes)
	assert.NotEmpty(t, *img.Bytes)
	doc := snapshot.Messages[0].Parts[2].Media
	require.NotNil(t, doc)
	require.NotNil(t, doc.Reference)
	assert.Equal(t, "https://x/f.pdf", *doc.Reference)

	// Act — volta ao canônico.
	restored, err := RestoreSession(snapshot)

	// Assert
	require.NoError(t, err)
	require.Len(t, restored.Messages, 1)
	media := restored.Messages[0].Parts[1].Media
	require.NotNil(t, media)
	assert.Equal(t, "image/png", media.MIME)
	assert.Equal(t, "f.png", media.Name)
	assert.Equal(t, int64(4), media.SizeBytes)
	assert.Equal(t, []byte("PNG!"), media.Bytes)
}
