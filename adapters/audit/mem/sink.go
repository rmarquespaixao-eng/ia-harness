// Package memory implementa um AuditSink em memória para dev/teste: acumula
// os eventos em ordem de emissão e permite inspecioná-los por Snapshot. Toda
// entrada/saída é copiada em profundidade, de modo que mutações no chamador ou
// no retorno não alteram o que está armazenado. Seguro para uso concorrente.
package memory

import (
	"context"
	"sync"

	"rmarquespaixao/ia-harness/harness"
)

// Sink acumula os eventos de auditoria emitidos, protegido por mutex.
type Sink struct {
	mu     sync.Mutex
	events []harness.AuditEvent
}

// New cria um sink vazio.
func New() *Sink {
	return &Sink{}
}

// Emit guarda uma cópia profunda do evento, preservando a ordem de emissão.
func (s *Sink) Emit(_ context.Context, ev harness.AuditEvent) error {
	copia := cloneEvent(ev)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, copia)

	return nil
}

// Snapshot devolve uma cópia profunda dos eventos na ordem em que chegaram;
// mutar o resultado (inclusive os campos apontados) não altera o armazenado.
func (s *Sink) Snapshot() []harness.AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()

	copias := make([]harness.AuditEvent, 0, len(s.events))
	for _, ev := range s.events {
		copias = append(copias, cloneEvent(ev))
	}
	return copias
}

// cloneEvent copia o evento e todos os campos apontados do contrato.
func cloneEvent(ev harness.AuditEvent) harness.AuditEvent {
	ev.ArgsRedacted = clonePtr(ev.ArgsRedacted)
	ev.ResultRedacted = clonePtr(ev.ResultRedacted)
	ev.CostMicros = clonePtr(ev.CostMicros)
	ev.Currency = clonePtr(ev.Currency)
	ev.Error = clonePtr(ev.Error)
	ev.InputTokens = clonePtr(ev.InputTokens)
	ev.Model = clonePtr(ev.Model)
	ev.OutputTokens = clonePtr(ev.OutputTokens)
	ev.Provider = clonePtr(ev.Provider)
	ev.Tool = clonePtr(ev.Tool)
	ev.ToolCallId = clonePtr(ev.ToolCallId)
	return ev
}

// clonePtr devolve um novo ponteiro para uma cópia do valor apontado.
func clonePtr[T any](v *T) *T {
	if v == nil {
		return nil
	}
	copia := *v
	return &copia
}
