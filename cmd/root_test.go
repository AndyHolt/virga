package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewRootCommand(t *testing.T) {
	t.Parallel()

	command := NewRootCommand()

	if command.Use != "virga" {
		t.Errorf("Use = %q, want %q", command.Use, "virga")
	}
	if command.Short != "Multi-branch development manager" {
		t.Errorf("Short = %q, want %q", command.Short, "Multi-branch development manager")
	}

	commands := command.Commands()
	if len(commands) != 2 || commands[0].Name() != "info" || commands[1].Name() != "new" {
		t.Errorf("subcommands = %v, want info and new", commands)
	}
	for _, flag := range []string{"config", "toggle"} {
		if command.Flag(flag) != nil {
			t.Errorf("flag %q is registered", flag)
		}
	}
}

func TestRootCommandHelp(t *testing.T) {
	t.Parallel()

	command := NewRootCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"--help"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	help := output.String()
	for _, want := range []string{
		"Multi-branch development manager",
		"Usage:",
		"virga [flags]",
		"info",
		"Describe the current Git worktree",
		"new",
		"Create a branch in a new Git worktree",
		"-h, --help",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("help output does not contain %q:\n%s", want, help)
		}
	}
}

func TestNewRootCommandReturnsIndependentCommands(t *testing.T) {
	t.Parallel()

	first := NewRootCommand()
	first.Flags().String("test-only", "", "test flag")
	first.Commands()[0].Short = "changed"
	first.Commands()[1].Short = "changed"

	second := NewRootCommand()
	if second.Flag("test-only") != nil {
		t.Error("a flag added to one root command was registered on another")
	}
	if got := second.Commands()[0].Short; got != "Describe the current Git worktree" {
		t.Errorf("info Short = %q after changing another command", got)
	}
	if got := second.Commands()[1].Short; got != "Create a branch in a new Git worktree" {
		t.Errorf("new Short = %q after changing another command", got)
	}
}
