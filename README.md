# 🌳 virga

Virga manages Git worktrees for parallel development.

## List worktrees

List the current repository's Git worktrees and the expected Virga tmux session
for each branch worktree:

```sh
virga list
virga list --json
```

Tmux status is reported as `running`, `missing`, `unavailable` when tmux is not
installed, or `unknown` when tmux cannot be queried. Detached worktrees do not
have an expected Virga tmux session.

JSON output is an object with a `worktrees` array. Each worktree contains
`path`, `type`, `head`, `branch`, `branch_ref`, `state`, `detached`, `bare`,
`locked`, `lock_reason`, `prunable`, `prune_reason`, and `tmux` fields. The
`tmux` object contains `session` and `status`.

## Open an existing worktree

Open a local branch that is already checked out in a Git worktree:

```sh
virga open feature/login
```

Virga reuses the existing worktree, ensures the deterministic tmux session for
that worktree exists, and attaches or switches to that session when running
interactively. It prints the branch, worktree path, `Worktree action: reused`,
and `Tmux action: created` or `reused`.

Skip tmux setup or leave the session detached with:

```sh
virga open feature/login --no-tmux
virga open feature/login --no-attach
```

This first `open` implementation does not create missing worktrees. If the local
branch exists but is not checked out in a worktree, Virga reports that creating a
worktree for an existing branch is not implemented yet.

## Create a worktree

Create a new branch and a sibling worktree from the branch currently checked out:

```sh
virga new feature/login
```

Virga prints the new branch, absolute worktree path, and tmux session name. For
a repository at `/projects/virga`, this command creates the worktree at
`/projects/virga_feature-login` and names the tmux session
`virga_feature-login`.

### Choose a base branch

By default, the new branch starts at the currently checked-out branch. Select a
different local base branch with one of these mutually exclusive options:

```sh
virga new feature/login --from release # a named local branch
virga new feature/login --main         # the local branch named main
virga new feature/login --pick         # choose a local branch interactively
```

`--pick` requires an interactive terminal. Its prompt is written to stderr, so
stdout remains suitable for scripts.

The command must run from inside a non-bare Git worktree. The default base
selection requires a checked-out branch. Existing local branches and destination
directories are rejected.

### Configured files

Virga can materialize repository-local files into each new worktree before tmux
starts. This is intended for files such as untracked local environment files that
startup commands need:

```yaml
files:
  - source: .env
    mode: symlink
  - source: config/local.yaml
    mode: copy
```

Each `source` is relative to the primary repository root and is created at the
same relative path in the new worktree. `mode` must be either `copy` or
`symlink`. Symlinks use relative targets where possible.

Configured file paths must stay inside the primary repository and destination
paths must stay inside the new worktree. Virga rejects existing destinations by
default and preflights all configured entries before copying or linking any of
them. If provisioning fails after Git creates the branch and worktree, Virga
keeps those resources and reports what was created rather than deleting them.

The `virga new` lifecycle is:

1. Create the branch and worktree.
2. Copy or link configured files.
3. Create the tmux session.
4. Attach to tmux when appropriate.

### tmux sessions

By default, `virga new` creates a detached tmux session in the new worktree. If
Virga is running interactively, it attaches or switches to that session after
creation.

Skip tmux setup or leave the created session detached with:

```sh
virga new feature/login --no-tmux
virga new feature/login --no-attach
```

Tmux windows and pane startup commands can be configured in `.virga.yaml` at the
primary repository root, in the user configuration file, or with an explicit
configuration file:

```yaml
tmux:
  windows:
    - name: editor
      panes:
        - command: nvim
        - command: make test
```

```sh
virga new feature/login --config ./local.virga.yaml
```

Pane commands execute project-controlled code in the new worktree. File entries
can copy or link sensitive local files into that worktree. Review repository
configuration before running `virga new` in repositories you do not trust.
