//go:build !unix

package exec

import osexec "os/exec"

// configureProcessGroup keeps the default behavior: only the direct child is killed.
func configureProcessGroup(cmd *osexec.Cmd) {
	cmd.Cancel = func() error { return cmd.Process.Kill() }
}
