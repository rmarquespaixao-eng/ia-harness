package proc

import (
	"bytes"
	"io"
	"log/slog"
	"strings"
	"sync"
)

const (
	// maxLineBytes é o teto por linha de stderr (FR-STD-009).
	maxLineBytes = 4096
	// maxTotalBytes é o teto total por sessão de stderr (FR-STD-009).
	maxTotalBytes = 64 * 1024
)

// StderrSink é um io.Writer que quebra o stderr por linha, trunca (por linha
// e por total), redige credenciais e emite logger.Debug (FR-STD-009).
type StderrSink struct {
	mu        sync.Mutex
	logger    *slog.Logger
	secrets   []string
	buf       bytes.Buffer
	total     int
	truncated bool
}

// NewStderrSink cria o sink com o logger e os valores secretos a redigir.
func NewStderrSink(logger *slog.Logger, secrets []string) *StderrSink {
	return &StderrSink{logger: logger, secrets: secrets}
}

// Write implementa io.Writer. Dados são acumulados até o newline; linhas
// completas são emitidas como Debug, truncadas e redigidas.
func (s *StderrSink) Write(p []byte) (int, error) {
	if s.logger == nil {
		return len(p), nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	n := len(p)
	for len(p) > 0 {
		idx := bytes.IndexByte(p, '\n')
		if idx < 0 {
			s.buf.Write(p)
			break
		}
		s.buf.Write(p[:idx])
		s.emitLine(s.buf.String())
		s.buf.Reset()
		p = p[idx+1:]
	}
	return n, nil
}

// emitLine emite uma linha completa, truncando e redigindo conforme necessário.
func (s *StderrSink) emitLine(line string) {
	if s.truncated {
		return
	}
	s.total += len(line)
	if s.total > maxTotalBytes {
		s.truncated = true
		s.logger.Debug("[TOTAL TRUNCATED]")
		return
	}
	if len(line) > maxLineBytes {
		line = line[:maxLineBytes] + " [TRUNCATED]"
	}
	line = s.redact(line)
	s.logger.Debug(line)
}

// redact substitui os valores secretos por [REDACTED].
func (s *StderrSink) redact(line string) string {
	for _, secret := range s.secrets {
		if secret == "" {
			continue
		}
		line = strings.ReplaceAll(line, secret, "[REDACTED]")
	}
	return line
}

var _ io.Writer = (*StderrSink)(nil)
