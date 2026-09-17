package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bucketT0 é o instante determinístico dos fixtures do bucket (o tempo entra
// por parâmetro, sem time.Now).
var bucketT0 = time.Date(2026, time.September, 17, 12, 0, 0, 0, time.UTC)

// bucketTolerance é a folga aceita nas comparações de espera, absorvendo o
// arredondamento de time.Duration sobre a taxa fracionária.
const bucketTolerance = time.Millisecond

// bucketAssertWait compara a espera devolvida por Reserve com a esperada,
// dentro da tolerância.
func bucketAssertWait(t *testing.T, want, got time.Duration) {
	t.Helper()
	assert.InDelta(t, float64(want), float64(got), float64(bucketTolerance), "espera fora da tolerância")
}

// TestBucket_DesligadoNaoLimita cobre requestsPerMinute ≤ 0: bucket desligado
// não limita e Reserve devolve sempre zero.
func TestBucket_DesligadoNaoLimita(t *testing.T) {
	cases := []struct {
		name string
		rpm  int
	}{
		{name: "zero", rpm: 0},
		{name: "negativo", rpm: -1},
		{name: "muito negativo", rpm: -60},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			b := New(tc.rpm, 10)

			// Act
			primeira := b.Reserve(bucketT0)
			segunda := b.Reserve(bucketT0.Add(time.Hour))

			// Assert
			assert.False(t, b.Limited())
			assert.Equal(t, 0, b.PerMinute())
			assert.Equal(t, time.Duration(0), primeira)
			assert.Equal(t, time.Duration(0), segunda)
		})
	}
}

// TestBucket_LimitedEPerMinute cobre a coerência dos acessores de configuração.
func TestBucket_LimitedEPerMinute(t *testing.T) {
	cases := []struct {
		name          string
		rpm           int
		burst         int
		wantLimited   bool
		wantPerMinute int
	}{
		{name: "desligado", rpm: 0, burst: 10, wantLimited: false, wantPerMinute: 0},
		{name: "ligado 60", rpm: 60, burst: 1, wantLimited: true, wantPerMinute: 60},
		{name: "ligado 120", rpm: 120, burst: 5, wantLimited: true, wantPerMinute: 120},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange & Act
			b := New(tc.rpm, tc.burst)

			// Assert
			assert.Equal(t, tc.wantLimited, b.Limited())
			assert.Equal(t, tc.wantPerMinute, b.PerMinute())
		})
	}
}

// TestBucket_ReservasConcorrentesAumentamEspera cobre um bucket 60/min com
// burst 1: a primeira reserva consome o único token e as seguintes no mesmo
// instante recebem esperas crescentes (~1s, ~2s, ...).
func TestBucket_ReservasConcorrentesAumentamEspera(t *testing.T) {
	// Arrange
	b := New(60, 1)

	// Act
	primeira := b.Reserve(bucketT0)
	segunda := b.Reserve(bucketT0)
	terceira := b.Reserve(bucketT0)

	// Assert
	bucketAssertWait(t, 0, primeira)
	bucketAssertWait(t, time.Second, segunda)
	require.Greater(t, segunda, time.Duration(0))
	bucketAssertWait(t, 2*time.Second, terceira)
	require.Greater(t, terceira, segunda)
}

// TestBucket_ReposicaoAposIntervalo cobre a reposição de tokens: após uma
// reserva, avançar o relógio em 1s libera a próxima reserva sem espera.
func TestBucket_ReposicaoAposIntervalo(t *testing.T) {
	// Arrange
	b := New(60, 1)
	require.Equal(t, time.Duration(0), b.Reserve(bucketT0))

	// Act
	wait := b.Reserve(bucketT0.Add(time.Second))

	// Assert
	bucketAssertWait(t, 0, wait)
}

// TestBucket_BurstPermiteRajada cobre burst > 1: as primeiras burst reservas
// no mesmo instante têm espera zero e a seguinte passa a esperar ~1s.
func TestBucket_BurstPermiteRajada(t *testing.T) {
	cases := []struct {
		name  string
		burst int
	}{
		{name: "burst 1", burst: 1},
		{name: "burst 3", burst: 3},
		{name: "burst 5", burst: 5},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			b := New(60, tc.burst)

			// Act & Assert
			for i := 0; i < tc.burst; i++ {
				wait := b.Reserve(bucketT0)
				assert.Equalf(t, time.Duration(0), wait, "reserva %d dentro do burst", i+1)
			}
			wait := b.Reserve(bucketT0)
			require.Greater(t, wait, time.Duration(0))
			bucketAssertWait(t, time.Second, wait)
		})
	}
}

// TestBucket_BurstNaoPositivoCaiParaUm cobre burst ≤ 0: a capacidade cai para
// 1 e a segunda reserva imediata passa a esperar ~1s.
func TestBucket_BurstNaoPositivoCaiParaUm(t *testing.T) {
	cases := []struct {
		name  string
		burst int
	}{
		{name: "zero", burst: 0},
		{name: "negativo", burst: -3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			b := New(60, tc.burst)
			require.Equal(t, time.Duration(0), b.Reserve(bucketT0))

			// Act
			wait := b.Reserve(bucketT0)

			// Assert
			require.Greater(t, wait, time.Duration(0))
			bucketAssertWait(t, time.Second, wait)
		})
	}
}

// TestBucket_RelogioParaTrasNaoGeraEsperaNegativa cobre um now anterior ao
// último: sem reposição retroativa, a espera continua positiva e não há pânico.
func TestBucket_RelogioParaTrasNaoGeraEsperaNegativa(t *testing.T) {
	// Arrange
	b := New(60, 1)
	require.Equal(t, time.Duration(0), b.Reserve(bucketT0))

	// Act
	wait := b.Reserve(bucketT0.Add(-time.Hour))

	// Assert
	require.Greater(t, wait, time.Duration(0))
	bucketAssertWait(t, time.Second, wait)

	// Act & Assert: chamadas ainda mais antigas continuam sem pânico nem espera negativa.
	assert.NotPanics(t, func() {
		assert.GreaterOrEqual(t, b.Reserve(bucketT0.Add(-2*time.Hour)), time.Duration(0))
	})
}
