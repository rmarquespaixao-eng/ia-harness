// Package proc reúne regras reutilizáveis para iniciar processos filhos de
// forma segura (sem shell, ambiente mínimo, grupo de processos). Feature 023.
package proc

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNotFound indica que o executável não foi encontrado no PATH.
var ErrNotFound = errors.New("proc: executável não encontrado")

// ErrRelativePath indica que o comando é um caminho relativo com separador.
var ErrRelativePath = errors.New("proc: caminho relativo com separador recusado (use caminho absoluto ou nome simples)")

// Resolve valida e resolve o executável:
//   - Caminho absoluto: devolve como-is.
//   - Nome sem separador: resolve por exec.LookPath.
//   - Caminho relativo com separador (./x, bin/x): recusado (ErrRelativePath).
//   - Inexistente: ErrNotFound.
func Resolve(cmd string) (string, error) {
	if cmd == "" {
		return "", ErrNotFound
	}
	if filepath.IsAbs(cmd) {
		return cmd, nil
	}
	if strings.ContainsRune(cmd, filepath.Separator) || strings.ContainsRune(cmd, '/') {
		return "", ErrRelativePath
	}
	resolved, err := exec.LookPath(cmd)
	if err != nil {
		return "", ErrNotFound
	}
	return resolved, nil
}
