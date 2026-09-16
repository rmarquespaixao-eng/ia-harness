// Command contractgen gera os tipos Go a partir dos JSON Schemas versionados em
// contracts/<assunto>/*.json (fonte única do contrato — ADR 0006). O gerador é
// deliberadamente pequeno: invoca o go-jsonschema (tool directive do go.mod)
// sobre cada domínio e escreve contracts/gen/<dominio>.go.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

// domains são os assuntos versionados em contracts/<dir>; cada um gera um arquivo
// em <out-dir>/<file>. A ordem é determinística para manter o gerado estável.
var domains = []struct{ dir, file string }{
	{"contracts/audit", "audit.go"},
	{"contracts/config", "config.go"},
	{"contracts/events", "events.go"},
	{"contracts/session", "session.go"},
}

func main() {
	root := flag.String("root", ".", "raiz do repositório (contém contracts/ e go.mod)")
	outDir := flag.String("out-dir", "contracts/gen", "diretório de saída dos tipos gerados")
	flag.Parse()

	if err := run(*root, *outDir); err != nil {
		fmt.Fprintln(os.Stderr, "contractgen:", err)
		os.Exit(1)
	}
}

// run gera todos os domínios a partir de root para outDir (relativo a root
// quando outDir não é absoluto).
func run(root, outDir string) error {
	out := outDir
	if !filepath.IsAbs(out) {
		out = filepath.Join(root, outDir)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return fmt.Errorf("criar diretório de saída: %w", err)
	}

	for _, d := range domains {
		matches, err := filepath.Glob(filepath.Join(root, d.dir, "*.json"))
		if err != nil {
			return fmt.Errorf("listar schemas de %s: %w", d.dir, err)
		}
		if len(matches) == 0 {
			return fmt.Errorf("nenhum schema em %s", d.dir)
		}
		sort.Strings(matches)

		// Os caminhos passados ao gerador são relativos a root, pois é o Dir do comando.
		schemas := make([]string, 0, len(matches))
		for _, m := range matches {
			schemas = append(schemas, filepath.Join(d.dir, filepath.Base(m)))
		}

		args := append([]string{
			"tool", "github.com/atombender/go-jsonschema",
			"-p", "gen",
			"--tags", "json",
			"--struct-name-from-title",
			"-o", filepath.Join(out, d.file),
		}, schemas...)

		cmd := exec.Command("go", args...)
		cmd.Dir = root
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("gerar %s: %w", d.file, err)
		}
	}
	return nil
}
