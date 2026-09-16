package engine

import (
	gen "github.com/rmarquespaixao-eng/ia-harness/contracts/gen"
	session "github.com/rmarquespaixao-eng/ia-harness/internal/engine/session"
)

// SnapshotSession converte a sessão canônica no snapshot do contrato.
func SnapshotSession(s *Session) (gen.SessionSnapshot, error) {
	return session.SnapshotSession(s)
}

// RestoreSession reconstrói a sessão canônica a partir do snapshot do contrato.
func RestoreSession(snap gen.SessionSnapshot) (*Session, error) {
	return session.RestoreSession(snap)
}
