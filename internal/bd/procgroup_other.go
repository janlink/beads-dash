//go:build !unix

package bd

import "os/exec"

func setProcessGroup(cmd *exec.Cmd) {
	cmd.Cancel = func() error { return cmd.Process.Kill() }
}
