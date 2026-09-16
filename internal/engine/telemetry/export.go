package telemetry

// EstimateTokens é o wrapper exportado da estimativa de tokens por volume.
func EstimateTokens(p Pricing, nBytes int) int64 { return estimateTokens(p, nBytes) }

// UsageReported monta o Usage do uso reportado pelo provedor.
func UsageReported(p Pricing, inputTokens, outputTokens int64) Usage {
	return usageReported(p, inputTokens, outputTokens)
}

// UsageReportedCached monta o Usage reportado com tokens de prompt caching.
func UsageReportedCached(p Pricing, inputTokens, outputTokens, cachedRead, cacheWrite int64) Usage {
	return usageReportedCached(p, inputTokens, outputTokens, cachedRead, cacheWrite)
}

// UsageEstimated estima o Usage por volume de bytes.
func UsageEstimated(p Pricing, inputBytes, outputBytes int) Usage {
	return usageEstimated(p, inputBytes, outputBytes)
}
