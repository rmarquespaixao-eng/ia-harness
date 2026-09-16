// Package httpx reúne utilitários HTTP compartilhados pelos adaptadores.
package httpx

import (
	"fmt"
	"net/http"
)

// NoCrossHostRedirect devolve uma cópia do cliente que recusa redirect para
// outro host. Os adaptadores injetam credenciais em headers customizados
// (x-api-key, Config.Headers) que o net/http não remove em redirect cross-host;
// seguir o redirect poderia vazar a credencial para um terceiro (SEC-01 da
// revisão OWASP — ADR 0007).
func NoCrossHostRedirect(base *http.Client) *http.Client {
	if base == nil {
		base = &http.Client{}
	}
	clone := *base
	previous := clone.CheckRedirect
	clone.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 0 && req.URL.Host != via[0].URL.Host {
			return fmt.Errorf("httpx: redirect cross-host bloqueado (%s → %s)", via[0].URL.Host, req.URL.Host)
		}
		if previous != nil {
			return previous(req, via)
		}
		return nil
	}
	return &clone
}
