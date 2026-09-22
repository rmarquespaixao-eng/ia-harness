//go:build unix

package mcpclient_test

import (
	"context"
	"errors"
	"fmt"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/adapters/mcpclient"
)

// alive verifica se o processo pid ainda existe (sinal 0 — não entrega sinal
// algum, só checa existência). Um zumbi ainda "existe" até ser reaped.
func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return !errors.Is(err, syscall.ESRCH)
}

// TestStdio_CloseNoOrphan cobre FR-STD-007 (CU-STD-3): após Close, nem o filho
// nem o neto (spawn-child) continuam vivos, verificado por poll com deadline —
// nunca por um time.Sleep fixo como asserção (T2320).
func TestStdio_CloseNoOrphan(t *testing.T) {
	client := mcpclient.New(stdioConfig(t, "spawn-child"), mcpclient.Deps{})

	tools, err := client.List(context.Background())
	require.NoError(t, err)
	require.Len(t, tools, 1)

	result, err := client.Call(context.Background(), "pids", nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, result.Content)

	var childPID, grandchildPID int
	_, err = fmt.Sscanf(result.Content[0].Text, "%d %d", &childPID, &grandchildPID)
	require.NoError(t, err)
	require.Greater(t, childPID, 0)
	require.Greater(t, grandchildPID, 0)
	require.True(t, alive(childPID), "filho deve estar vivo antes do Close")
	require.True(t, alive(grandchildPID), "neto deve estar vivo antes do Close")

	require.NoError(t, client.Close())

	require.Eventually(t, func() bool {
		return !alive(childPID)
	}, 7*time.Second, 20*time.Millisecond, "filho não deve sobreviver ao Close")

	require.Eventually(t, func() bool {
		return !alive(grandchildPID)
	}, 7*time.Second, 20*time.Millisecond, "neto não deve sobreviver ao Close (sem órfão)")
}
