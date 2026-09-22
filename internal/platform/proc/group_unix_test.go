//go:build unix

package proc_test

import (
	"bufio"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/internal/platform/proc"
)

// esperaAte faz poll em cond a cada 10ms até que retorne true ou o prazo
// expire; no expirar do prazo, falha o teste com msgEFormatar.
func esperaAte(t *testing.T, prazo time.Duration, cond func() bool, msgEFormatar string, args ...interface{}) {
	t.Helper()

	deadline := time.Now().Add(prazo)
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf(msgEFormatar, args...)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// processoVivo verifica se o PID ainda existe via signal 0.
func processoVivo(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// processoMorto verifica se o PID não existe mais (ESRCH).
func processoMorto(pid int) bool {
	err := syscall.Kill(pid, 0)
	return errors.Is(err, syscall.ESRCH)
}

func TestGroupAndKillGroup(t *testing.T) {
	t.Run("kills_child_and_grandchild", func(t *testing.T) {
		// Arrange — filho que cria um neto (sleep), informa o PID dele e espera.
		child := exec.Command("sh", "-c", "sleep 60 & echo $!; wait")
		child.SysProcAttr = proc.Group()
		stdout, err := child.StdoutPipe()
		require.NoError(t, err)
		require.NoError(t, child.Start())
		pid := child.Process.Pid

		linha, err := bufio.NewReader(stdout).ReadString('\n')
		require.NoError(t, err, "filho não informou o PID do neto")
		netoPid, err := strconv.Atoi(strings.TrimSpace(linha))
		require.NoError(t, err)
		require.True(t, processoVivo(netoPid), "neto deveria estar vivo antes do KillGroup")

		// Act
		err = proc.KillGroup(pid)

		// Assert — ESRCH é ignorado; o grupo deve estar morto.
		require.NoError(t, err)

		// Colhe o filho (senão fica zumbi e Kill(pid,0) continua "vivo").
		_ = child.Wait()

		// Verifica que o filho morreu.
		esperaAte(t, 5*time.Second, func() bool {
			return processoMorto(pid)
		}, "processo filho não morreu a tempo (pid=%d)", pid)

		// Verifica que o neto (sleep 60 em background) também morreu.
		// O neto é órfão adotado e colhido pelo init; o poll aguarda essa colheita.
		esperaAte(t, 5*time.Second, func() bool {
			return processoMorto(netoPid)
		}, "processo neto não morreu a tempo (pid=%d)", netoPid)
	})

	t.Run("esrch_ignored", func(t *testing.T) {
		// Arrange — PID inexistente.
		err := proc.KillGroup(999999)
		assert.NoError(t, err, "ESRCH deve ser ignorado")
	})

	t.Run("group_attr_sets_pgid", func(t *testing.T) {
		attr := proc.Group()
		require.NotNil(t, attr)
		assert.True(t, attr.Setpgid, "Setpgid deve ser true")
	})
}

func TestGroupProcessDiesCleanly(t *testing.T) {
	// Arrange — processo que termina sozinho.
	child := exec.Command("sh", "-c", "exit 0")
	child.SysProcAttr = proc.Group()
	require.NoError(t, child.Start())
	pid := child.Process.Pid

	_ = child.Wait()

	// Act — KillGroup após o processo já ter saído.
	err := proc.KillGroup(pid)
	assert.NoError(t, err, "ESRCH após saída limpa deve ser ignorado")

	// Verifica que o PID não existe mais.
	esperaAte(t, 5*time.Second, func() bool {
		return processoMorto(pid)
	}, "processo filho não deveria mais existir (pid=%d)", pid)
}
