package clock_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"rmarquespaixao/ia-harness/internal/platform/clock"
)

func TestSystem_Now_ProximoDoRelogioReal(t *testing.T) {
	// Arrange
	relogio := clock.System{}

	// Act
	obtido := relogio.Now()

	// Assert
	assert.WithinDuration(t, time.Now(), obtido, time.Second)
}

func TestSystem_Now_NaoDecresceEntreChamadas(t *testing.T) {
	// Arrange
	relogio := clock.System{}

	// Act
	primeiro := relogio.Now()
	segundo := relogio.Now()

	// Assert
	assert.False(t, segundo.Before(primeiro), "segunda leitura %v anterior à primeira %v", segundo, primeiro)
}
