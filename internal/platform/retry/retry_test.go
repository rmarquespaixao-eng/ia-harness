package retry_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rmarquespaixao/ia-harness/internal/platform/retry"
)

// sleeperFake registra os delays pedidos sem dormir de verdade.
type sleeperFake struct {
	delays []time.Duration
	err    error
}

func (f *sleeperFake) sleep(_ context.Context, d time.Duration) error {
	f.delays = append(f.delays, d)
	return f.err
}

func TestDo_SucessoNaPrimeiraTentativa(t *testing.T) {
	// Arrange
	chamadas := 0
	op := func(context.Context) error {
		chamadas++
		return nil
	}
	fake := &sleeperFake{}

	// Act
	err := retry.Do(context.Background(), retry.Policy{MaxAttempts: 3}, fake.sleep, op)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, chamadas)
	assert.Empty(t, fake.delays)
}

func TestDo_SucessoNaTerceiraTentativaRespeitaBackoff(t *testing.T) {
	// Arrange
	sentinela := errors.New("falha transitória")
	chamadas := 0
	op := func(context.Context) error {
		chamadas++
		if chamadas < 3 {
			return sentinela
		}
		return nil
	}
	fake := &sleeperFake{}
	policy := retry.Policy{MaxAttempts: 3, BaseDelay: 100 * time.Millisecond, MaxDelay: time.Second}

	// Act
	err := retry.Do(context.Background(), policy, fake.sleep, op)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 3, chamadas)
	assert.Equal(t, []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}, fake.delays)
}

func TestDo_EsgotaTentativasEMantemCausa(t *testing.T) {
	// Arrange
	sentinela := errors.New("falha permanente")
	chamadas := 0
	op := func(context.Context) error {
		chamadas++
		return fmt.Errorf("op: %w", sentinela)
	}
	fake := &sleeperFake{}
	policy := retry.Policy{MaxAttempts: 3, BaseDelay: 100 * time.Millisecond, MaxDelay: time.Second}

	// Act
	err := retry.Do(context.Background(), policy, fake.sleep, op)

	// Assert
	require.Error(t, err)
	assert.Equal(t, 3, chamadas)
	assert.Equal(t, []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}, fake.delays)
	assert.ErrorIs(t, err, sentinela)
	assert.ErrorContains(t, err, "retry: 3 tentativas")
}

func TestDo_EsperaCancelada(t *testing.T) {
	// Arrange
	chamadas := 0
	op := func(context.Context) error {
		chamadas++
		return errors.New("falha")
	}
	fake := &sleeperFake{err: context.Canceled}
	policy := retry.Policy{MaxAttempts: 3}

	// Act
	err := retry.Do(context.Background(), policy, fake.sleep, op)

	// Assert
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.ErrorContains(t, err, "retry: espera cancelada")
	assert.Equal(t, 1, chamadas)
	assert.Len(t, fake.delays, 1)
}

func TestDo_LimitaEsperaAoMaxDelay(t *testing.T) {
	// Arrange
	op := func(context.Context) error {
		return errors.New("falha")
	}
	fake := &sleeperFake{}
	policy := retry.Policy{MaxAttempts: 4, BaseDelay: 100 * time.Millisecond, MaxDelay: 150 * time.Millisecond}

	// Act
	err := retry.Do(context.Background(), policy, fake.sleep, op)

	// Assert
	require.Error(t, err)
	assert.Equal(t, []time.Duration{100 * time.Millisecond, 150 * time.Millisecond, 150 * time.Millisecond}, fake.delays)
}

func TestDo_AplicaValoresPadraoDaPolitica(t *testing.T) {
	// Arrange
	casos := []struct {
		nome     string
		policy   retry.Policy
		esperado []time.Duration
	}{
		{"max attempts zero vale 1", retry.Policy{MaxAttempts: 0}, nil},
		{"base delay zero vale 100ms", retry.Policy{MaxAttempts: 2}, []time.Duration{100 * time.Millisecond}},
		{"max delay menor que base vale base", retry.Policy{MaxAttempts: 3, BaseDelay: 100 * time.Millisecond, MaxDelay: 50 * time.Millisecond}, []time.Duration{100 * time.Millisecond, 100 * time.Millisecond}},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			// Arrange
			fake := &sleeperFake{}
			op := func(context.Context) error {
				return errors.New("falha")
			}

			// Act
			err := retry.Do(context.Background(), caso.policy, fake.sleep, op)

			// Assert
			require.Error(t, err)
			assert.Equal(t, caso.esperado, fake.delays)
		})
	}
}

func TestDo_JitterMantemEsperaDentroDeVintePorCento(t *testing.T) {
	// Arrange
	op := func(context.Context) error {
		return errors.New("falha")
	}
	fake := &sleeperFake{}
	policy := retry.Policy{MaxAttempts: 3, BaseDelay: 100 * time.Millisecond, MaxDelay: time.Second, Jitter: true}
	esperadas := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}

	// Act
	err := retry.Do(context.Background(), policy, fake.sleep, op)

	// Assert
	require.Error(t, err)
	require.Len(t, fake.delays, len(esperadas))
	for i, delay := range fake.delays {
		assert.GreaterOrEqual(t, delay, esperadas[i]*80/100, "delay %d abaixo do limite", i)
		assert.LessOrEqual(t, delay, esperadas[i]*120/100, "delay %d acima do limite", i)
	}
}
