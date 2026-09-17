package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	core "github.com/rmarquespaixao-eng/ia-harness/internal/core"
)

// TestFilterTools cobre a herança por lista vazia, o casamento exato por chave
// ou nome puro, o glob de namespace e o resultado vazio sem correspondência.
func TestFilterTools(t *testing.T) {
	tools := []core.Tool{
		{Namespace: "db", Name: "query"},
		{Namespace: "db", Name: "exec"},
		{Namespace: "web", Name: "fetch"},
		{Name: "ping"},
	}

	tests := []struct {
		name     string
		patterns []string
		want     []core.Tool
	}{
		{
			name:     "lista vazia herda o catálogo inteiro",
			patterns: nil,
			want:     tools,
		},
		{
			name:     "padrão exato com namespace",
			patterns: []string{"db.query"},
			want:     []core.Tool{tools[0]},
		},
		{
			name:     "padrão exato por nome puro",
			patterns: []string{"fetch"},
			want:     []core.Tool{tools[2]},
		},
		{
			name:     "glob de namespace",
			patterns: []string{"db.*"},
			want:     []core.Tool{tools[0], tools[1]},
		},
		{
			name:     "glob casa nome sem namespace",
			patterns: []string{"p*"},
			want:     []core.Tool{tools[3]},
		},
		{
			name:     "sem correspondência devolve vazio",
			patterns: []string{"nao-existe"},
			want:     []core.Tool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := FilterTools(tools, tt.patterns)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestDelegateTool garante o nome/namespace, o schema válido que exige agent e
// task e a descrição determinística com os nomes em ordem alfabética.
func TestDelegateTool(t *testing.T) {
	agents := map[string]core.AgentSpec{
		"zeta": {},
		"alfa": {Description: "primeiro"},
		"meio": {},
	}

	// Act
	tool := DelegateTool(agents)

	// Assert
	assert.Equal(t, DelegateName, tool.Name)
	assert.Equal(t, Namespace, tool.Namespace)
	assert.True(t, json.Valid(tool.InputSchema), "InputSchema deve ser JSON válido")

	var schema struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(tool.InputSchema, &schema))
	assert.ElementsMatch(t, []string{"agent", "task"}, schema.Required)
	assert.Contains(t, schema.Properties, "agent")
	assert.Contains(t, schema.Properties, "task")

	idxAlfa := strings.Index(tool.Description, "alfa")
	idxMeio := strings.Index(tool.Description, "meio")
	idxZeta := strings.Index(tool.Description, "zeta")
	require.GreaterOrEqual(t, idxAlfa, 0)
	require.GreaterOrEqual(t, idxMeio, 0)
	require.GreaterOrEqual(t, idxZeta, 0)
	assert.Less(t, idxAlfa, idxMeio)
	assert.Less(t, idxMeio, idxZeta)
	assert.Contains(t, tool.Description, "primeiro")
}

// TestCallContext cobre o roundtrip de WithCallContext/FromCallContext e o
// zero-value quando o contexto de delegação está ausente.
func TestCallContext(t *testing.T) {
	t.Run("roundtrip preserva os campos", func(t *testing.T) {
		// Arrange
		want := CallContext{Depth: 2, UserID: "u1", ParentSessionID: "s1"}

		// Act
		ctx := WithCallContext(context.Background(), want)
		got := FromCallContext(ctx)

		// Assert
		assert.Equal(t, want, got)
	})

	t.Run("ausente devolve zero-value", func(t *testing.T) {
		// Act
		got := FromCallContext(context.Background())

		// Assert
		assert.Equal(t, CallContext{}, got)
	})
}
