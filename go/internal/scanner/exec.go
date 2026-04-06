package scanner

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
)

// RunCommand executes a command with the given context and returns stdout.
// If the binary is not found (tool not installed), returns ("", nil) for
// graceful skip. All other errors are returned normally.
func RunCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", nil
		}
		return "", err
	}
	return stdout.String(), nil
}
