// Package media valida as partes multimodais (imagem/documento) do turno contra
// a política de MIME, o teto de tamanho e as capacidades do modelo (feature 002,
// ADR 0009). É regra pura: não faz I/O nem depende do estado do Harness.
package media

import (
	"fmt"
	"strings"

	core "rmarquespaixao/ia-harness/internal/core"
)

const (
	// DefaultMaxMediaBytes é o teto por mídia quando o perfil não define outro
	// (5 MiB — research R3).
	DefaultMaxMediaBytes int64 = 5 << 20
	// DefaultMaxPerTurn é o número máximo de mídias por turno (research R3).
	DefaultMaxPerTurn = 10
)

// imageMIME é a allowlist de MIME de imagem (research R4).
var imageMIME = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
	"image/gif":  true,
}

// documentMIME é a allowlist de MIME de documento.
var documentMIME = map[string]bool{
	"application/pdf": true,
	"text/plain":      true,
	"text/csv":        true,
}

// textualMIME marca documentos que viajam como texto (não exigem suporte a
// documento binário no modelo).
var textualMIME = map[string]bool{
	"text/plain": true,
	"text/csv":   true,
}

// IsTextual informa se o MIME é textual (inline como texto).
func IsTextual(mime string) bool { return textualMIME[normalizeMIME(mime)] }

// FromMessages extrai as partes de mídia do histórico (imagem/documento).
func FromMessages(messages []core.Message) []core.Part {
	var out []core.Part
	for _, message := range messages {
		for _, part := range message.Parts {
			if part.Media != nil && (part.Kind == core.PartImage || part.Kind == core.PartDocument) {
				out = append(out, part)
			}
		}
	}
	return out
}

// Required informa quais capacidades o conjunto de mídias exige: visão para
// imagens e documento para binário não textual (PDF).
func Required(parts []core.Part) (vision, documents bool) {
	for _, part := range parts {
		if part.Media == nil {
			continue
		}
		switch part.Kind {
		case core.PartImage:
			vision = true
		case core.PartDocument:
			if !IsTextual(part.Media.MIME) {
				documents = true
			}
		}
	}
	return vision, documents
}

// Supports informa se as capacidades do modelo cobrem as mídias do turno
// (usado na elegibilidade de fallback — FR-MM-011).
func Supports(caps core.Capabilities, parts []core.Part) bool {
	vision, documents := Required(parts)
	if vision && !caps.Vision {
		return false
	}
	if documents && !caps.Documents {
		return false
	}
	return true
}

// Validate confere MIME (allowlist), tamanho (teto do perfil com default),
// fonte única (Bytes xor Reference) e capacidade do modelo; erro nomeado antes
// de qualquer I/O (FR-MM-003/004/005).
func Validate(caps core.Capabilities, parts []core.Part) error {
	maxBytes := caps.MaxMediaBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxMediaBytes
	}
	count := 0
	for _, part := range parts {
		if part.Media == nil {
			continue
		}
		if part.Kind != core.PartImage && part.Kind != core.PartDocument {
			continue
		}
		count++
		m := part.Media
		if !allowedMIME(part.Kind, m.MIME) {
			return configErr("media/mime-nao-suportado",
				fmt.Sprintf("MIME %q não suportado para %s", m.MIME, part.Kind))
		}
		if m.SizeBytes <= 0 {
			return configErr("media/tamanho-invalido", "size_bytes da mídia deve ser > 0")
		}
		if m.SizeBytes > maxBytes {
			return configErr("media/grande",
				fmt.Sprintf("mídia de %d bytes excede o teto de %d", m.SizeBytes, maxBytes))
		}
		hasBytes := len(m.Bytes) > 0
		hasRef := strings.TrimSpace(m.Reference) != ""
		if hasBytes == hasRef {
			return configErr("media/fonte-invalida", "informe exatamente uma fonte: bytes ou reference")
		}
		switch part.Kind {
		case core.PartImage:
			if !caps.Vision {
				return configErr("media/visao-nao-suportada",
					fmt.Sprintf("modelo não declara visão; imagem %q recusada", m.Name))
			}
		case core.PartDocument:
			if !IsTextual(m.MIME) && !caps.Documents {
				return configErr("media/documento-nao-suportado",
					fmt.Sprintf("modelo não declara suporte a documento; %q recusado", m.Name))
			}
		}
	}
	if count > DefaultMaxPerTurn {
		return configErr("media/excesso",
			fmt.Sprintf("%d mídias no turno excedem o limite de %d", count, DefaultMaxPerTurn))
	}
	return nil
}

// allowedMIME informa se o MIME é permitido para o tipo de parte.
func allowedMIME(kind core.PartKind, mime string) bool {
	mime = normalizeMIME(mime)
	switch kind {
	case core.PartImage:
		return imageMIME[mime]
	case core.PartDocument:
		return documentMIME[mime]
	default:
		return false
	}
}

// normalizeMIME remove parâmetros (ex.: "; charset=utf-8") e caixa.
func normalizeMIME(mime string) string {
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = mime[:i]
	}
	return strings.ToLower(strings.TrimSpace(mime))
}

// configErr monta o erro nomeado de validação de mídia.
func configErr(code, message string) error {
	return &core.ConfigError{Code: code, Message: message}
}
