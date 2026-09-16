package testutil

import (
	"context"

	"rmarquespaixao/ia-harness/harness"
)

// FakeAuditSink acumula eventos de auditoria em memória.
type FakeAuditSink struct {
	Events []harness.AuditEvent
	Err    error
}

// Emit appende o evento ou devolve o erro injetado, sem registrar o evento nesse caso.
func (s *FakeAuditSink) Emit(_ context.Context, ev harness.AuditEvent) error {
	if s.Err != nil {
		return s.Err
	}
	s.Events = append(s.Events, ev)
	return nil
}

var _ harness.AuditSink = (*FakeAuditSink)(nil)
