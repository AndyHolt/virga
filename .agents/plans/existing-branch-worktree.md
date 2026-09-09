# Existing branch worktree command implementation plan

## Objective

Add a Virga workflow for an existing local branch: if the branch is already checked out in a Git worktree, Virga should take the user to that worktree's tmux environment; if the branch exists but has no worktree, Virga should create the worktree and tmux environment without creating a new branch.

The plan also adds inventory/switching primitives first, because the requested behavior depends on reliably discovering current worktrees and Virga-managed tmux sessions.

## Scope

### In scope

- List worktrees for the current repository, including the expected Virga tmux session status for branch worktrees.
- Add an explicit command, proposed as `virga open <branch>`, for existing local branches.
- For `virga open <branch>`:
  - Reuse the branch's existing worktree when one is already checked out.
  - Create a sibling worktree with `git worktree add <destination> refs/heads/<branch>` when the branch exists but is not checked out.
  - Ensure the deterministic Virga tmux session exists for that branch/worktree.
  - Attach or switch to the session when interactive, respecting `--no-attach` and `--no-tmux`-style escape hatches.
  - Materialize configured files only when Virga creates a new worktree, not when it reuses an existing one.
- Improve `virga new <branch>` errors for existing branches so users are directed to `virga open <branch>` when appropriate.
- Add tests and README/help documentation for the new behavior.

### Out of scope

- Remote branch checkout/tracking; initial support is for existing local branches only.
- Moving or deleting existing worktrees.
- Force-opening locked/prunable worktrees, or running `git worktree prune` automatically.
- Associating arbitrary tmux sessions with worktrees. Initial tmux discovery should use Virga's deterministic `tmux.SessionName(repositoryRoot, branch)` convention.
- Prompting from `virga new` by default. The safer first behavior is an actionable message/suggestion; an interactive prompt can be added later if desired.

## Current state

Verified in this worktree:

- The repository currently has `info` and `new` commands. `go run ./main.go --help` lists only those application commands plus Cobra's generated help/completion commands.
- `virga new <branch>` creates a new local branch and sibling worktree, then optionally materializes files and creates/attaches tmux.
- `internal/git.CreateWorktree` always uses `git worktree add -b <new-branch> ...` and intentionally rejects any existing local branch via `LocalBranchExistsError`.
- Worktree inspection exists for the current directory (`internal/git.InspectWorktree`), but there is no API to list all worktrees and no command to switch/open a different one.
- tmux session names are deterministic (`internal/tmux.SessionName`), and sessions can be created/attached, but there is no API to check whether a session already exists or to switch clients when already inside tmux.
- Configuration and file materialization already exist and are wired into `new`.
- `go test ./...` passed before planning.

## Proposed UX and decisions

### Commands

```sh
virga list
virga list --json
virga open feature/login
virga open feature/login --no-attach
virga open feature/login --no-tmux
```

Rationale:

- Keep `virga new <branch>` strict: it creates a new branch and remains compatible with current behavior.
- Use `virga open <branch>` for idempotent "open this existing branch environment" behavior. This avoids surprising prompts in scripts and directly covers the case where the branch already has a worktree.
- Use `virga list` first so users can discover worktrees and sessions before opening them.

### `virga open <branch>` behavior

1. Discover the current repository's primary worktree root from any nested directory or linked worktree.
2. Validate that `<branch>` is a local branch (`refs/heads/<branch>`), using fully-qualified refs to avoid tag/revision ambiguity.
3. List existing worktrees with `git worktree list --porcelain -z`.
4. If a usable worktree already has `branch refs/heads/<branch>`:
   - Reuse that path.
   - Do not materialize configured files by default, because the worktree may contain user changes or intentional local files.
5. If the branch exists but is not checked out in a worktree:
   - Preflight the sibling destination using the existing naming convention (`<repo>_<branch-with-slashes-replaced>`).
   - Run `git worktree add <destination> refs/heads/<branch>` without `-b`.
   - Materialize configured files before tmux, matching the current `new` lifecycle.
6. Ensure tmux unless disabled:
   - Compute the deterministic session name from primary repository root and branch.
   - If the session exists, reuse it.
   - If it does not exist, create it using the worktree root and configured tmux layout.
7. If interactive and attachment is enabled:
   - Outside tmux, attach to the session.
   - Inside tmux, switch the current client to the session instead of nesting `tmux attach-session`.
8. Print stable stdout describing what was used/created, for example:

```text
Branch: feature/login
Worktree: /projects/virga_feature-login
Worktree action: reused
Tmux session: virga_feature-login_abcd1234
Tmux action: created
```

If tmux is disabled, omit the tmux lines. Errors and prompts remain on stderr.

### Existing checked-out branch handling

When `virga new <branch>` receives a branch that already exists, improve the error path by checking whether it is checked out in a worktree. If so, return an actionable message such as:

```text
local branch "feature/login" already exists and is checked out at /projects/virga_feature-login; run "virga open feature/login" to use it
```

This "offers to switch" without changing `new` into an interactive command. If the maintainer wants an actual prompt later, add it behind explicit interactive-terminal checks and preserve non-interactive script behavior.

## Delivery plan

### PR 1: Worktree and session inventory

**Outcome:** Users and later commands can discover existing repository worktrees and Virga-managed tmux session status.

#### Commit 1: Add Git worktree listing

- Implementation:
  - Add an `internal/git` type such as `ListedWorktree` with path, HEAD, branch name/ref, kind (`main`/`linked`), and Git state fields for detached, bare, locked, and prunable worktrees.
  - Implement `ListWorktrees(ctx, dir)` using `git worktree list --porcelain -z`.
  - Parse porcelain records defensively: each `worktree` starts a record; handle `HEAD`, `branch`, `detached`, `bare`, `locked`, and `prunable` fields.
  - Return `git.ErrNotGitRepository` consistently outside a repository.
- Tests:
  - Unit tests for porcelain parsing, including main, linked, detached, locked, prunable, and malformed output.
  - Integration tests with isolated Git repositories for main plus linked worktrees, invocation from nested directories, paths with spaces, and detached worktrees.
- Documentation: package comments only in this commit.

#### Commit 2: Add tmux session discovery and activation primitives

- Implementation:
  - Add tmux support for checking session existence, e.g. `HasSession(ctx, name)` using `tmux has-session -t <name>`.
  - Add an activation method that attaches outside tmux and switches clients inside tmux. Keep environment lookup injectable so tests can simulate `$TMUX`.
  - Keep `CreateSession` behavior unchanged for now; do not silently ignore existing-session errors in this commit.
- Tests:
  - Existing session, missing session, missing tmux, and command failure cases.
  - Activation chooses `attach-session` when `$TMUX` is unset and `switch-client` when `$TMUX` is set.
  - Validation for empty session names.
- Documentation: update tmux package comments as needed.

#### Commit 3: Add `virga list`

- Implementation:
  - Wire `virga list` into root command construction with injectable Git/tmux dependencies.
  - Human output should include branch, worktree path, worktree kind/state, and expected Virga tmux session status.
  - Add `--json` for scriptable output with a documented schema.
  - Listing should not require tmux to be installed; report tmux status as unavailable/unknown rather than failing the entire command.
- Tests:
  - CLI tests for human and JSON output.
  - Non-Git repository error behavior.
  - tmux installed/running/missing/unavailable states.
- Documentation:
  - README section for listing worktrees and session status.

**Acceptance criteria:**

- `virga list` works from the main worktree, a linked worktree, and nested directories.
- Detached/locked/prunable worktrees are visible and not mistaken for normal branch worktrees.
- `virga list --json` has deterministic field names and ordering.

### PR 2: Open an already-existing worktree/session

**Outcome:** Users can switch to a branch that is already checked out in a worktree and land in its existing or newly-created tmux session.

#### Commit 1: Add session ensure/reuse behavior

- Implementation:
  - Add an `EnsureSession(ctx, options)` operation in `internal/tmux` or a small command-level helper that checks `HasSession` before `CreateSession`.
  - Return both session name and action (`created` or `reused`) so command output can be explicit.
  - Treat missing tmux as an error for `open` when tmux is enabled.
- Tests:
  - Existing session is reused without calling create.
  - Missing session creates with the expected options.
  - Missing tmux and command failures are wrapped with actionable context.

#### Commit 2: Add `virga open <branch>` for checked-out branches

- Implementation:
  - Add a new Cobra command with flags `--config`, `--no-tmux`, and `--no-attach` mirroring relevant `new` behavior.
  - In this first slice, require the branch to already be present in the worktree list; if it is not checked out, return a clear "no worktree for branch" error and mention that creating one comes next.
  - Reuse existing configuration loading for tmux layout.
  - Ensure/reuse tmux session and activate it when interactive.
  - Print `Worktree action: reused` and `Tmux action: created|reused`.
- Tests:
  - Branch already checked out with existing tmux session.
  - Branch already checked out with missing tmux session.
  - `--no-tmux` skips tmux lookup/creation/activation.
  - `--no-attach` leaves the session detached.
  - Non-interactive execution creates/reuses the session but does not attach/switch.
  - Missing local branch, branch not checked out, detached-only worktree, and non-Git repository errors.
- Documentation:
  - README section for opening an existing worktree.

**Acceptance criteria:**

- `virga open <branch>` never creates a new branch.
- Existing branch worktrees are reused safely.
- Inside tmux, opening switches clients instead of nesting an attach.

### PR 3: Create a worktree for an existing branch

**Outcome:** `virga open <branch>` covers the full requested workflow: existing local branch with or without an existing worktree.

#### Commit 1: Add Git operation for existing-branch worktree creation

- Implementation:
  - Add `internal/git` functionality such as `AddExistingBranchWorktree(ctx, dir, branch)`.
  - Validate non-bare repository, local branch existence, branch ref format, and destination conflicts.
  - If the branch is already checked out in a usable worktree, return a typed error containing that path rather than invoking Git.
  - Use `git worktree add <destination> refs/heads/<branch>` with argument vectors only.
- Tests:
  - Successful creation from existing branch at the expected commit.
  - Invocation from nested directories and linked worktrees.
  - Branch names containing `/`, paths with spaces, missing branch, invalid branch, destination conflict, branch already checked out, bare/non-Git repository, and Git command failure.

#### Commit 2: Extend `virga open` to create missing worktrees

- Implementation:
  - If `ListWorktrees` finds no usable worktree for the branch, call the new existing-branch worktree creation operation.
  - Load configuration before provisioning, as `new` does.
  - Materialize configured files only for newly-created worktrees, before tmux session creation.
  - If file materialization or tmux creation fails after worktree creation, retain the worktree and report what was created. Do not delete existing branches or worktrees.
  - Output `Worktree action: created` when a worktree is created.
- Tests:
  - Existing branch with no worktree creates worktree, materializes files, creates/reuses tmux, and attaches/switches only when appropriate.
  - File materialization collision after worktree creation retains the worktree and reports partial state.
  - Tmux failure after worktree creation retains the worktree and reports partial state.
  - Existing worktree path does not trigger file materialization.
- Documentation:
  - Update README examples for `virga open <branch>` creating or reusing worktrees.

**Acceptance criteria:**

- Existing local branches can be opened without creating a new branch.
- Operation ordering is deterministic: discover, create/reuse worktree, materialize files only if newly created, ensure tmux, activate.
- Partial failures preserve user data and clearly report retained resources.

### PR 4: Improve `virga new` existing-branch guidance

**Outcome:** Users who run `virga new` for an already-existing branch get a helpful path to the new `open` workflow.

#### Commit 1: Add worktree-aware existing-branch errors

- Implementation:
  - Extend the existing `LocalBranchExistsError` handling path or add a new typed error that can include the checked-out worktree path when known.
  - When `CreateWorktree` detects an existing branch, consult `ListWorktrees` to determine whether that branch is already checked out in this repository.
  - Keep the current strict behavior: `new` does not switch, attach, or create a worktree for an existing branch.
- Tests:
  - Existing branch checked out in a worktree reports the worktree path and suggests `virga open <branch>`.
  - Existing branch not checked out still reports that the local branch exists and suggests `virga open <branch>` to create/reuse an existing-branch environment.
  - Usage/error behavior remains consistent with existing Cobra tests.
- Documentation:
  - README note: use `new` for new branches and `open` for existing branches.

#### Commit 2: Consider an explicit convenience flag only if desired

- Optional, only after maintainer confirmation:
  - Add `virga new <branch> --open-existing` or an interactive prompt for checked-out existing branches.
  - Do not make prompting the default in non-interactive contexts.
  - If added, implement by delegating to the same open workflow rather than duplicating logic.

**Acceptance criteria:**

- `virga new` remains backward compatible for scripts and strict branch creation.
- Existing-branch errors are actionable and do not expose low-level Git implementation details.

## Testing and validation

Run after each commit-sized change, leaving changes uncommitted for review:

```sh
make fmt
make test
make lint
```

Also run `make tidy` whenever dependencies change. For CLI/workflow changes, add focused integration tests with isolated temporary repositories and fake tmux runners so tests never depend on the developer's real tmux server, home directory, or global Git configuration.

## Risks and mitigations

- **Prompting can break scripts:** keep `new` non-interactive initially; use `open` for explicit switching/opening.
- **tmux attach inside tmux can nest or fail:** add an activation primitive that uses `switch-client` when `$TMUX` is set.
- **Stale/prunable worktrees may point at missing directories:** list them, but do not automatically prune or reuse them for `open`; return an actionable error.
- **Configured file materialization can overwrite user files:** materialize only when Virga creates a worktree, retain existing no-overwrite preflights, and do nothing when reusing a worktree.
- **Session name collisions or manual sessions:** reuse only the deterministic Virga session name. Document that Virga treats that name as its session for the branch.

## Assumptions and open questions

- Proposed command name is `open`. `add` would also fit existing-branch worktree creation, but `open` better describes the idempotent create-or-reuse-and-switch behavior.
- `virga list` should list Git worktrees for the current repository only, not all tmux sessions on the machine.
- Initial support is local branches only. Remote branch materialization can be a separate plan.
- Maintainer decision needed before adding an actual interactive prompt or `--open-existing` convenience behavior to `virga new`.

## Definition of done

- `virga list` documents and displays worktrees and Virga tmux session status.
- `virga open <branch>` reuses checked-out worktrees, creates worktrees for existing local branches, ensures/reuses tmux sessions, and activates only when appropriate.
- `virga new` clearly distinguishes new-branch creation from existing-branch opening and points users to `virga open`.
- README, command help, tests, and implementation agree.
- Formatting, race-enabled tests, linting, and tidy checks pass.
