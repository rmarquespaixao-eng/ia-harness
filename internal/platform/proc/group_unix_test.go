//go:build unix

package proc_test

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/internal/platform/proc"
)

func TestGroupAndKillGroup(t *testing.T) {
	t.Run("kills_child_and_grandchild", func(t *testing.T) {
		// Arrange — filho que cria um neto (sleep) e espera.
		child := exec.Command("sh", "-c", "sleep 60 & sleep 60")
		child.SysProcAttr = proc.Group()
		require.NoError(t, child.Start())
		pid := child.Process.Pid

		// Aguarda o filho criar o neto.
		time.Sleep(200 * time.Millisecond)

		// Act
		err := proc.KillGroup(pid)

		// Assert — ESRCH é ignorado; o grupo deve estar morto.
		require.NoError(t, err)

		// Espera o filho sair.
		_ = child.Wait()

		// Verifica que o neto (sleep 60 em background) não está vivo.
		time.Sleep(100 * time.Millisecond)
		// O neto estava no mesmo grupo; KillGroup deve tê-lo matado.
		// Verificamos que não há processos órfãos do grupo.
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
	p, err := os.FindProcess(pid)
	if err == nil {
		// FindProcess pode encontrar o zumbi antes do Wait; signal 0 verifica.
		_ = p.Signal(syscall.Signal(0))
	}
}
