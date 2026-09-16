package telemetry

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

const (
	// redactedPlaceholder substitui o valor de qualquer chave sensível.
	redactedPlaceholder = "[REDACTED]"
	// sensitiveKeySuffix marca payloads binários codificados, sempre sensíveis.
	sensitiveKeySuffix = "_base64"
	// truncationMarker sinaliza o corte por limite de campo.
	truncationMarker = "…"
	// defaultMaxFieldBytes é o teto padrão por campo auditado (16 KiB).
	defaultMaxFieldBytes = 16 * 1024
)

// defaultSensitiveKeys é a allowlist de chaves usada quando a config não define uma.
var defaultSensitiveKeys = []string{
	"api_key",
	"token",
	"secret",
	"password",
	"authorization",
	"cookie",
	// Mídia (feature 002): bytes inline e URL/ref de mídia nunca saem.
	"bytes",
	"reference",
}

// defaultSensitiveSet é o conjunto normalizado da allowlist padrão.
var defaultSensitiveSet = newSensitiveSet(defaultSensitiveKeys)

// Redactor redige chaves sensíveis e trunca campos antes de log, auditoria ou
// telemetria (FR-024). É imutável após a construção: seguro para uso concorrente.
type Redactor struct {
	sensitive map[string]struct{}
	maxBytes  int
}

// NewRedactor resolve os defaults da configuração: lista de chaves vazia usa a
// allowlist padrão (api_key, token, secret, password, authorization, cookie) e
// MaxFieldBytes <= 0 vale 16 KiB. O sufixo _base64 é sempre sensível.
func NewRedactor(cfg RedactionConfig) *Redactor {
	keys := cfg.SensitiveKeys
	if len(keys) == 0 {
		keys = defaultSensitiveKeys
	}
	maxBytes := cfg.MaxFieldBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxFieldBytes
	}
	return &Redactor{sensitive: newSensitiveSet(keys), maxBytes: maxBytes}
}

// newSensitiveSet normaliza as chaves em minúsculas para consulta sem diferenciar
// maiúsculas de minúsculas.
func newSensitiveSet(keys []string) map[string]struct{} {
	set := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		key = strings.ToLower(key)
		if key == "" {
			continue
		}
		set[key] = struct{}{}
	}
	return set
}

// RedactJSON redige os valores das chaves sensíveis e trunca a string
// resultante. A estrutura e os tipos do JSON são preservados; JSON inválido é
// tratado como string opaca, sem pânico.
func (r *Redactor) RedactJSON(raw json.RawMessage) (string, bool) {
	if !json.Valid(raw) {
		return r.RedactString(string(raw))
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // preserva a representação numérica original na re-serialização
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return r.RedactString(string(raw))
	}

	redigido, err := json.Marshal(r.redactValue(doc))
	if err != nil {
		return r.RedactString(string(raw))
	}
	return r.RedactString(string(redigido))
}

// RedactString aplica apenas o truncamento por campo, sem inspecionar chaves.
func (r *Redactor) RedactString(s string) (string, bool) {
	return truncate(s, r.maxFieldBytes())
}

// maxFieldBytes devolve o limite efetivo, cobrindo também o valor zero do tipo.
func (r *Redactor) maxFieldBytes() int {
	if r == nil || r.maxBytes <= 0 {
		return defaultMaxFieldBytes
	}
	return r.maxBytes
}

// redactValue percorre mapas e arrays recursivamente, trocando o valor das
// chaves sensíveis pela marca de redação.
func (r *Redactor) redactValue(v any) any {
	switch node := v.(type) {
	case map[string]any:
		for key, value := range node {
			if r.isSensitive(key) {
				node[key] = redactedPlaceholder
				continue
			}
			node[key] = r.redactValue(value)
		}
		return node
	case []any:
		for i, value := range node {
			node[i] = r.redactValue(value)
		}
		return node
	default:
		return v
	}
}

// isSensitive compara a chave sem diferenciar maiúsculas de minúsculas; o
// sufixo _base64 é sensível mesmo fora da allowlist configurada.
func (r *Redactor) isSensitive(key string) bool {
	lower := strings.ToLower(key)
	if strings.HasSuffix(lower, sensitiveKeySuffix) {
		return true
	}
	keys := defaultSensitiveSet
	if r != nil && r.sensitive != nil {
		keys = r.sensitive
	}
	_, ok := keys[lower]
	return ok
}

// truncate corta s para caber em max bytes, recuando até a fronteira de rune e
// sinalizando o corte com o marcador de truncamento. O resultado cabe em max
// bytes (o marcador ocupa 3 bytes) e no máximo max+1 runes.
func truncate(s string, max int) (string, bool) {
	if len(s) <= max {
		return s, false
	}
	limit := max - len(truncationMarker)
	if limit < 0 {
		limit = 0
	}
	for limit > 0 && !utf8.RuneStart(s[limit]) {
		limit--
	}
	return s[:limit] + truncationMarker, true
}
