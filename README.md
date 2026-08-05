# 🌳 virga

Virga manages Git worktrees for parallel development.

## Create a worktree

Create a new branch and a sibling worktree from the branch currently checked out:

```sh
virga new feature/login
```

Virga prints the new branch and absolute worktree path. For a repository at
`/projects/virga`, this command creates the worktree at
`/projects/virga_feature-login`.

The command must run from inside a non-bare Git worktree with a checked-out
branch. Existing local branches and destination directories are rejected.
