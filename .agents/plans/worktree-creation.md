Recommended command

Use:

```text
  virga new <branch>
```

new communicates that Virga creates both a new branch and worktree. Reserve add for potentially adding an existing branch/worktree later.

Proposed base-selection interface:

```bash
  virga new feature/login                  # from current checked-out branch
  virga new feature/login --main           # from local main
  virga new feature/login --from release   # from a named local branch
  virga new feature/login --pick           # interactively select a local branch
```

--main, --from, and --pick would be mutually exclusive.

Proposed conventions

- Primary repository: discovered through Git’s common directory, even when invoked from a linked worktree or nested directory.
- Worktree location: sibling of the primary repository.
- Directory naming: <repository>_<branch>.
- Replace / in branch names with -; for example:
    - Repository: virga
    - Branch: feature/login
    - Directory: ../virga_feature-login
- Reject existing destination directories and existing local branches.
- Default creation requires a checked-out branch; detached HEAD produces an actionable error.
- Uncommitted files are not transferred—the new branch starts at the selected branch’s commit.

The underscore convention also matches the existing virga_test-worktree sibling in this repository.

────────────────────────────────────────────────────────────────────────────────

PR 1: Basic worktree creation

Deliver the first complete vertical slice:

```bash
  virga new <branch>
```

It creates a branch from the currently checked-out branch and creates its sibling worktree.

### Commit 1: Make command construction testable

- Replace package-global Cobra state with a root-command constructor.
- Keep os.Exit at the application entry point.
- Remove generated placeholder flags and help text advertising unimplemented commands.
- Add focused root-command tests.

### Commit 2: Implement Git worktree creation

Add a small internal package responsible for:

- Discovering the current and primary repository roots.
- Confirming the repository is non-bare.
- Reading the currently checked-out branch.
- Validating the requested branch with Git.
- Calculating the sibling destination.
- Checking branch and path conflicts.
- Running Git using argument vectors, approximately:

```bash
  git worktree add -b <new-branch> <destination> refs/heads/<current-branch>
```

Use context.Context, wrap errors, and avoid shell command construction.

Tests should use isolated temporary Git repositories and cover:

- Successful creation.
- Invocation from a nested directory.
- Invocation from an existing linked worktree.
- Branch names containing /.
- Paths containing spaces.
- Existing branch and destination conflicts.
- Detached HEAD.
- Git command failure.
- Verification that the new branch points at the expected commit.

### Commit 3: Wire virga new

- Add Cobra argument validation and practical examples.
- Print a stable success result containing the branch and absolute worktree path.
- Keep diagnostics on stderr.
- Document the command in README.md.
- Add CLI-level tests.

────────────────────────────────────────────────────────────────────────────────

PR 2: Base branch selection

### Commit 1: Explicit base branches

Add:

```bash
  virga new <branch> --from <local-branch>
  virga new <branch> --main
```

- --main means the local branch literally named main.
- Resolve fully qualified local refs to avoid Git revision ambiguity.
- Return clear errors when the base branch does not exist.
- Add tests for selection, conflicts, and mutually exclusive options.

A later configuration option could define a project’s primary branch if supporting master or another convention becomes necessary.

### Commit 2: Interactive selection

Add:

```bash
  virga new <branch> --pick
```

- List local branches using git for-each-ref and a machine-readable format.
- Use an injected selector so tests do not require a real terminal.
- Start with a dependency-free numbered selector.
- Require an interactive terminal and fail clearly otherwise.
- Keep prompts on the terminal/stderr, leaving stdout suitable for command output.

### Commit 3: Documentation and end-to-end tests

Document all base-selection forms and test that each creates the branch from the expected commit.

────────────────────────────────────────────────────────────────────────────────

PR 3: Repository and user configuration, and tmux sessions

Introduce .virga.yaml in the primary repository, discoverable from any linked worktree, plus a per-user configuration file for defaults shared across repositories.

Suggested precedence, from highest to lowest:

1. Explicit --config
2. VIRGA_* environment variables where appropriate
3. Primary repository .virga.yaml
4. User configuration at ${XDG_CONFIG_HOME:-$HOME/.config}/virga/config.yaml on Unix-like systems, including macOS
5. Built-in defaults

Follow the XDG location on macOS as well as Linux because Virga is a developer-focused CLI and interoperability with dotfile managers is more useful here than the native macOS application-support convention. Use $XDG_CONFIG_HOME when it is set to an absolute path and otherwise fall back to $HOME/.config; on Windows, use os.UserConfigDir, which normally yields %AppData%. A dedicated virga/config.yaml is more idiomatic for a new application and scales better than adding another top-level home dotfile. Missing repository and user files are optional; an explicitly supplied --config path is not.

### Commit 1: Configuration foundation

- Define and validate the tmux schema.
- Load optional user configuration first, then merge repository configuration over it; apply environment and explicit configuration according to the precedence above.
- Resolve the repository configuration and any project-relative values against the primary repository; resolve the user configuration using the XDG rule above on Unix-like systems and os.UserConfigDir on Windows.
- Keep environment, home-directory, platform, and config-directory lookup injectable so tests never depend on the developer’s real configuration.
- Avoid printing unsolicited “Using config file” diagnostics.
- Test absent files, each source independently, precedence across all sources, valid merged configuration, malformed and unknown configuration, XDG set/unset/relative handling, an unavailable home or user config directory, and a missing explicit --config path.

### Commit 2: tmux session creation

Add an internal tmux component that:

- Detects whether tmux is installed.
- Generates a deterministic session name from repository and branch.
- Creates the session with the worktree as its working directory.
- Creates configured windows and panes.
- Uses explicit subprocess arguments.
- Is testable through a fake command runner.

### Commit 3: Integrate with virga new

After successful worktree creation:

1. Create the tmux session.
2. Attach when running interactively.
3. Leave the session running without attaching in non-interactive use.

Provide escape hatches such as:

```bash
  virga new feature/login --no-tmux
  virga new feature/login --no-attach
```

If tmux setup fails, retain the valid worktree and clearly report what was created rather than deleting it unexpectedly.

Configuration-defined pane commands execute project-controlled code, so this trust boundary should be documented.

────────────────────────────────────────────────────────────────────────────────

PR 4: Configured file copying and symlinking

Extend .virga.yaml with entries such as:

```yaml
  files:
    - source: .env
      mode: symlink
    - source: config/local.yaml
      mode: copy
```

### Commit 1: File materialization engine

- Support files relative to the primary repository.
- Support copy and symlink.
- Use relative symlinks where practical.
- Reject path traversal and destinations outside the worktree.
- Reject existing destinations by default.
- Preflight all entries before making changes.
- Test spaces, Unicode, missing sources, collisions, and symlink behavior.

### Commit 2: Integrate into creation lifecycle

The final order should be:

1. Create branch and worktree.
2. Copy or link configured files.
3. Create tmux session.
4. Attach when appropriate.

This ensures tmux startup commands can use files such as .env.

On provisioning failure, retain the worktree and report partial state conservatively. Automatic cleanup can be added later only with strong guarantees that it cannot remove user-created data.

### Commit 3: Documentation and end-to-end coverage

Document the configuration schema, lifecycle, collision behavior, and security implications.

────────────────────────────────────────────────────────────────────────────────

Validation for every commit-sized change

After each commit-sized unit, stop for review with changes uncommitted and run:

```bash
  make fmt
  make test
  make lint
```

Run make tidy whenever dependencies change, and review git diff plus git status before handoff.
