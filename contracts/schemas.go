// Package contracts expõe os JSON Schemas canônicos versionados em contracts/
// embutidos no binário. Os schemas são a fonte única do contrato (ADR 0006):
// contracts/gen/*.go são as structs geradas para serialização e persistência;
// este embed serve quem precisa do schema em runtime — validação do arquivo de
// configuração, teste de conformidade do marshal e geração de tipos do host.
package contracts

import (
	"embed"
	"fmt"
)

// schemaFiles embute todos os schemas de contrato (contracts/<assunto>/*.json).
//
//go:embed */*.json
var schemaFiles embed.FS

// Schema devolve o conteúdo bruto do schema identificado por um caminho
// relativo à raiz de contracts/ (ex.: "audit/audit_event.json").
// Caminho desconhecido é erro de programação e falha com mensagem explícita.
func Schema(path string) ([]byte, error) {
	data, err := schemaFiles.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("contracts: schema %q não embutido: %w", path, err)
	}
	return data, nil
}
