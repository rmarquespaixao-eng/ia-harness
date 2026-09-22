//go:build unix

package proc

import (
	"errors"
	"syscall"
)

// Group devolve SysProcAttr que coloca o filho em grupo próprio (Setpgid=true),
// permitindo KillGroup matar a árvore inteira.
func Group() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// KillGroup envia SIGKILL para o grupo de processos identificado por pgid.
// ESRCH (grupo já encerrado) é ignorado.
func KillGroup(pgid int) error {
	err := syscall.Kill(-pgid, syscall.SIGKILL)
	if err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}
