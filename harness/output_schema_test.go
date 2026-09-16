package harness_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/harness"
	"github.com/rmarquespaixao-eng/ia-harness/internal/testutil"
)

const outputSchema = `{"type":"object","properties":{"valor":{"type":"integer"}},"required":["valor"]}`

func TestStructuredOutput_ValidoPassa(t *testing.T) {
	// Arrange — o modelo responde dentro do schema.
	env := newTurnEnv(t, []testutil.ProviderStep{{Text: `{"valor": 10}`}})
	req := pergunta("extraia o valor")
	req.OutputSchema = json.RawMessage(outputSchema)

	// Act
	_, err := env.h.Run(context.Background(), req, &turnHandler{})

	// Assert
	require.NoError(t, err)
	require.Len(t, env.provider.Requests, 1)
	assert.JSONEq(t, outputSchema, string(env.provider.Requests[0].OutputSchema), "schema chega ao provider")
}

func TestStructuredOutput_InvalidoRetornaOutputError(t *testing.T) {
	// Arrange — o modelo responde fora do schema.
	env := newTurnEnv(t, []testutil.ProviderStep{{Text: `não é json`}})
	req := pergunta("extraia o valor")
	req.OutputSchema = json.RawMessage(outputSchema)

	// Act
	_, err := env.h.Run(context.Background(), req, &turnHandler{})

	// Assert
	var outErr *harness.OutputError
	require.ErrorAs(t, err, &outErr)
	assert.Equal(t, "output/schema-invalido", outErr.Code)
}
