package testutil

import (
	"context"
	"errors"
	"fmt"

	"github.com/rmarquespaixao-eng/ia-harness/harness"
)

// ErrUnknownCredential indica referência de credencial ausente no mapa do fake.
var ErrUnknownCredential = errors.New("testutil: credencial não registrada")

// FakeCredentialProvider resolve referências de credencial a partir de um mapa fixo.
type FakeCredentialProvider struct {
	Values   map[string]string
	Err      error
	Resolved []string
}

// Resolve registra a tentativa e devolve o valor; ref desconhecida → ErrUnknownCredential.
func (c *FakeCredentialProvider) Resolve(_ context.Context, ref string) (string, error) {
	c.Resolved = append(c.Resolved, ref)
	if c.Err != nil {
		return "", c.Err
	}
	value, ok := c.Values[ref]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownCredential, ref)
	}
	return value, nil
}

var _ harness.CredentialProvider = (*FakeCredentialProvider)(nil)
