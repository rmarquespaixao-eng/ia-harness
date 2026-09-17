package semcache

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	core "github.com/rmarquespaixao-eng/ia-harness/internal/core"
)

// testRequest monta um turno elegível base: system + uma mensagem de texto.
func testRequest() core.ChatRequest {
	return core.ChatRequest{
		System: "você é um assistente financeiro",
		Messages: []core.Message{{
			Role:  core.RoleUser,
			Parts: []core.Part{{Kind: core.PartText, Text: "quanto gastei este mês?"}},
		}},
	}
}

// TestDefaults garante que valores ausentes/negativos caem no default e que os
// informados são preservados (FR-SC-001).
func TestDefaults(t *testing.T) {
	tests := []struct {
		name        string
		cfg         core.SemanticCacheConfig
		wantMin     float64
		wantMax     int
		wantEnabled bool
	}{
		{
			name:        "campos zerados usam os defaults",
			cfg:         core.SemanticCacheConfig{Enabled: true},
			wantMin:     DefaultMinScore,
			wantMax:     DefaultMaxEntries,
			wantEnabled: true,
		},
		{
			name:    "valores negativos usam os defaults",
			cfg:     core.SemanticCacheConfig{MinScore: -0.5, MaxEntries: -10},
			wantMin: DefaultMinScore,
			wantMax: DefaultMaxEntries,
		},
		{
			name:        "valores informados são preservados",
			cfg:         core.SemanticCacheConfig{Enabled: true, MinScore: 0.75, MaxEntries: 42},
			wantMin:     0.75,
			wantMax:     42,
			wantEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := Defaults(tt.cfg)

			// Assert
			require.InDelta(t, tt.wantMin, got.MinScore, 1e-9)
			assert.Equal(t, tt.wantMax, got.MaxEntries)
			assert.Equal(t, tt.wantEnabled, got.Enabled)
		})
	}
}

// TestEligible cobre FR-SC-002: cache ligado, contexto presente e ausência de
// tools, structured output e parâmetros não-determinísticos.
func TestEligible(t *testing.T) {
	tests := []struct {
		name   string
		cfg    core.SemanticCacheConfig
		tools  []core.Tool
		mutate func(*core.ChatRequest)
		want   bool
	}{
		{
			name: "elegível com cache ligado e contexto simples",
			cfg:  core.SemanticCacheConfig{Enabled: true},
			want: true,
		},
		{
			name: "cache desligado não é elegível",
			cfg:  core.SemanticCacheConfig{Enabled: false},
			want: false,
		},
		{
			name:  "catálogo de tools no argumento barra o cache",
			cfg:   core.SemanticCacheConfig{Enabled: true},
			tools: []core.Tool{{Name: "consultar_saldo"}},
			want:  false,
		},
		{
			name:   "tools no request barram o cache",
			cfg:    core.SemanticCacheConfig{Enabled: true},
			mutate: func(r *core.ChatRequest) { r.Tools = []core.Tool{{Name: "consultar_saldo"}} },
			want:   false,
		},
		{
			name:   "structured output barra o cache",
			cfg:    core.SemanticCacheConfig{Enabled: true},
			mutate: func(r *core.ChatRequest) { r.OutputSchema = json.RawMessage(`{"type":"object"}`) },
			want:   false,
		},
		{
			name:   "sem mensagens não é elegível",
			cfg:    core.SemanticCacheConfig{Enabled: true},
			mutate: func(r *core.ChatRequest) { r.Messages = nil },
			want:   false,
		},
		{
			name:   "parâmetro determinístico desconhecido não barra",
			cfg:    core.SemanticCacheConfig{Enabled: true},
			mutate: func(r *core.ChatRequest) { r.Params = map[string]any{"max_tokens": 100} },
			want:   true,
		},
	}

	for _, key := range []string{"temperature", "top_p", "top_k", "seed"} {
		tests = append(tests, struct {
			name   string
			cfg    core.SemanticCacheConfig
			tools  []core.Tool
			mutate func(*core.ChatRequest)
			want   bool
		}{
			name:   "parâmetro " + key + " barra o cache",
			cfg:    core.SemanticCacheConfig{Enabled: true},
			mutate: func(r *core.ChatRequest) { r.Params = map[string]any{key: 1} },
			want:   false,
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			req := testRequest()
			if tt.mutate != nil {
				tt.mutate(&req)
			}

			// Act
			got := Eligible(req, tt.tools, tt.cfg)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestCanonicalText garante representação determinística e sensível ao system e
// às mensagens, incluindo o texto das partes (FR-SC-003).
func TestCanonicalText(t *testing.T) {
	t.Run("determinístico para o mesmo request", func(t *testing.T) {
		// Arrange
		req := testRequest()

		// Act
		first := CanonicalText(req)
		second := CanonicalText(req)

		// Assert
		assert.Equal(t, first, second)
	})

	t.Run("muda quando o system muda", func(t *testing.T) {
		// Arrange
		req := testRequest()
		other := req
		other.System = "outro system"

		// Act
		got := CanonicalText(other)

		// Assert
		assert.NotEqual(t, CanonicalText(req), got)
	})

	t.Run("muda quando o texto da mensagem muda", func(t *testing.T) {
		// Arrange
		req := testRequest()
		other := testRequest()
		other.Messages[0].Parts[0].Text = "quanto gastei no cartão?"

		// Act & Assert
		assert.NotEqual(t, CanonicalText(req), CanonicalText(other))
	})

	t.Run("inclui o texto das partes", func(t *testing.T) {
		// Arrange
		req := testRequest()

		// Act
		got := CanonicalText(req)

		// Assert
		assert.Contains(t, got, "quanto gastei este mês?")
		assert.Contains(t, got, "user:")
	})
}

// TestKey garante chave estável para o mesmo request/modelo e distinta quando o
// modelo muda (FR-SC-003).
func TestKey(t *testing.T) {
	t.Run("estável para o mesmo request e modelo", func(t *testing.T) {
		// Arrange
		req := testRequest()

		// Act
		first := Key(req, "gpt-x")
		second := Key(req, "gpt-x")

		// Assert
		assert.Equal(t, first, second)
		assert.NotEmpty(t, first)
	})

	t.Run("muda quando o modelo muda", func(t *testing.T) {
		// Arrange
		req := testRequest()

		// Act & Assert
		assert.NotEqual(t, Key(req, "gpt-x"), Key(req, "gpt-y"))
	})

	t.Run("muda quando o contexto muda", func(t *testing.T) {
		// Arrange
		req := testRequest()
		other := testRequest()
		other.System = "outro system"

		// Act & Assert
		assert.NotEqual(t, Key(req, "gpt-x"), Key(other, "gpt-x"))
	})
}

// TestCosine cobre a similaridade de cosseno: idênticos, ortogonais, dimensões
// divergentes e vetor nulo nunca devolvem NaN.
func TestCosine(t *testing.T) {
	tests := []struct {
		name string
		a    []float32
		b    []float32
		want float64
	}{
		{name: "vetores idênticos", a: []float32{1, 2, 3}, b: []float32{1, 2, 3}, want: 1},
		{name: "vetores ortogonais", a: []float32{1, 0}, b: []float32{0, 1}, want: 0},
		{name: "dimensões divergentes", a: []float32{1, 2}, b: []float32{1, 2, 3}, want: 0},
		{name: "vetor nulo", a: []float32{0, 0, 0}, b: []float32{1, 2, 3}, want: 0},
		{name: "ambos vazios", a: nil, b: nil, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := Cosine(tt.a, tt.b)

			// Assert
			require.False(t, math.IsNaN(got))
			require.InDelta(t, tt.want, got, 1e-9)
		})
	}
}

// TestValid garante que vetores vazios e com NaN/Inf são rejeitados e que um
// vetor normal é aceito.
func TestValid(t *testing.T) {
	tests := []struct {
		name   string
		vector []float32
		want   bool
	}{
		{name: "vazio", vector: nil, want: false},
		{name: "slice vazio", vector: []float32{}, want: false},
		{name: "com NaN", vector: []float32{1, float32(math.NaN())}, want: false},
		{name: "com +Inf", vector: []float32{1, float32(math.Inf(1))}, want: false},
		{name: "com -Inf", vector: []float32{1, float32(math.Inf(-1))}, want: false},
		{name: "normal", vector: []float32{0.1, -0.2, 0.3}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := Valid(tt.vector)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestBest devolve o índice do maior score que atinge o limiar e -1 quando
// nenhum qualifica.
func TestBest(t *testing.T) {
	tests := []struct {
		name     string
		scores   []float64
		minScore float64
		want     int
	}{
		{name: "maior score acima do limiar", scores: []float64{0.5, 0.95, 0.7}, minScore: 0.9, want: 1},
		{name: "score exatamente no limiar qualifica", scores: []float64{0.9, 0.8}, minScore: 0.9, want: 0},
		{name: "nenhum atinge o limiar", scores: []float64{0.5, 0.8}, minScore: 0.9, want: -1},
		{name: "lista vazia", scores: nil, minScore: 0.9, want: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := Best(tt.scores, tt.minScore)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}
