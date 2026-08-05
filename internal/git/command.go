package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

type outputRunner func(context.Context, string, ...string) ([]byte, error)

// output runs Git in dir and returns its standard output. Keeping command
// execution here gives all semantic Git operations consistent locale and error
// handling without exposing raw command execution to callers.
func output(ctx context.Context, dir string, arguments ...string) ([]byte, error) {
	args := append([]string{"-C", dir}, arguments...)
	command := exec.CommandContext(ctx, "git", args...)
	command.Env = append(os.Environ(), "LC_ALL=C")
	commandOutput, err := command.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("%w: %s", err, bytes.TrimSpace(exitErr.Stderr))
		}
		return nil, err
	}
	return commandOutput, nil
}
