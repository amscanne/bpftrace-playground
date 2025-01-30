//go:build !linux

package evaluate

import (
	"context"
	"os/exec"
)

func getCommand(ctx context.Context, _ string, code string) *exec.Cmd {
	// On non-Linux systems, simply echo the code back.
	return exec.CommandContext(ctx, "/bin/echo", code)
}
