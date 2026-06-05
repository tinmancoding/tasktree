//go:build !unix

package bootstrap

import (
	"os/exec"
	"syscall"
)

// On non-unix platforms process-group control is unavailable; fall back to
// signalling the direct child only.
const (
	syscallSIGTERM = syscall.Signal(0xf) // SIGTERM placeholder
	syscallSIGKILL = syscall.Signal(0x9) // SIGKILL placeholder
)

func setProcGroup(cmd *exec.Cmd) {}

func killGroup(cmd *exec.Cmd, _ syscall.Signal) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
