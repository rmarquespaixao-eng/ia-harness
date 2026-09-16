package testutil_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/internal/testutil"
)

func TestFakeCredentialProvider_ResolveERegistraRefs(t *testing.T) {
	// Arrange
	provider := &testutil.FakeCredentialProvider{Values: map[string]string{"env:API_KEY": "segredo"}}

	// Act
	valor, err := provider.Resolve(context.Background(), "env:API_KEY")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "segredo", valor)
	assert.Equal(t, []string{"env:API_KEY"}, provider.Resolved)
}

func TestFakeCredentialProvider_RefDesconhecida(t *testing.T) {
	// Arrange
	provider := &testutil.FakeCredentialProvider{Values: map[string]string{}}

	// Act
	_, err := provider.Resolve(context.Background(), "file:/run/secrets/key")

	// Assert
	require.ErrorIs(t, err, testutil.ErrUnknownCredential)
	assert.ErrorContains(t, err, `"file:/run/secrets/key"`)
	assert.Equal(t, []string{"file:/run/secrets/key"}, provider.Resolved)
}

func TestFakeCredentialProvider_ErroInjetado(t *testing.T) {
	// Arrange
	sentinela := errors.New("cofre fora do ar")
	provider := &testutil.FakeCredentialProvider{Values: map[string]string{"env:X": "v"}, Err: sentinela}

	// Act
	_, err := provider.Resolve(context.Background(), "env:X")

	// Assert
	require.ErrorIs(t, err, sentinela)
	assert.Equal(t, []string{"env:X"}, provider.Resolved)
}
