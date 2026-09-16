// Package memory implementa um SessionStore em memória para dev/teste.
package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/rmarquespaixao-eng/ia-harness/harness"
)

// ErrNotFound indica que a sessão pedida não existe no store.
var ErrNotFound = errors.New("store/memory: sessão não encontrada")

// ErrNilSession indica Save chamado com sessão nula.
var ErrNilSession = errors.New("store/memory: sessão nula")

// Store é um SessionStore em memória, seguro para uso concorrente.
type Store struct {
	mu   sync.Mutex
	data map[string]*harness.Session
}

// New cria um Store vazio.
func New() *Store {
	return &Store{data: make(map[string]*harness.Session)}
}

// Load devolve uma cópia profunda da sessão; ausente vira erro que envolve ErrNotFound.
func (s *Store) Load(ctx context.Context, sessionID string) (*harness.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sessao, ok := s.data[sessionID]
	if !ok {
		return nil, fmt.Errorf("store/memory: %q: %w", sessionID, ErrNotFound)
	}
	return cloneSession(sessao), nil
}

// Save armazena uma cópia profunda da sessão.
func (s *Store) Save(ctx context.Context, session *harness.Session) error {
	if session == nil {
		return ErrNilSession
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[session.ID] = cloneSession(session)
	return nil
}

// cloneSession devolve uma cópia profunda da sessão, incluindo Pending e Usage.
func cloneSession(s *harness.Session) *harness.Session {
	if s == nil {
		return nil
	}
	copia := *s
	copia.Messages = cloneMessages(s.Messages)
	if s.Pending != nil {
		pending := *s.Pending
		copia.Pending = &pending
	}
	return &copia
}

// cloneMessages copia o histórico e as partes de cada mensagem.
func cloneMessages(msgs []harness.Message) []harness.Message {
	if msgs == nil {
		return nil
	}
	out := make([]harness.Message, len(msgs))
	for i, msg := range msgs {
		out[i] = msg
		out[i].Parts = cloneParts(msg.Parts)
	}
	return out
}

// cloneParts copia cada parte e as estruturas aninhadas de call/result.
func cloneParts(parts []harness.Part) []harness.Part {
	if parts == nil {
		return nil
	}
	out := make([]harness.Part, len(parts))
	for i, part := range parts {
		out[i] = part
		out[i].Call = cloneToolCall(part.Call)
		out[i].Result = cloneToolResult(part.Result)
	}
	return out
}

// cloneToolCall copia a chamada e clona os bytes de Args.
func cloneToolCall(call *harness.ToolCall) *harness.ToolCall {
	if call == nil {
		return nil
	}
	copia := *call
	copia.Args = cloneRaw(call.Args)
	return &copia
}

// cloneToolResult copia o resultado e cada item de conteúdo.
func cloneToolResult(result *harness.ToolResult) *harness.ToolResult {
	if result == nil {
		return nil
	}
	copia := *result
	copia.Content = cloneContents(result.Content)
	return &copia
}

// cloneContents copia cada item e clona os bytes do JSON estruturado.
func cloneContents(items []harness.ResultContent) []harness.ResultContent {
	if items == nil {
		return nil
	}
	out := make([]harness.ResultContent, len(items))
	for i, item := range items {
		out[i] = item
		out[i].JSON = cloneRaw(item.JSON)
	}
	return out
}

// cloneRaw clona os bytes de um json.RawMessage preservando nil.
func cloneRaw(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}
