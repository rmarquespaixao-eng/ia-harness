package trace_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmarquespaixao/ia-harness/internal/platform/trace"
)

// chaveExterna simula outra chave de contexto, sem relação com o trace_id.
type chaveExterna struct{}

func TestIDFromContext_SemID(t *testing.T) {
	// Arrange
	casos := []struct {
		nome string
		ctx  context.Context
	}{
		{"contexto vazio", context.Background()},
		{"contexto com outra chave", context.WithValue(context.Background(), chaveExterna{}, "valor")},
		{"contexto com id vazio", trace.WithID(context.Background(), "")},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			// Act
			id, ok := trace.IDFromContext(caso.ctx)

			// Assert
			assert.False(t, ok)
			assert.Empty(t, id)
		})
	}
}

func TestWithID_RecuperaID(t *testing.T) {
	// Arrange
	ctx := trace.WithID(context.Background(), "trace-123")

	// Act
	id, ok := trace.IDFromContext(ctx)

	// Assert
	require.True(t, ok)
	assert.Equal(t, "trace-123", id)
}

func TestEnsureID_GeraUmaVezEPreserva(t *testing.T) {
	// Arrange
	ctx := context.Background()

	// Act
	ctxComID, primeiro := trace.EnsureID(ctx)
	_, segundo := trace.EnsureID(ctxComID)

	// Assert
	require.NotEmpty(t, primeiro)
	assert.Equal(t, primeiro, segundo)

	id, ok := trace.IDFromContext(ctxComID)
	require.True(t, ok)
	assert.Equal(t, primeiro, id)

	gerado, err := uuid.Parse(primeiro)
	require.NoError(t, err)
	assert.Equal(t, uuid.Version(4), gerado.Version())
}

func TestEnsureID_RespeitaIDExistente(t *testing.T) {
	// Arrange
	original := trace.WithID(context.Background(), "trace-existente")

	// Act
	ctx, id := trace.EnsureID(original)

	// Assert
	assert.Equal(t, "trace-existente", id)

	recuperado, ok := trace.IDFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, "trace-existente", recuperado)
}
