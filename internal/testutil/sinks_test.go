package testutil_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/harness"
	"github.com/rmarquespaixao-eng/ia-harness/internal/testutil"
)

func TestFakeAuditSink_AcumulaEventos(t *testing.T) {
	// Arrange
	sink := &testutil.FakeAuditSink{}

	// Act
	errPrimeiro := sink.Emit(context.Background(), harness.AuditEvent{Id: "ev-1", Kind: "model_call"})
	errSegundo := sink.Emit(context.Background(), harness.AuditEvent{Id: "ev-2", Kind: "tool_call"})

	// Assert
	require.NoError(t, errPrimeiro)
	require.NoError(t, errSegundo)
	require.Len(t, sink.Events, 2)
	assert.Equal(t, "ev-1", sink.Events[0].Id)
	assert.Equal(t, "model_call", string(sink.Events[0].Kind))
	assert.Equal(t, "ev-2", sink.Events[1].Id)
}

func TestFakeAuditSink_NaoRegistraQuandoHaErro(t *testing.T) {
	// Arrange
	sentinela := errors.New("trilha indisponível")
	sink := &testutil.FakeAuditSink{Err: sentinela}

	// Act
	err := sink.Emit(context.Background(), harness.AuditEvent{Id: "ev-1"})

	// Assert
	require.ErrorIs(t, err, sentinela)
	assert.Empty(t, sink.Events)
}
