// Package semcache contém a regra pura do cache semântico de respostas
// (feature 020): elegibilidade, chave canônica e similaridade de cosseno. Não
// faz I/O; o armazenamento é porta do host (adapters/cache/inmem no dev).
package semcache

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"sort"
	"strings"

	core "github.com/rmarquespaixao-eng/ia-harness/internal/core"
)

const (
	// DefaultMinScore é o limiar de similaridade quando o host não define.
	DefaultMinScore = 0.9
	// DefaultMaxEntries é o teto de entradas quando o host não define.
	DefaultMaxEntries = 1000
)

// Defaults preenche MinScore/MaxEntries ausentes.
func Defaults(cfg core.SemanticCacheConfig) core.SemanticCacheConfig {
	if cfg.MinScore <= 0 {
		cfg.MinScore = DefaultMinScore
	}
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = DefaultMaxEntries
	}
	return cfg
}

// Eligible informa se o turno pode usar o cache semântico (FR-SC-002): cache
// ligado, contexto presente, sem tools, sem structured output e sem parâmetros
// não-determinísticos.
func Eligible(req core.ChatRequest, tools []core.Tool, cfg core.SemanticCacheConfig) bool {
	if !cfg.Enabled {
		return false
	}
	if len(tools) > 0 || len(req.Tools) > 0 {
		return false
	}
	if len(req.OutputSchema) > 0 {
		return false
	}
	if len(req.Messages) == 0 {
		return false
	}
	for _, key := range nonDeterministicParams {
		if _, ok := req.Params[key]; ok {
			return false
		}
	}
	return true
}

// nonDeterministicParams são chaves de Params que impedem o cache: a resposta
// deixa de ser função determinística do contexto.
var nonDeterministicParams = []string{"temperature", "top_p", "top_k", "seed"}

// CanonicalText monta a representação canônica do contexto (system + mensagens)
// usada para gerar o vetor e a chave.
func CanonicalText(req core.ChatRequest) string {
	var b strings.Builder
	b.WriteString("system:")
	b.WriteString(strings.TrimSpace(req.System))
	for _, message := range req.Messages {
		b.WriteString("\n")
		b.WriteString(string(message.Role))
		b.WriteString(":")
		b.WriteString(messageText(message))
	}
	return b.String()
}

// messageText concatena o payload textual/estruturado de uma mensagem.
func messageText(message core.Message) string {
	var b strings.Builder
	for _, part := range message.Parts {
		switch {
		case part.Text != "":
			b.WriteString(part.Text)
		case part.Call != nil:
			b.WriteString(part.Call.Name)
			b.WriteString("(")
			b.WriteString(string(part.Call.Args))
			b.WriteString(")")
		case part.Result != nil:
			for _, content := range part.Result.Content {
				b.WriteString(content.Text)
				b.WriteString(string(content.JSON))
			}
		}
	}
	return b.String()
}

// Key devolve a chave estável do contexto no modelo (hash SHA-256 hex). Serve
// de metadado e de atalho exato; a busca no store usa o vetor (FR-SC-003).
func Key(req core.ChatRequest, model string) string {
	sum := sha256.Sum256([]byte(model + "\x00" + CanonicalText(req)))
	return hex.EncodeToString(sum[:])
}

// Cosine devolve a similaridade de cosseno entre dois vetores (0 quando
// dimensões divergem ou há vetor nulo). Nunca NaN.
func Cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		av := float64(a[i])
		bv := float64(b[i])
		dot += av * bv
		na += av * av
		nb += bv * bv
	}
	if na == 0 || nb == 0 {
		return 0
	}
	score := dot / (math.Sqrt(na) * math.Sqrt(nb))
	if math.IsNaN(score) {
		return 0
	}
	return score
}

// Valid informa se o vetor pode ser comparado (não vazio e sem NaN/Inf).
func Valid(vector []float32) bool {
	if len(vector) == 0 {
		return false
	}
	for _, v := range vector {
		f := float64(v)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return false
		}
	}
	return true
}

// Best devolve o índice da melhor entrada acima do limiar (empate resolvido
// pela ordem estável); -1 quando nenhuma qualifica.
func Best(scores []float64, minScore float64) int {
	best := -1
	bestScore := minScore
	for i, score := range scores {
		if score >= bestScore {
			best = i
			bestScore = score
		}
	}
	return best
}

// SortKeys ordena chaves de forma determinística (auxiliar de teste/host).
func SortKeys(keys []string) []string {
	sort.Strings(keys)
	return keys
}
