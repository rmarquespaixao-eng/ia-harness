//go:build !unix

package proc

// Group devolve nil em plataformas não-Unix (best-effort, documentado).
func Group() any {
	return nil
}

// KillGroup é no-op em plataformas não-Unix.
func KillGroup(_ int) error {
	return nil
}
