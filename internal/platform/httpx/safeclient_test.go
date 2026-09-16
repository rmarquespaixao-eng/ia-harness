package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmarquespaixao/ia-harness/internal/platform/httpx"
)

// TestNoCrossHostRedirectBloqueiaHostDiferente cobre a regressão do SEC-01
// (ADR 0007): header customizado de credencial não pode seguir redirect
// cross-host.
func TestNoCrossHostRedirectBloqueiaHostDiferente(t *testing.T) {
	// Arrange
	destino := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Fail(t, "destino cross-host não deveria receber a requisição")
	}))
	defer destino.Close()
	origem := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destino.URL, http.StatusFound)
	}))
	defer origem.Close()
	client := httpx.NoCrossHostRedirect(&http.Client{})

	// Act
	resp, err := client.Get(origem.URL)

	// Assert
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redirect cross-host bloqueado")
}

// TestNoCrossHostRedirectPermiteMesmoHost garante que o guard não quebra
// redirect legítimo dentro do mesmo host.
func TestNoCrossHostRedirectPermiteMesmoHost(t *testing.T) {
	// Arrange
	mux := http.NewServeMux()
	mux.HandleFunc("/final", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/inicio", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final", http.StatusFound)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	client := httpx.NoCrossHostRedirect(&http.Client{})

	// Act
	resp, err := client.Get(server.URL + "/inicio")

	// Assert
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestNoCrossHostRedirectPreservaCheckRedirectDoHost respeita o callback
// preexistente do cliente injetado.
func TestNoCrossHostRedirectPreservaCheckRedirectDoHost(t *testing.T) {
	// Arrange
	chamado := false
	base := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		chamado = true
		return http.ErrUseLastResponse
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/outro", http.StatusFound)
	}))
	defer server.Close()

	// Act
	resp, err := httpx.NoCrossHostRedirect(base).Get(server.URL)

	// Assert
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.True(t, chamado, "CheckRedirect do host deve ser consultado")
	assert.Equal(t, http.StatusFound, resp.StatusCode)
}
