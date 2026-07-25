# AGENTS.md

## Project overview

Virga is a Go command-line application for managing parallel development workflows. Its goal is to make multi-branch development simple by coordinating:

- Git branches and worktrees
- tmux sessions
- Local development services

The intended workflow includes creating a feature branch in its own worktree, starting or restoring its tmux session and services, and listing or managing active development environments.

Virga is currently at an early stage. The repository contains a Cobra/Viper CLI scaffold, but the commands described in the root help text are not yet implemented. Prefer small, complete vertical slices over speculative frameworks or broad abstractions.

## Repository structure

- `main.go`: application entry point
- `cmd/`: Cobra commands and CLI wiring
- `internal/`: preferred location for non-public application packages as the project grows
- `bin/`: local build output
- `.github/workflows/`: CI configuration
- `.pre-commit-config.yaml`: repository checks
- `Makefile`: common development commands

## Development commands

Use the Make targets where possible so local development matches the documented workflow:

```bash
make build   # Build bin/virga
make run     # Run the CLI from source
make test    # Run tests with race detection and coverage
make fmt     # Format Go source
make lint    # Run golangci-lint
make tidy    # Synchronize go.mod and go.sum
```

Before presenting work for review, run at least:

```bash
make fmt
make test
make lint
```

When dependencies change, also run `make tidy`. To run all repository hooks, use:

```bash
prek run --all-files
```

Do not hide, disable, or weaken checks merely to make CI pass.

## Preferred working process

Work **one commit-sized change at a time, but do not create the commit**.

A commit-sized change should be a small, coherent unit that can be reviewed independently. Once that unit is implemented and validated:

1. Stop and summarize what changed.
2. List the checks that were run and their results.
3. Call out assumptions, trade-offs, and anything not tested.
4. Leave all changes uncommitted for review.
5. Wait for feedback or explicit approval before beginning another commit-sized change.

Never run `git commit`, amend commits, create tags, push branches, or rewrite history unless explicitly instructed. Do not combine several planned commits into one large uncommitted change. This review-first workflow allows the maintainer to understand the code, request changes early, and approve each step before later work depends on it.

Before editing, inspect the working tree with `git status`. Existing uncommitted changes belong to the user unless clearly identified otherwise. Do not overwrite, discard, stage, or modify unrelated changes. Avoid `git reset --hard`, `git checkout --`, `git clean`, and equivalent destructive commands.

## Go development standards

Follow idiomatic Go and the conventions documented by the Go project.

### Design

- Keep packages focused and give them concise, descriptive names.
- Prefer simple concrete implementations until an abstraction has a demonstrated need.
- Define interfaces at the point of use, generally in the consuming package.
- Keep interfaces small. Do not introduce interfaces solely for mocking when a function or small dependency can be injected instead.
- Use `internal/` for application implementation that is not intended as a public API.
- Separate CLI concerns from domain logic: Cobra commands should parse input, invoke application logic, and render results rather than contain Git, tmux, or process-management logic directly.
- Make side effects explicit and injectable where practical, especially command execution, filesystem access, environment variables, time, and terminal detection.
- Prefer Git's stable machine-readable or porcelain formats over parsing human-oriented output.
- Avoid global mutable state. Cobra command construction should remain testable and should not depend unnecessarily on package globals.
- Preserve backward compatibility for documented CLI flags, output formats, configuration keys, and exit behavior unless a breaking change is deliberate and documented.

### Correctness and errors

- Handle every meaningful error. Do not silently discard errors.
- Add context when returning errors, using `%w` when wrapping: `fmt.Errorf("list worktrees: %w", err)`.
- Use errors for exceptional conditions rather than panics. Panics are reserved for unrecoverable programmer errors.
- Validate user input at command boundaries and return actionable error messages.
- Keep stdout suitable for requested command output; send diagnostics and errors to stderr.
- Use exit codes consistently. Do not call `os.Exit` below the top-level application boundary because it prevents deferred cleanup and makes testing difficult.
- Pass `context.Context` through operations that may block or invoke external processes.
- Set subprocess working directories, environments, and arguments explicitly. Never construct shell commands by concatenating untrusted input.
- Clean up partially created resources when an operation fails, or clearly report what remains.

### Style and documentation

- Format all Go code with `gofmt`; use `goimports` if it is adopted by the repository.
- Follow standard Go naming: initialisms such as `ID`, `URL`, and `CLI` remain consistently capitalized.
- Keep functions short enough to understand, but do not split code into trivial wrappers without benefit.
- Comments should explain intent, constraints, and non-obvious decisions rather than restate the code.
- Exported identifiers require useful Go documentation comments.
- Remove dead code and stale generated comments. Do not leave commented-out implementation templates.
- Avoid adding dependencies when the standard library is sufficient. Explain and justify significant new dependencies.
- Keep `go.mod` and `go.sum` tidy and commit-sized with the code that needs dependency changes.

### Testing

- Add or update tests for every behavioral change and bug fix.
- Prefer table-driven tests for multiple input/output cases.
- Test observable behavior rather than implementation details.
- Use temporary directories (`t.TempDir`) and isolated repositories for filesystem and Git integration tests.
- Never rely on the developer's global Git configuration, current branches, tmux server, home directory, or existing worktrees.
- Use dependency injection or controlled test binaries for external-command unit tests. Add focused integration tests where exercising real Git behavior provides meaningful confidence.
- Mark test helpers with `t.Helper()` and use `t.Cleanup()` for cleanup.
- Keep tests deterministic and safe to run in parallel. Do not add sleeps to resolve synchronization issues.
- Test successful behavior, malformed input, external-command failures, and important edge cases such as detached or locked worktrees.
- Run tests with the race detector via `make test`.

## CLI standards

- Commands and flags should have concise help text and practical examples.
- Do not advertise unimplemented behavior in user-facing help or documentation.
- Prefer predictable, script-friendly behavior. Human-readable output should be stable where practical, and structured output such as `--json` should have an explicitly defined schema.
- Avoid prompts in non-interactive environments. Destructive actions should require explicit intent and offer a non-interactive mode when appropriate.
- Respect configuration precedence consistently: explicit flags, environment variables, configuration files, then defaults. Document any deviations.
- Ensure commands work from nested directories inside a repository when that is a reasonable user expectation.

## Security and portability

- Treat repository content, branch names, paths, configuration, and subprocess output as untrusted input.
- Pass subprocess arguments as an argument vector; avoid invoking a shell unless shell semantics are genuinely required.
- Do not log secrets, environment values, tokens, or private configuration.
- Use restrictive permissions for files that may contain sensitive data.
- Prefer cross-platform Go APIs. When functionality depends on Git, tmux, or operating-system-specific behavior, detect missing prerequisites and return clear errors.
- Consider paths containing spaces, Unicode, symlinks, and platform-specific separators.

## Agent coding standards

- Read relevant code, tests, configuration, and documentation before proposing changes.
- Confirm the requested scope and avoid unrelated refactoring or formatting churn.
- Prefer the smallest change that completely solves the stated problem.
- Do not fabricate APIs, command behavior, test results, or repository state. Verify them directly.
- Keep README files, CLI help, examples, configuration documentation, and this file consistent with behavior.
- If requirements are ambiguous and the choice would materially affect behavior or architecture, ask before implementing.
- Record assumptions when a minor ambiguity does not justify blocking progress.
- Do not introduce generated files, binaries, editor metadata, or local configuration unless they are intentionally part of the project.
- Review the final diff and `git status` before handing work back.
- Clearly identify any checks that could not be run and why.

## Definition of done

A commit-sized change is ready for maintainer review when:

- The requested behavior is implemented without unrelated changes.
- Relevant tests have been added or updated and pass.
- Formatting, linting, race-enabled tests, and applicable repository hooks pass.
- User-facing behavior and documentation agree.
- Dependency files are tidy.
- The diff has been reviewed for correctness, security, and accidental files.
- Changes remain uncommitted and are summarized for the maintainer.
