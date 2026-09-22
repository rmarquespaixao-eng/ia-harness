package proc_test

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/internal/platform/proc"
)

func TestResolve(t *testing.T) {
	t.Run("absolute_path_ok", func(t *testing.T) {
		abs := "/bin/sh"
		if runtime.GOOS == "windows" {
			abs = `C:\Windows\System32\cmd.exe`
		}
		got, err := proc.Resolve(abs)
		require.NoError(t, err)
		assert.Equal(t, abs, got)
	})

	t.Run("name_by_path", func(t *testing.T) {
		got, err := proc.Resolve("go")
		require.NoError(t, err)
		assert.NotEmpty(t, got)
		assert.Equal(t, filepath.Base(got), "go"+exeSuffix())
	})

	t.Run("relative_with_separator_rejected", func(t *testing.T) {
		_, err := proc.Resolve("./server")
		require.Error(t, err)
		assert.ErrorIs(t, err, proc.ErrRelativePath)
	})

	t.Run("relative_bin_separator_rejected", func(t *testing.T) {
		_, err := proc.Resolve("bin/server")
		require.Error(t, err)
		assert.ErrorIs(t, err, proc.ErrRelativePath)
	})

	t.Run("not_found", func(t *testing.T) {
		_, err := proc.Resolve("nao-existe-iah-test")
		require.Error(t, err)
		assert.ErrorIs(t, err, proc.ErrNotFound)
	})

	t.Run("name_with_spaces_rejected", func(t *testing.T) {
		_, err := proc.Resolve("npx -y pkg")
		require.Error(t, err)
		assert.True(t, errors.Is(err, proc.ErrNotFound) || errors.Is(err, proc.ErrRelativePath),
			"nome com espaço deve ser recusado: %v", err)
	})
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
