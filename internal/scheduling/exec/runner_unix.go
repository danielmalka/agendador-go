//go:build unix

package exec

import (
	"errors"
	"os"
	osexec "os/exec"
	"syscall"
)

// configureProcessGroup runs the command in its own process group and, on
// cancellation, kills the whole group so grandchildren do not outlive the job.
func configureProcessGroup(cmd *osexec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}
