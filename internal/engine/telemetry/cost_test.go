package telemetry

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEstimateTokens_PremissaDefaultDeQuatroBytes(t *testing.T) {
	// Arrange — BytesPerToken zero cai no default de 4 bytes/token
	p := Pricing{}

	// Act e Assert
	assert.Equal(t, int64(0), estimateTokens(p, 0))
	assert.Equal(t, int64(1), estimateTokens(p, 4))
	assert.Equal(t, int64(2), estimateTokens(p, 5))
	assert.Equal(t, int64(2), estimateTokens(p, 8))
}

func TestEstimateTokens_PremissaNegativaCaiNoDefault(t *testing.T) {
	// Arrange
	p := Pricing{BytesPerToken: -2}

	// Act
	got := estimateTokens(p, 9)

	// Assert
	assert.Equal(t, int64(3), got)
}

func TestEstimateTokens_ArredondaParaCima(t *testing.T) {
	casos := []struct {
		nome          string
		bytesPerToken float64
		nBytes        int
		esperado      int64
	}{
		{"um byte por token", 1, 5, 5},
		{"fração exata não sobe", 4, 8, 2},
		{"fração não exata sobe", 4, 9, 3},
		{"premissa fracionária", 0.5, 3, 6},
		{"premissa quebrada", 2.5, 6, 3},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			// Arrange
			p := Pricing{BytesPerToken: caso.bytesPerToken}

			// Act
			got := estimateTokens(p, caso.nBytes)

			// Assert
			assert.Equal(t, caso.esperado, got)
		})
	}
}

func TestEstimateTokens_NaoPositivoDevolveZero(t *testing.T) {
	// Arrange
	p := Pricing{BytesPerToken: 4}

	// Act e Assert
	assert.Equal(t, int64(0), estimateTokens(p, 0))
	assert.Equal(t, int64(0), estimateTokens(p, -1))
	assert.Equal(t, int64(0), estimateTokens(p, -1024))
}

func TestEstimateTokens_VolumeExorbitanteSatura(t *testing.T) {
	// Arrange — premissa minúscula leva o quociente além do int64
	p := Pricing{BytesPerToken: math.SmallestNonzeroFloat64}

	// Act
	got := estimateTokens(p, math.MaxInt)

	// Assert
	assert.Equal(t, int64(math.MaxInt64), got)
}

func TestCostMicros_ArredondaParaBaixoNoTotal(t *testing.T) {
	casos := []struct {
		nome         string
		pricing      Pricing
		inputTokens  int64
		outputTokens int64
		esperado     int64
	}{
		{
			"preço exato por token",
			Pricing{InputPriceMicrosPerMillion: 3_000_000},
			1, 0, 3,
		},
		{
			"fração descartada",
			Pricing{InputPriceMicrosPerMillion: 1_500_000},
			1, 0, 1,
		},
		{
			"entrada e saída somam antes de dividir",
			Pricing{InputPriceMicrosPerMillion: 1_500_000, OutputPriceMicrosPerMillion: 1_500_000},
			1, 1, 3,
		},
		{
			"preços distintos por direção",
			Pricing{InputPriceMicrosPerMillion: 1_000_000, OutputPriceMicrosPerMillion: 2_000_000},
			10, 4, 18,
		},
		{
			"sem tokens zera",
			Pricing{InputPriceMicrosPerMillion: 3_000_000, OutputPriceMicrosPerMillion: 15_000_000},
			0, 0, 0,
		},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			// Arrange
			p := caso.pricing

			// Act
			got := costMicros(p, caso.inputTokens, caso.outputTokens)

			// Assert
			assert.Equal(t, caso.esperado, got)
		})
	}
}

func TestCostMicros_NegativosViramZero(t *testing.T) {
	// Arrange
	p := Pricing{InputPriceMicrosPerMillion: -1, OutputPriceMicrosPerMillion: -1}

	// Act
	got := costMicros(p, -10, -20)

	// Assert
	assert.Equal(t, int64(0), got)
}

func TestCostMicros_OverflowSaturaSemVirarNegativo(t *testing.T) {
	// Arrange — preço no teto do int64 com mais de um token estoura o produto
	p := Pricing{
		InputPriceMicrosPerMillion:  math.MaxInt64,
		OutputPriceMicrosPerMillion: math.MaxInt64,
	}

	// Act
	got := costMicros(p, 2, 2)

	// Assert
	assert.Positive(t, got)
	assert.Equal(t, int64(math.MaxInt64)/1_000_000, got)
}

func TestUsageReported_MarcaEstimativaFalsaECalculaCusto(t *testing.T) {
	// Arrange
	p := Pricing{
		Currency:                    "BRL",
		InputPriceMicrosPerMillion:  3_000_000,
		OutputPriceMicrosPerMillion: 15_000_000,
	}

	// Act
	got := usageReported(p, 100, 50)

	// Assert
	assert.Equal(t, Usage{
		InputTokens:  100,
		OutputTokens: 50,
		Estimated:    false,
		CostMicros:   1050,
		Currency:     "BRL",
	}, got)
}

func TestUsageEstimated_EstimaTokensECustoComRotulo(t *testing.T) {
	// Arrange
	p := Pricing{
		Currency:                    "USD",
		InputPriceMicrosPerMillion:  3_000_000,
		OutputPriceMicrosPerMillion: 15_000_000,
		BytesPerToken:               4,
	}

	// Act — 10 bytes de entrada viram 3 tokens; 4 bytes de saída viram 1
	got := usageEstimated(p, 10, 4)

	// Assert
	assert.Equal(t, Usage{
		InputTokens:  3,
		OutputTokens: 1,
		Estimated:    true,
		CostMicros:   24,
		Currency:     "USD",
	}, got)
}

func TestUsageEstimated_PremissaDefaultEVolumeZero(t *testing.T) {
	// Arrange — sem BytesPerToken configurado e sem volume
	p := Pricing{Currency: "BRL"}

	// Act
	got := usageEstimated(p, 0, 0)

	// Assert
	assert.Equal(t, Usage{
		InputTokens:  0,
		OutputTokens: 0,
		Estimated:    true,
		CostMicros:   0,
		Currency:     "BRL",
	}, got)
}
