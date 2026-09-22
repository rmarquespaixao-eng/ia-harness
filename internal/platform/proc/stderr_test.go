package proc_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/internal/platform/proc"
)

func TestStderrSink(t *testing.T) {
	t.Run("breaks_by_line_and_emits_debug", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		sink := proc.NewStderrSink(logger, nil)

		_, err := sink.Write([]byte("line1\nline2\n"))
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "line1")
		assert.Contains(t, output, "line2")
		assert.Contains(t, output, "level=DEBUG")
	})

	t.Run("truncates_per_line", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		sink := proc.NewStderrSink(logger, nil)

		long := strings.Repeat("x", 8192)
		_, err := sink.Write([]byte(long + "\n"))
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "[TRUNCATED]")
		assert.Less(t, len(output), 8192)
	})

	t.Run("truncates_total", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		sink := proc.NewStderrSink(logger, nil)

		line := strings.Repeat("a", 1024) + "\n"
		for i := 0; i < 100; i++ {
			_, _ = sink.Write([]byte(line))
		}

		output := buf.String()
		assert.Contains(t, output, "[TOTAL TRUNCATED]")
	})

	t.Run("redacts_secrets", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		sink := proc.NewStderrSink(logger, []string{"super-secret-token"})

		_, err := sink.Write([]byte("error: token=super-secret-token failed\n"))
		require.NoError(t, err)

		output := buf.String()
		assert.NotContains(t, output, "super-secret-token")
		assert.Contains(t, output, "[REDACTED]")
	})

	t.Run("partial_line_buffered", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		sink := proc.NewStderrSink(logger, nil)

		_, _ = sink.Write([]byte("partial"))
		assert.Empty(t, buf.String(), "linha incompleta não deve emitir")

		_, _ = sink.Write([]byte(" complete\n"))
		assert.Contains(t, buf.String(), "partial complete")
	})
}
