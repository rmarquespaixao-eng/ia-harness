package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCostMicrosWithCache(t *testing.T) {
	// Arrange — input 1.0/M, output 2.0/M, cache read 0.1/M, cache write 1.25/M.
	p := Pricing{
		InputPriceMicrosPerMillion:       1_000_000,
		OutputPriceMicrosPerMillion:      2_000_000,
		CachedInputPriceMicrosPerMillion: 100_000,
		CacheWritePriceMicrosPerMillion:  1_250_000,
	}

	// Act — 1000 não cacheado + 4000 lidos + 0 escritos + 100 de saída.
	got := costMicrosWithCache(p, 5000, 100, 4000, 0)

	// Assert — 1000 + 400 + 200 = 1600 micros.
	assert.Equal(t, int64(1600), got)
}

func TestCostMicrosWithCache_SemPrecoDeCacheNaoSubestima(t *testing.T) {
	// Sem preço de cache, os tokens cacheados caem no preço de input.
	p := Pricing{InputPriceMicrosPerMillion: 1_000_000, OutputPriceMicrosPerMillion: 2_000_000}
	got := costMicrosWithCache(p, 5000, 100, 4000, 0)
	// 1000 + 4000 + 200 = 5200 (mesmo do cálculo sem cache para o mesmo input total).
	assert.Equal(t, costMicros(p, 5000, 100), got)
}
