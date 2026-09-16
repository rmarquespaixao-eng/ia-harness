package harness_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/harness"
	"github.com/rmarquespaixao-eng/ia-harness/internal/testutil"
)

// redeTocada é acionada por qualquer RoundTrip que chegue ao transporte HTTP
// padrão durante os testes do pacote — o gate FR-030 proíbe rede real.
var redeTocada atomic.Bool

// transportBloqueado substitui http.DefaultTransport: nenhuma requisição sai,
// a violação é registrada na flag e um erro controlado é devolvido.
type transportBloqueado struct{}

func (transportBloqueado) RoundTrip(req *http.Request) (*http.Response, error) {
	redeTocada.Store(true)
	return nil, fmt.Errorf("teste sem rede real: %s %s", req.Method, req.URL.String())
}

// TestMain instala o guard de rede para todo o binário de teste do pacote e
// restaura o transporte original ao final (FR-030 / quickstart §gate local).
// É o único TestMain do pacote.
func TestMain(m *testing.M) {
	original := http.DefaultTransport
	http.DefaultTransport = transportBloqueado{}
	code := m.Run()
	http.DefaultTransport = original
	os.Exit(code)
}

// TestRun_ComFakes_NaoTocaRedeReal roda um turno completo (modelo, tool e
// persistência) com fakes e prova que o transporte HTTP padrão não foi usado.
func TestRun_ComFakes_NaoTocaRedeReal(t *testing.T) {
	// Arrange
	handler := &turnHandler{}
	env := newTurnEnv(t, []testutil.ProviderStep{
		{
			Text:      "Consultando.",
			ToolCalls: []harness.ToolCall{toolCall("call-1", "consultar_saldo", `{"q":"saldo"}`)},
		},
		{Text: "Saldo ok."},
	})

	// Act
	result, err := env.h.Run(context.Background(), pergunta("qual o saldo?"), handler)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, harness.StopCompleted, result.StopReason)
	assert.False(t, redeTocada.Load(), "nenhum teste do pacote pode usar a rede real")

	// Confirma que o turno percorreu os caminhos que, sem fakes, seriam HTTP.
	require.Len(t, env.provider.Requests, 2)
	require.Len(t, env.tools.Calls, 1)
}
