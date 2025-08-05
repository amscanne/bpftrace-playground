//go:build linux

package evaluate

import (
	"context"
	"os/exec"
)

func getCommand(ctx context.Context, path string, code string) *exec.Cmd {
	return exec.CommandContext(ctx, path, "-e", code)
}
