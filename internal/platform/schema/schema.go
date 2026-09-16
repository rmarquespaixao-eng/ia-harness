// Package schema compila JSON Schemas de tools MCP (inputSchema) ou de
// contracts/ uma única vez e valida argumentos em runtime, preservando o erro
// detalhado (*jsonschema.ValidationError) para inspeção do chamador —
// constitution §4 (validação sempre) e ADR 0001 (santhosh-tekuri/jsonschema/v6).
package schema

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// schemaURL é a localização sintética do recurso registrado no compilador;
// evita disco/rede e permite compilar o schema bruto recebido do servidor MCP.
const schemaURL = "schema.json"

// Validator é um JSON Schema já compilado e reutilizável entre validações.
type Validator struct {
	schema *jsonschema.Schema
}

// Compile carrega e compila um JSON Schema bruto. O draft é detectado pelo
// campo $schema do próprio documento (fallback: draft padrão da lib).
func Compile(raw []byte) (*Validator, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("schema: compilar: %w", err)
	}

	c := jsonschema.NewCompiler()
	if err := c.AddResource(schemaURL, doc); err != nil {
		return nil, fmt.Errorf("schema: compilar: %w", err)
	}

	compiled, err := c.Compile(schemaURL)
	if err != nil {
		return nil, fmt.Errorf("schema: compilar: %w", err)
	}

	return &Validator{schema: compiled}, nil
}

// Validate confere os argumentos contra o schema compilado. Argumento vazio ou
// nil equivale a "{}". Erros de sintaxe e de validação saem com wrap
// "schema: argumento inválido: %w", preservando o *jsonschema.ValidationError
// para inspeção com errors.As.
func (v *Validator) Validate(args json.RawMessage) error {
	if len(bytes.TrimSpace(args)) == 0 {
		args = []byte("{}")
	}

	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(args))
	if err != nil {
		return fmt.Errorf("schema: argumento inválido: %w", err)
	}

	if err := v.schema.Validate(doc); err != nil {
		return fmt.Errorf("schema: argumento inválido: %w", err)
	}

	return nil
}
