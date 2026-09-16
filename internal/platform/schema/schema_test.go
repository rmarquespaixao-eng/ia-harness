package schema

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// toolSchema inline simula o inputSchema de uma tool MCP: draft 2020-12,
// tipos, enum, required e additionalProperties: false.
const toolSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "name": {"type": "string"},
    "age": {"type": "integer"},
    "status": {"enum": ["active", "inactive"]}
  },
  "required": ["name"],
  "additionalProperties": false
}`

// TestValidate cobre os casos de borda exigidos pela constitution §7:
// objeto válido, tipo errado, obrigatório ausente, campo adicional e enum.
func TestValidate(t *testing.T) {
	// Arrange
	v, err := Compile([]byte(toolSchema))
	require.NoError(t, err, "schema de teste precisa compilar")

	tests := []struct {
		name    string
		args    string
		wantErr bool
	}{
		{name: "objeto válido passa", args: `{"name":"ana","age":30,"status":"active"}`},
		{name: "tipo errado falha", args: `{"name":42}`, wantErr: true},
		{name: "campo obrigatório ausente falha", args: `{"age":30}`, wantErr: true},
		{name: "campo extra com additionalProperties false falha", args: `{"name":"ana","extra":true}`, wantErr: true},
		{name: "enum inválido falha", args: `{"name":"ana","status":"paused"}`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := v.Validate(json.RawMessage(tt.args))

			// Assert
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			var validationErr *jsonschema.ValidationError
			require.ErrorAs(t, err, &validationErr,
				"o erro precisa preservar o *jsonschema.ValidationError para inspeção")
		})
	}
}

// TestValidateArgsVazio garante que nil e vazio são tratados como "{}".
func TestValidateArgsVazio(t *testing.T) {
	// Arrange
	v, err := Compile([]byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`))
	require.NoError(t, err, "schema de teste precisa compilar")

	// Act
	errNil := v.Validate(nil)
	errVazio := v.Validate(json.RawMessage(""))

	// Assert
	require.NoError(t, errNil, "args nil deve valer contra {\"type\":\"object\"}")
	require.NoError(t, errVazio, "args vazio deve valer contra {\"type\":\"object\"}")
}

// TestValidateJSONMalformado garante erro (não pânico) para argumento inválido.
func TestValidateJSONMalformado(t *testing.T) {
	// Arrange
	v, err := Compile([]byte(`{"type":"object"}`))
	require.NoError(t, err, "schema de teste precisa compilar")

	// Act
	err = v.Validate(json.RawMessage(`{"name":`))

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema: argumento inválido")
	var validationErr *jsonschema.ValidationError
	assert.False(t, errors.As(err, &validationErr),
		"erro de sintaxe não é erro de validação")
}

// TestCompileJSONMalformado garante wrap "schema: compilar" no schema bruto.
func TestCompileJSONMalformado(t *testing.T) {
	// Arrange
	raw := []byte(`{"type":`)

	// Act
	_, err := Compile(raw)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema: compilar")
}

// TestValidatorReutilizado prova que o mesmo Validator compilado uma vez serve
// a validações sucessivas, com resultado independente.
func TestValidatorReutilizado(t *testing.T) {
	// Arrange
	v, err := Compile([]byte(toolSchema))
	require.NoError(t, err, "schema de teste precisa compilar")

	// Act
	primeira := v.Validate(json.RawMessage(`{"name":"ana"}`))
	segunda := v.Validate(json.RawMessage(`{"name":1}`))

	// Assert
	require.NoError(t, primeira, "primeira validação deve passar")
	require.Error(t, segunda, "segunda validação deve falhar no mesmo compilado")
}
