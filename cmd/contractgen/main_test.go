package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRunIsIdempotent garante que gerar os contratos em um diretório limpo
// produz exatamente o mesmo conteúdo commitado em contracts/gen — o gate de
// "go generate sem diff" (constitution §2/§7, ADR 0006).
func TestRunIsIdempotent(t *testing.T) {
	// Arrange
	root := filepath.Join("..", "..")
	outDir := t.TempDir()

	// Act
	err := run(root, outDir)

	// Assert
	require.NoError(t, err, "contractgen falhou")
	for _, d := range domains {
		generated, readErr := os.ReadFile(filepath.Join(outDir, d.file))
		require.NoError(t, readErr)
		committed, readErr := os.ReadFile(filepath.Join(root, "contracts", "gen", d.file))
		require.NoError(t, readErr, "contracts/gen/%s não existe — rode `go generate ./...`", d.file)
		require.Equal(t, string(committed), string(generated),
			"contracts/gen/%s divergiu do gerado — rode `go generate ./...` e commite", d.file)
	}
}
