//go:build !unix

package bootstrap

import (
	"os/exec"
	"syscall"
)

const (
	syscallSIGTERM = syscall.Signal(0xf)
	syscallSIGKILL = syscall.Signal(0x9)
)

func setProcGroup(cmd *exec.Cmd) {}
func killGroup(cmd *exec.Cmd, _ syscall.Signal) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
