//go:build linux

package evaluate

import (
	"context"
	"os/exec"
	"syscall"
)

func getCommand(ctx context.Context, path string, code string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, path, "-e", code)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// While we have full kernel access, put the process in
		// network namespace to make shelling out at least moderately
		// more difficult. We don't assume this as part of the our
		// basic security model, but it helps.
		Cloneflags: syscall.CLONE_NEWNET,
	}
	return cmd
}
