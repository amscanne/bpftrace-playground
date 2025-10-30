//go:build linux

package evaluate

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
)

func getCommand(ctx context.Context, workload string, path string, code string) *exec.Cmd {
	args := []string{path}
	if workload != "" {
		childCommand := fmt.Sprintf("/proc/self/exe %s", workload)
		args = append(args, "-c", childCommand)
	}
	args = append(args, "-e", code)
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWPID,
	}
	return cmd
}
