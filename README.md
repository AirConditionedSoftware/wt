# wt

A small CLI for managing git worktrees. Worktree placement is driven by a JSON
config file and can be overridden per repository.

```console
$ wt add feature/login
Creating worktree with new branch "feature/login" from main
/Users/you/worktrees/myapp/feature-login

$ wt list
  BRANCH         PATH                                         HEAD      STATUS
* main           /Users/you/code/myapp                        1a2b3c4d
  feature-login  /Users/you/worktrees/myapp/feature-login     1a2b3c4d
```

## Install

```sh
brew install piskandar/tap/wt
```

Or with Go:

```sh
go install github.com/piskandar/wt@latest
```

## Usage

- `wt list [--json]` — list all worktrees of the current repository. The `*`
  marks the worktree you're in.
- `wt add <branch> [--base <ref>] [--path <dir>]` — create a worktree:
  - branch exists locally → checked out as-is
  - branch exists on `origin` → local branch created tracking it (fetches
    once if the remote ref isn't known yet)
  - otherwise → new branch created from `--base`, the config's
    `default_base`, or the current HEAD

`wt add` prints only the created path on stdout (everything else goes to
stderr), so you can hop straight into a new worktree with a shell function:

```sh
wtcd() { cd "$(wt add "$1")"; }
```

## Configuration

Config lives at `~/.wt/wt.json`, or wherever `$WT_CONFIG` points. A missing
file at the default location just means defaults; a missing file at an
explicit `$WT_CONFIG` path is an error.

```json
{
  "worktree_dir": "~/worktrees/{repo}/{branch}",
  "default_base": "main",
  "repos": {
    "myapp": {
      "worktree_dir": "~/code/myapp-trees/{branch}",
      "default_base": "develop"
    }
  }
}
```

- `worktree_dir` — path template for new worktrees. `{repo}` is the directory
  basename of the main worktree, `{branch}` is the branch name with `/`
  replaced by `-` (so `feature/login` → `feature-login`). A leading `~`
  expands to your home directory. Default: `~/worktrees/{repo}/{branch}`.
- `default_base` — ref that brand-new branches start from. Default: current
  HEAD.
- `repos` — per-repository overrides, keyed by repo name (`{repo}` as defined
  above). A repo entry overrides the top-level values field by field.

Unknown keys are rejected so typos fail loudly.

## Development

```sh
go test ./...
go build .
```

Requires the `git` binary on PATH (all git operations shell out; worktree
porcelain output is a stable interface).

## Releasing

Releases are automated with [goreleaser](https://goreleaser.com) via GitHub
Actions on tags matching `v*`. One-time setup:

1. Create a `homebrew-tap` repository on GitHub.
2. Add a `TAP_GITHUB_TOKEN` repo secret: a token with write access to the tap
   repository (used to push the updated Homebrew cask).
3. Tag and push: `git tag v0.1.0 && git push origin v0.1.0`.

Validate the release config locally with
`goreleaser release --snapshot --clean` (needs at least one commit).
