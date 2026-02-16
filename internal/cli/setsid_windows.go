//go:build windows

package cli

import "os/exec"

func setSysProcAttr(cmd *exec.Cmd) {
	// Setsid not available on Windows; background daemon runs without session detach.
}
