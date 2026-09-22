package proc

import (
	"os"
	"runtime"
	"sort"
)

// hostAllowlist são as variáveis do host que o filho pode herdar (FR-STD-004).
var hostAllowlist = []string{"PATH", "HOME", "LANG", "TMPDIR"}

// windowsAllowlist são variáveis adicionais no Windows.
var windowsAllowlist = []string{"SystemRoot", "ComSpec", "PATHEXT"}

// MinimalEnv monta o ambiente do filho: allowlist mínima do host + extra, em
// ordem determinística. Variáveis do host fora da allowlist não aparecem.
// Extra sobrepõe o valor do host. Variáveis do host vazias são omitidas.
func MinimalEnv(extra map[string]string) []string {
	allow := hostAllowlist
	if runtime.GOOS == "windows" {
		allow = append(append([]string(nil), hostAllowlist...), windowsAllowlist...)
	}

	seen := make(map[string]bool, len(allow)+len(extra))
	var env []string

	for _, key := range allow {
		if seen[key] {
			continue
		}
		seen[key] = true
		if v, ok := extra[key]; ok {
			env = append(env, key+"="+v)
			continue
		}
		if v := os.Getenv(key); v != "" {
			env = append(env, key+"="+v)
		}
	}

	extraKeys := make([]string, 0, len(extra))
	for k := range extra {
		if !seen[k] {
			extraKeys = append(extraKeys, k)
		}
	}
	sort.Strings(extraKeys)
	for _, k := range extraKeys {
		env = append(env, k+"="+extra[k])
	}

	return env
}
