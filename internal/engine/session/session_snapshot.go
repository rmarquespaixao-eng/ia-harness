package session

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"

	gen "github.com/rmarquespaixao-eng/ia-harness/contracts/gen"
)

// SnapshotSession converte o histórico canônico no snapshot tipado do contrato
// contracts/session/session_snapshot.json — a fonte única do formato persistido
// pelo SessionStore do host (ADR 0006). Listas nulas viram listas vazias para
// permanecerem válidas no schema; args/json viajam como json.RawMessage, sem
// re-serialização, preservando os bytes originais.
func SnapshotSession(s *Session) (gen.SessionSnapshot, error) {
	if s == nil {
		return gen.SessionSnapshot{}, fmt.Errorf("harness: snapshot de sessão: sessão nula")
	}
	if !validSessionState(s.State) {
		return gen.SessionSnapshot{}, fmt.Errorf("harness: snapshot de sessão: estado inválido %q", s.State)
	}
	usage, err := snapshotUsage(s.Usage)
	if err != nil {
		return gen.SessionSnapshot{}, err
	}
	return gen.SessionSnapshot{
		Id:        s.ID,
		UserId:    s.UserID,
		AgentId:   s.AgentID,
		Model:     s.Model,
		State:     gen.SessionSnapshotState(s.State),
		Messages:  snapshotMessages(s.Messages),
		Pending:   snapshotPending(s.Pending),
		Usage:     usage,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}, nil
}

// RestoreSession reconstrói a sessão canônica a partir do snapshot do contrato;
// estado fora do enum do schema é erro explícito (snapshot inválido).
func RestoreSession(snap gen.SessionSnapshot) (*Session, error) {
	state := SessionState(snap.State)
	if !validSessionState(state) {
		return nil, fmt.Errorf("harness: snapshot de sessão: estado inválido %q", snap.State)
	}
	messages, err := restoreMessages(snap.Messages)
	if err != nil {
		return nil, err
	}
	return &Session{
		ID:        snap.Id,
		UserID:    snap.UserId,
		AgentID:   snap.AgentId,
		Model:     snap.Model,
		State:     state,
		Messages:  messages,
		Pending:   restorePending(snap.Pending),
		Usage:     restoreUsage(snap.Usage),
		CreatedAt: snap.CreatedAt,
		UpdatedAt: snap.UpdatedAt,
	}, nil
}

// validSessionState informa se o estado pertence ao enum do contrato de sessão.
func validSessionState(state SessionState) bool {
	switch state {
	case SessionActive, SessionAwaitingConfirmation, SessionClosed:
		return true
	default:
		return false
	}
}

// snapshotMessages converte o histórico canônico para os tipos gerados.
func snapshotMessages(messages []Message) []gen.Message {
	out := make([]gen.Message, 0, len(messages))
	for _, message := range messages {
		out = append(out, gen.Message{
			Id:        message.ID,
			Role:      gen.MessageRole(message.Role),
			Parts:     snapshotParts(message.Parts),
			CreatedAt: message.CreatedAt,
		})
	}
	return out
}

// restoreMessages converte o histórico gerado para o tipo canônico.
func restoreMessages(messages []gen.Message) ([]Message, error) {
	if messages == nil {
		return nil, nil
	}
	out := make([]Message, 0, len(messages))
	for i, message := range messages {
		parts, err := restoreParts(message.Parts)
		if err != nil {
			return nil, fmt.Errorf("harness: snapshot de sessão: mensagem %d: %w", i, err)
		}
		out = append(out, Message{
			ID:        message.Id,
			Role:      Role(message.Role),
			Parts:     parts,
			CreatedAt: message.CreatedAt,
		})
	}
	return out, nil
}

// snapshotParts converte as partes canônicas para os tipos gerados.
func snapshotParts(parts []Part) []gen.Part {
	out := make([]gen.Part, 0, len(parts))
	for _, part := range parts {
		out = append(out, gen.Part{
			Kind:   gen.PartKind(part.Kind),
			Text:   optionalString(part.Text),
			Call:   snapshotToolCall(part.Call),
			Result: snapshotToolResult(part.Result),
			Media:  snapshotMedia(part.Media),
		})
	}
	return out
}

// restoreParts converte as partes geradas para o tipo canônico.
func restoreParts(parts []gen.Part) ([]Part, error) {
	if parts == nil {
		return nil, nil
	}
	out := make([]Part, 0, len(parts))
	for i, part := range parts {
		call, err := restoreToolCall(part.Call)
		if err != nil {
			return nil, fmt.Errorf("parte %d: %w", i, err)
		}
		result, err := restoreToolResult(part.Result)
		if err != nil {
			return nil, fmt.Errorf("parte %d: %w", i, err)
		}
		media, err := restoreMedia(part.Media)
		if err != nil {
			return nil, fmt.Errorf("parte %d: %w", i, err)
		}
		out = append(out, Part{
			Kind:   PartKind(part.Kind),
			Text:   stringValue(part.Text),
			Call:   call,
			Result: result,
			Media:  media,
		})
	}
	return out, nil
}

// snapshotToolCall converte a chamada canônica; os args JSON viram json.RawMessage.
func snapshotToolCall(call *ToolCall) *gen.ToolCall {
	if call == nil {
		return nil
	}
	return &gen.ToolCall{
		Id:        call.ID,
		Name:      call.Name,
		Namespace: call.Namespace,
		Args:      rawToAny(call.Args),
	}
}

// restoreToolCall converte a chamada gerada, materializando os args em JSON.
func restoreToolCall(call *gen.ToolCall) (*ToolCall, error) {
	if call == nil {
		return nil, nil
	}
	args, err := anyToRaw(call.Args)
	if err != nil {
		return nil, err
	}
	return &ToolCall{
		ID:        call.Id,
		Name:      call.Name,
		Namespace: call.Namespace,
		Args:      args,
	}, nil
}

// snapshotToolResult converte o resultado canônico para os tipos gerados.
func snapshotToolResult(result *ToolResult) *gen.ToolResult {
	if result == nil {
		return nil
	}
	content := make([]gen.ResultContent, 0, len(result.Content))
	for _, item := range result.Content {
		content = append(content, gen.ResultContent{
			Kind: gen.ResultContentKind(item.Kind),
			Text: optionalString(item.Text),
			Json: rawToAny(item.JSON),
		})
	}
	return &gen.ToolResult{
		CallId:    result.CallID,
		Content:   content,
		IsError:   optionalBool(result.IsError),
		Denied:    optionalBool(result.Denied),
		Truncated: optionalBool(result.Truncated),
	}
}

// restoreToolResult converte o resultado gerado, materializando o JSON estruturado.
func restoreToolResult(result *gen.ToolResult) (*ToolResult, error) {
	if result == nil {
		return nil, nil
	}
	content := make([]ResultContent, 0, len(result.Content))
	for i, item := range result.Content {
		structured, err := anyToRaw(item.Json)
		if err != nil {
			return nil, fmt.Errorf("conteúdo %d: %w", i, err)
		}
		content = append(content, ResultContent{
			Kind: ResultContentKind(item.Kind),
			Text: stringValue(item.Text),
			JSON: structured,
		})
	}
	return &ToolResult{
		CallID:    result.CallId,
		Content:   content,
		IsError:   boolValue(result.IsError),
		Denied:    boolValue(result.Denied),
		Truncated: boolValue(result.Truncated),
	}, nil
}

// snapshotMedia converte a mídia canônica (bytes inline em base64) para o tipo
// gerado; nome/ref vazios viram nil (omissão no schema).
func snapshotMedia(media *Media) *gen.Media {
	if media == nil {
		return nil
	}
	out := &gen.Media{Mime: media.MIME, SizeBytes: int(media.SizeBytes)}
	if media.Name != "" {
		out.Name = optionalString(media.Name)
	}
	if media.Reference != "" {
		out.Reference = optionalString(media.Reference)
	}
	if len(media.Bytes) > 0 {
		out.Bytes = optionalString(base64.StdEncoding.EncodeToString(media.Bytes))
	}
	return out
}

// restoreMedia reconstrói a mídia canônica, decodificando bytes base64.
func restoreMedia(media *gen.Media) (*Media, error) {
	if media == nil {
		return nil, nil
	}
	out := &Media{
		MIME:      media.Mime,
		Name:      stringValue(media.Name),
		SizeBytes: int64(media.SizeBytes),
		Reference: stringValue(media.Reference),
	}
	if media.Bytes != nil && *media.Bytes != "" {
		decoded, err := base64.StdEncoding.DecodeString(*media.Bytes)
		if err != nil {
			return nil, fmt.Errorf("media: bytes base64 inválidos: %w", err)
		}
		out.Bytes = decoded
	}
	return out, nil
}

// snapshotPending converte a confirmação pendente canônica para o tipo gerado.
func snapshotPending(pending *PendingConfirmation) *gen.PendingConfirmation {
	if pending == nil {
		return nil
	}
	return &gen.PendingConfirmation{
		CallId:       pending.CallID,
		ToolName:     pending.ToolName,
		ArgsRedacted: pending.ArgsRedacted,
		Reason:       pending.Reason,
		RequestedAt:  pending.RequestedAt,
	}
}

// restorePending converte a confirmação pendente gerada para o tipo canônico.
func restorePending(pending *gen.PendingConfirmation) *PendingConfirmation {
	if pending == nil {
		return nil
	}
	return &PendingConfirmation{
		CallID:       pending.CallId,
		ToolName:     pending.ToolName,
		ArgsRedacted: pending.ArgsRedacted,
		Reason:       pending.Reason,
		RequestedAt:  pending.RequestedAt,
	}
}

// snapshotUsage converte o consumo canônico; o tipo gerado usa int e ponteiros
// para os opcionais, então valores fora do intervalo de int são erro.
func snapshotUsage(usage UsageTotals) (gen.UsageTotals, error) {
	input, err := int64ToInt(usage.InputTokens)
	if err != nil {
		return gen.UsageTotals{}, fmt.Errorf("harness: snapshot de sessão: input_tokens: %w", err)
	}
	output, err := int64ToInt(usage.OutputTokens)
	if err != nil {
		return gen.UsageTotals{}, fmt.Errorf("harness: snapshot de sessão: output_tokens: %w", err)
	}
	var cost *int
	if usage.CostMicros != 0 {
		converted, err := int64ToInt(usage.CostMicros)
		if err != nil {
			return gen.UsageTotals{}, fmt.Errorf("harness: snapshot de sessão: cost_micros: %w", err)
		}
		cost = &converted
	}
	return gen.UsageTotals{
		InputTokens:       input,
		OutputTokens:      output,
		Estimated:         usage.Estimated,
		CostMicros:        cost,
		Currency:          optionalString(usage.Currency),
		CachedInputTokens: optionalInt(usage.CachedInputTokens),
		CacheWriteTokens:  optionalInt(usage.CacheWriteTokens),
	}, nil
}

// restoreUsage converte o consumo gerado para o tipo canônico.
func restoreUsage(usage gen.UsageTotals) UsageTotals {
	return UsageTotals{
		InputTokens:       int64(usage.InputTokens),
		OutputTokens:      int64(usage.OutputTokens),
		Estimated:         usage.Estimated,
		CostMicros:        int64Value(usage.CostMicros),
		Currency:          stringValue(usage.Currency),
		CachedInputTokens: int64Value(usage.CachedInputTokens),
		CacheWriteTokens:  int64Value(usage.CacheWriteTokens),
	}
}

// optionalInt devolve o ponteiro do valor positivo (omissão do zero).
func optionalInt(value int64) *int {
	if value <= 0 {
		return nil
	}
	if value > math.MaxInt {
		converted := math.MaxInt
		return &converted
	}
	converted := int(value)
	return &converted
}

// rawToAny preserva o JSON cru como json.RawMessage (que serializa os bytes
// originais verbatim); nil permanece nil.
func rawToAny(raw json.RawMessage) any {
	if raw == nil {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

// anyToRaw materializa um valor JSON dinâmico em json.RawMessage; o caminho
// json.RawMessage/[]byte é cópia direta e os demais são re-serializados.
func anyToRaw(value any) (json.RawMessage, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case json.RawMessage:
		return append(json.RawMessage(nil), typed...), nil
	case []byte:
		return append(json.RawMessage(nil), typed...), nil
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return nil, fmt.Errorf("payload JSON inválido: %w", err)
		}
		return raw, nil
	}
}

// optionalString devolve o ponteiro do valor não vazio (omissão do campo).
func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// stringValue desreferencia o ponteiro, tratando nil como vazio.
func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// optionalBool devolve o ponteiro do valor verdadeiro (omissão do falso).
func optionalBool(value bool) *bool {
	if !value {
		return nil
	}
	return &value
}

// boolValue desreferencia o ponteiro, tratando nil como falso.
func boolValue(value *bool) bool {
	return value != nil && *value
}

// int64Value desreferencia o ponteiro, tratando nil como zero.
func int64Value(value *int) int64 {
	if value == nil {
		return 0
	}
	return int64(*value)
}

// int64ToInt converte com checagem de intervalo (o tipo gerado usa int).
func int64ToInt(value int64) (int, error) {
	if value > math.MaxInt || value < math.MinInt {
		return 0, fmt.Errorf("valor %d fora do intervalo de int", value)
	}
	return int(value), nil
}
