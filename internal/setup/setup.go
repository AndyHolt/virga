// Package setup runs configured commands in newly created worktrees.
package setup

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
)

// Options describes configured setup commands to run in a worktree.
type Options struct {
	WorktreeRoot string
	Commands     []string
	Stdout       io.Writer
	Stderr       io.Writer
}

type executor func(context.Context, string, string, io.Writer, io.Writer) error

// Run executes commands sequentially in WorktreeRoot. Commands are interpreted
// by the platform shell so configuration can use shell syntax such as pipes and
// variable expansion.
func Run(ctx context.Context, options Options) error {
	return run(ctx, options, executeShell)
}

func run(ctx context.Context, options Options, execute executor) error {
	if options.WorktreeRoot == "" {
		return fmt.Errorf("worktree root is required")
	}
	for index, command := range options.Commands {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := execute(ctx, options.WorktreeRoot, command, options.Stdout, options.Stderr); err != nil {
			return fmt.Errorf("run setup.commands[%d] %q: %w", index, command, err)
		}
	}
	return nil
}

func executeShell(ctx context.Context, directory, command string, stdout, stderr io.Writer) error {
	var shell string
	var arguments []string
	if runtime.GOOS == "windows" {
		shell = "cmd.exe"
		arguments = []string{"/C", command}
	} else {
		shell = "sh"
		arguments = []string{"-c", command}
	}

	process := exec.CommandContext(ctx, shell, arguments...)
	process.Dir = directory
	process.Stdout = stdout
	process.Stderr = stderr
	return process.Run()
}
