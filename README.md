# 🌳 virga

Virga manages Git worktrees for parallel development.

## Create a worktree

Create a new branch and a sibling worktree from the branch currently checked out:

```sh
virga new feature/login
```

Virga prints the new branch, absolute worktree path, and tmux session name. For
a repository at `/projects/virga`, this command creates the worktree at
`/projects/virga_feature-login`.

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

### tmux sessions

By default, `virga new` creates a detached tmux session in the new worktree. If
Virga is running interactively, it attaches to that session after creation.

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

Pane commands execute project-controlled code in the new worktree. Review
repository configuration before running `virga new` in repositories you do not
trust.
