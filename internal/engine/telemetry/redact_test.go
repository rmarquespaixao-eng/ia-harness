package telemetry

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedactJSON_RedigeChavesSensiveisEmObjetoAninhado(t *testing.T) {
	// Arrange
	red := NewRedactor(RedactionConfig{})
	raw := json.RawMessage(`{
		"api_key": "isca-chave",
		"API_KEY": "isca-chave-maiuscula",
		"Authorization": "Bearer isca-auth",
		"document_base64": "isca-base64",
		"cartao": {"token": "isca-token", "last_digits": "1234"},
		"month": 8,
		"ativo": true
	}`)

	// Act
	saida, truncated := red.RedactJSON(raw)

	// Assert
	require.False(t, truncated)
	assert.NotContains(t, saida, "isca")
	assert.JSONEq(t, `{
		"api_key": "[REDACTED]",
		"API_KEY": "[REDACTED]",
		"Authorization": "[REDACTED]",
		"document_base64": "[REDACTED]",
		"cartao": {"token": "[REDACTED]", "last_digits": "1234"},
		"month": 8,
		"ativo": true
	}`, saida)
}

func TestRedactJSON_RedigeDentroDeArrays(t *testing.T) {
	// Arrange
	red := NewRedactor(RedactionConfig{})
	raw := json.RawMessage(`[
		{"token": "isca-token", "nome": "ana"},
		{"secret": "isca-secret", "itens": [{"cookie": "isca-cookie"}]}
	]`)

	// Act
	saida, truncated := red.RedactJSON(raw)

	// Assert
	require.False(t, truncated)
	assert.NotContains(t, saida, "isca")
	assert.JSONEq(t, `[
		{"token": "[REDACTED]", "nome": "ana"},
		{"secret": "[REDACTED]", "itens": [{"cookie": "[REDACTED]"}]}
	]`, saida)
}

func TestRedactJSON_PreservaCamposNaoSensiveis(t *testing.T) {
	// Arrange
	red := NewRedactor(RedactionConfig{})
	raw := json.RawMessage(`{
		"month": 8,
		"amount_cents": 12345,
		"flag": true,
		"vazio": null,
		"texto": "olá",
		"nested": {"ok": "valor"}
	}`)

	// Act
	saida, truncated := red.RedactJSON(raw)

	// Assert
	require.False(t, truncated)
	assert.JSONEq(t, `{
		"month": 8,
		"amount_cents": 12345,
		"flag": true,
		"vazio": null,
		"texto": "olá",
		"nested": {"ok": "valor"}
	}`, saida)
}

func TestRedactJSON_ChavesCustomizadasSubstituemDefaults(t *testing.T) {
	// Arrange
	red := NewRedactor(RedactionConfig{SensitiveKeys: []string{"X_CUSTOM"}})
	raw := json.RawMessage(`{
		"x_custom": "isca-custom",
		"token": "dado-preservado",
		"payload_base64": "isca-base64"
	}`)

	// Act
	saida, truncated := red.RedactJSON(raw)

	// Assert
	require.False(t, truncated)
	assert.NotContains(t, saida, "isca")
	assert.JSONEq(t, `{
		"x_custom": "[REDACTED]",
		"token": "dado-preservado",
		"payload_base64": "[REDACTED]"
	}`, saida)
}

func TestRedactJSON_TruncaSaidaAcimaDoLimite(t *testing.T) {
	// Arrange
	const limite = 32
	red := NewRedactor(RedactionConfig{MaxFieldBytes: limite})
	raw := json.RawMessage(`{"api_key": "` + strings.Repeat("a", 200) + `", "dados": "` + strings.Repeat("b", 200) + `"}`)

	// Act
	saida, truncated := red.RedactJSON(raw)

	// Assert
	require.True(t, truncated)
	assert.LessOrEqual(t, utf8.RuneCountInString(saida), limite+1)
	assert.True(t, strings.HasSuffix(saida, truncationMarker))
	assert.NotContains(t, saida, "[REDACTED]"+strings.Repeat("a", 10))
}

func TestRedactJSON_JSONInvalidoNaoEntraEmPanico(t *testing.T) {
	// Arrange
	red := NewRedactor(RedactionConfig{})
	entrada := "{chave: sem aspas, token: isca}"

	// Act
	saida, truncated := red.RedactJSON(json.RawMessage(entrada))

	// Assert
	require.False(t, truncated)
	assert.Equal(t, entrada, saida)
}

func TestRedactJSON_StringOpacaRespeitaOLimite(t *testing.T) {
	// Arrange
	red := NewRedactor(RedactionConfig{MaxFieldBytes: 10})
	entrada := "isto não é json e passa de dez bytes"

	// Act
	saida, truncated := red.RedactJSON(json.RawMessage(entrada))

	// Assert
	require.True(t, truncated)
	assert.True(t, strings.HasPrefix(entrada, strings.TrimSuffix(saida, truncationMarker)))
	assert.LessOrEqual(t, utf8.RuneCountInString(saida), 11)
}

func TestRedactString_TruncaAcimaDoLimite(t *testing.T) {
	// Arrange
	const limite = 16
	red := NewRedactor(RedactionConfig{MaxFieldBytes: limite})
	longa := strings.Repeat("a", 100)

	// Act
	saida, truncated := red.RedactString(longa)

	// Assert
	require.True(t, truncated)
	assert.True(t, strings.HasSuffix(saida, truncationMarker))
	assert.True(t, strings.HasPrefix(longa, strings.TrimSuffix(saida, truncationMarker)))
	assert.LessOrEqual(t, len(saida), limite)
	assert.LessOrEqual(t, utf8.RuneCountInString(saida), limite+1)
}

func TestRedactString_NoLimiteNaoTrunca(t *testing.T) {
	// Arrange
	const limite = 8
	red := NewRedactor(RedactionConfig{MaxFieldBytes: limite})
	exata := "12345678"

	// Act
	saida, truncated := red.RedactString(exata)

	// Assert
	require.False(t, truncated)
	assert.Equal(t, exata, saida)
}

func TestRedactString_NaoCortaRuneAoMeio(t *testing.T) {
	// Arrange — cada rune ocupa 2 bytes; o limite ímpar forçaria corte no meio.
	red := NewRedactor(RedactionConfig{MaxFieldBytes: 11})
	entrada := strings.Repeat("á", 50)

	// Act
	saida, truncated := red.RedactString(entrada)

	// Assert
	require.True(t, truncated)
	assert.True(t, utf8.ValidString(saida))
	assert.LessOrEqual(t, len(saida), 12)
	assert.True(t, strings.HasPrefix(entrada, strings.TrimSuffix(saida, truncationMarker)))
}

func TestNewRedactor_UsaChavesSensiveisPadrao(t *testing.T) {
	// Arrange
	red := NewRedactor(RedactionConfig{})
	raw := json.RawMessage(`{
		"api_key": "isca",
		"token": "isca",
		"secret": "isca",
		"password": "isca",
		"authorization": "isca",
		"cookie": "isca"
	}`)

	// Act
	saida, truncated := red.RedactJSON(raw)

	// Assert
	require.False(t, truncated)
	assert.NotContains(t, saida, "isca")
	assert.JSONEq(t, `{
		"api_key": "[REDACTED]",
		"token": "[REDACTED]",
		"secret": "[REDACTED]",
		"password": "[REDACTED]",
		"authorization": "[REDACTED]",
		"cookie": "[REDACTED]"
	}`, saida)
}

func TestNewRedactor_UsaLimitePadrao(t *testing.T) {
	// Arrange
	red := NewRedactor(RedactionConfig{MaxFieldBytes: -1})

	// Act
	noLimite, truncadoNoLimite := red.RedactString(strings.Repeat("a", defaultMaxFieldBytes))
	acima, truncadoAcima := red.RedactString(strings.Repeat("a", defaultMaxFieldBytes+1))

	// Assert
	require.False(t, truncadoNoLimite)
	assert.Len(t, noLimite, defaultMaxFieldBytes)
	require.True(t, truncadoAcima)
	assert.LessOrEqual(t, len(acima), defaultMaxFieldBytes)
	assert.LessOrEqual(t, utf8.RuneCountInString(acima), defaultMaxFieldBytes+1)
}
