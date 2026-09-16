package telemetry

import "math"

// defaultBytesPerToken é a premissa de estimativa quando o host não configura
// Pricing.BytesPerToken (D-07/R5): 1 token ≈ 4 bytes.
const defaultBytesPerToken = 4

// estimateTokens converte volume em bytes para tokens estimados, sempre para
// cima (ceil) para nunca subestimar o consumo. Bytes não positivos devolvem 0;
// premissa de bytes/token inválida cai no default. Satura em MaxInt64 para
// premissas exóticas não estourarem o int64.
func estimateTokens(p Pricing, nBytes int) int64 {
	if nBytes <= 0 {
		return 0
	}

	bytesPerToken := p.BytesPerToken
	if bytesPerToken <= 0 {
		bytesPerToken = defaultBytesPerToken
	}

	estimativa := math.Ceil(float64(nBytes) / bytesPerToken)
	if estimativa >= float64(math.MaxInt64) {
		return math.MaxInt64
	}
	return int64(estimativa)
}

// costMicros devolve o custo em micros (arredondado para baixo) sem cache.
func costMicros(p Pricing, inputTokens, outputTokens int64) int64 {
	return costMicrosWithCache(p, inputTokens, outputTokens, 0, 0)
}

// costMicrosWithCache soma o custo considerando prompt caching (feature 006):
// o input não cacheado usa o preço de input, os tokens lidos usam o preço de
// cache e os escritos o preço de escrita; preço de cache zerado cai no de input
// (nunca subestima). InputTokens é o total (inclui os cacheados).
func costMicrosWithCache(p Pricing, inputTokens, outputTokens, cachedRead, cacheWrite int64) int64 {
	input := naoNegativo(inputTokens)
	output := naoNegativo(outputTokens)
	cached := naoNegativo(cachedRead)
	written := naoNegativo(cacheWrite)
	uncached := input - cached
	if uncached < 0 {
		uncached = 0
	}

	inputPrice := naoNegativo(p.InputPriceMicrosPerMillion)
	cachedPrice := p.CachedInputPriceMicrosPerMillion
	if cachedPrice <= 0 {
		cachedPrice = p.InputPriceMicrosPerMillion
	}
	writePrice := p.CacheWritePriceMicrosPerMillion
	if writePrice <= 0 {
		writePrice = p.InputPriceMicrosPerMillion
	}
	outputPrice := naoNegativo(p.OutputPriceMicrosPerMillion)

	total := somaSaturante(
		multiplicaSaturante(uncached, inputPrice),
		somaSaturante(
			multiplicaSaturante(cached, naoNegativo(cachedPrice)),
			somaSaturante(
				multiplicaSaturante(written, naoNegativo(writePrice)),
				multiplicaSaturante(output, outputPrice),
			),
		),
	)
	return total / 1_000_000
}

// usageReported monta o Usage a partir do uso reportado pelo provedor
// (estimated=false) e calcula o custo pela premissa do host (FR-025).
func usageReported(p Pricing, inputTokens, outputTokens int64) Usage {
	return usageReportedCached(p, inputTokens, outputTokens, 0, 0)
}

// usageReportedCached monta o Usage reportado incluindo os tokens de cache
// (feature 006), com o custo pelo preço de cache correspondente.
func usageReportedCached(p Pricing, inputTokens, outputTokens, cachedRead, cacheWrite int64) Usage {
	return Usage{
		InputTokens:       inputTokens,
		OutputTokens:      outputTokens,
		CachedInputTokens: cachedRead,
		CacheWriteTokens:  cacheWrite,
		Estimated:         false,
		CostMicros:        costMicrosWithCache(p, inputTokens, outputTokens, cachedRead, cacheWrite),
		Currency:          p.Currency,
	}
}

// usageEstimated estima os tokens pelo volume de bytes (medido antes da
// redação, ADR 0004) e devolve o Usage rotulado como estimativa (FR-025).
func usageEstimated(p Pricing, inputBytes, outputBytes int) Usage {
	inputTokens := estimateTokens(p, inputBytes)
	outputTokens := estimateTokens(p, outputBytes)

	return Usage{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		Estimated:    true,
		CostMicros:   costMicros(p, inputTokens, outputTokens),
		Currency:     p.Currency,
	}
}

// naoNegativo zera valores negativos de tokens ou preços.
func naoNegativo(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}

// multiplicaSaturante multiplica dois inteiros não negativos saturando em
// MaxInt64 quando o produto estouraria o int64.
func multiplicaSaturante(a, b int64) int64 {
	if a == 0 || b == 0 {
		return 0
	}
	if a > math.MaxInt64/b {
		return math.MaxInt64
	}
	return a * b
}

// somaSaturante soma dois inteiros não negativos saturando em MaxInt64.
func somaSaturante(a, b int64) int64 {
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}
