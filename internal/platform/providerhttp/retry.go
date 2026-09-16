// Package providerhttp concentra o retry classificado das chamadas HTTP dos
// adaptadores de provedor (feature 007): repete apenas erros transitórios
// (429, timeout, 5xx e falha de transporte) e nunca antes de consumir o stream.
package providerhttp

import (
	"context"
	"errors"
	"net/http"

	"github.com/rmarquespaixao-eng/ia-harness/harness"
	"github.com/rmarquespaixao-eng/ia-harness/internal/platform/retry"
)

// codesRetryable são os códigos estáveis de *harness.ProviderError tratados
// como transitórios (espelham os prefixos provider_* dos adaptadores).
var codesRetryable = map[string]bool{
	"provider_rate_limited": true,
	"provider_timeout":      true,
	"provider_unavailable":  true,
	"provider_transport":    true,
}

// Retryable informa se o erro é transitório e pode ser repetido; cancelamento
// de contexto nunca é repetido.
func Retryable(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var provErr *harness.ProviderError
	if !errors.As(err, &provErr) {
		return false
	}
	return codesRetryable[provErr.Code]
}

// Do executa attempt sob política de retry; attempt monta a requisição e
// classifica o status (fechando o corpo em erro). O corpo do request precisa
// ser reconstruído a cada tentativa pelo próprio attempt. Só erros retryable
// são repetidos; a última falha é devolvida preservando a causa.
func Do(ctx context.Context, policy retry.Policy, attempt func(ctx context.Context) (*http.Response, error)) (*http.Response, error) {
	var resp *http.Response
	err := retry.DoIf(ctx, policy, retry.SleepCtx, func(ctx context.Context) error {
		out, err := attempt(ctx)
		if err != nil {
			return err
		}
		resp = out
		return nil
	}, Retryable)
	return resp, err
}
