//go:build !unix

package executor

import "os/exec"

// configureProcessGroup keeps the default behavior: only the direct child is killed.
func configureProcessGroup(cmd *exec.Cmd) {
	cmd.Cancel = func() error { return cmd.Process.Kill() }
}
