package proc_test

import (
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/internal/platform/proc"
)

func TestMinimalEnv(t *testing.T) {
	t.Run("only_allowlist_from_host", func(t *testing.T) {
		t.Setenv("PATH", "/usr/bin")
		t.Setenv("HOME", "/home/test")
		t.Setenv("LANG", "en_US.UTF-8")
		t.Setenv("TMPDIR", "/tmp")
		t.Setenv("SEGREDO_HOST", "deveria-nao-vazar")

		env := proc.MinimalEnv(nil)
		envMap := envToMap(env)

		assert.Equal(t, "/usr/bin", envMap["PATH"])
		assert.Equal(t, "/home/test", envMap["HOME"])
		assert.Equal(t, "en_US.UTF-8", envMap["LANG"])
		assert.Equal(t, "/tmp", envMap["TMPDIR"])
		_, hasSegredo := envMap["SEGREDO_HOST"]
		assert.False(t, hasSegredo, "variável fora da allowlist não deve vazar")
	})

	t.Run("extra_overrides", func(t *testing.T) {
		t.Setenv("PATH", "/usr/bin")

		env := proc.MinimalEnv(map[string]string{"PATH": "/custom/bin", "CUSTOM": "val"})
		envMap := envToMap(env)

		assert.Equal(t, "/custom/bin", envMap["PATH"], "extra deve sobrepor o host")
		assert.Equal(t, "val", envMap["CUSTOM"])
	})

	t.Run("deterministic_order", func(t *testing.T) {
		t.Setenv("PATH", "/usr/bin")
		t.Setenv("HOME", "/home")

		first := proc.MinimalEnv(map[string]string{"Z": "1", "A": "2"})
		second := proc.MinimalEnv(map[string]string{"A": "2", "Z": "1"})

		require.Equal(t, len(first), len(second))
		firstKeys := envKeys(first)
		secondKeys := envKeys(second)
		assert.Equal(t, firstKeys, secondKeys, "ordem deve ser determinística")
	})

	t.Run("missing_host_var_absent", func(t *testing.T) {
		t.Setenv("TMPDIR", "")

		env := proc.MinimalEnv(nil)
		envMap := envToMap(env)

		if runtime.GOOS != "windows" {
			_, hasTmpdir := envMap["TMPDIR"]
			assert.False(t, hasTmpdir, "TMPDIR vazio no host não deve aparecer")
		}
	})
}

func envToMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		k, v, _ := strings.Cut(e, "=")
		m[k] = v
	}
	return m
}

func envKeys(env []string) []string {
	keys := make([]string, 0, len(env))
	for _, e := range env {
		k, _, _ := strings.Cut(e, "=")
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
