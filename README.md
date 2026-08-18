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

Grab a binary from
[GitHub Releases](https://github.com/AirConditionedSoftware/wt/releases), or
install with Go:

```sh
go install github.com/AirConditionedSoftware/wt@latest
```

(A Homebrew tap is planned but not wired up yet.)

## Usage

- `wt list [--json]` — list all worktrees of the current repository. The `*`
  marks the worktree you're in.
- `wt add <branch> [--base <ref>] [--path <dir>]` — create a worktree:
  - branch exists locally → checked out as-is
  - branch exists on `origin` → local branch created tracking it (fetches
    once if the remote ref isn't known yet)
  - otherwise → new branch created from `--base`, the config's
    `default_base`, or the current HEAD — named with the config's
    `branch_prefix` if one is set (`--no-prefix` skips it)
  - `--copy-hooks` copies the repo's git hooks into the new worktree;
    `--no-copy-hooks` overrides a `copy_hooks` config that enables it
  - `--copy-file <path-or-glob>` (repeatable) copies untracked files into
    the new worktree on top of the config's `copy_files` list;
    `--no-copy-files` skips the config list
- `wt prune [--dry-run]` — clean up git's bookkeeping for stale worktrees,
  i.e. entries whose directories were deleted manually (shown as `prunable`
  in `wt list`). Prints what it prunes; branches and existing directories
  are untouched.
- `wt du [--unit KB|MB|GB]` (alias: `wt disk`) — disk space used by each
  worktree, largest first, plus a total. Sizes count the working files (the
  shared repository database in `.git` isn't attributed to any worktree);
  by default each row picks a readable unit, `--unit` forces one.
- `wt config` — print the config file location (stderr) and its content
  (stdout, so `wt config | jq` works). Prints the built-in defaults if no
  file exists, and fails loudly if the file is invalid.
- `wt remove [branch...] [--force]` (aliases: `wt rm`, `wt -r`) — remove the
  worktrees that have the given branches checked out; the branches themselves
  are kept. Paths work too. With no arguments, an interactive picker lets you
  select one or more worktrees to delete, showing each branch's last commit
  and how long ago it was made. Refuses to remove the main worktree
  or the one you're in; `--force`/`-f` passes through to git for dirty or
  locked worktrees.

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
  "branch_prefix": "peter",
  "repos": [
    {
      "name": "myapp",
      "path": "~/code/myapp",
      "worktree_dir": "~/code/myapp-trees/{branch}",
      "default_base": "develop",
      "branch_prefix": "team",
      "prefix_separator": "-",
      "copy_hooks": true,
      "copy_files": [".env*", "config/local.json"]
    }
  ]
}
```

- `worktree_dir` — path template for new worktrees. `{repo}` is the directory
  basename of the main worktree, `{branch}` is the branch name with `/`
  replaced by `-` (so `feature/login` → `feature-login`). A leading `~`
  expands to your home directory. Default: `~/worktrees/{repo}/{branch}`.
- `default_base` — ref that brand-new branches start from. Default: current
  HEAD.
- `branch_prefix` — prefix for branch names that `wt add` creates, joined to
  the name with `prefix_separator`: with `"branch_prefix": "peter"`,
  `wt add fix-login` creates the branch `peter/fix-login`. A prefix that
  already ends in the separator isn't doubled, so `"peter/"` works the same.
  Branches that already exist — with or without the prefix — are used as-is,
  and typing an already-prefixed name won't double-prefix it. Bypass for one
  invocation with `--no-prefix`.
- `prefix_separator` — what joins `branch_prefix` to the branch name.
  Default: `/`. Set `"-"` for branches like `peter-fix-login`.
- `copy_hooks` — copy the repo's git hooks into each new worktree. This
  matters when `core.hooksPath` points inside the worktree (husky's
  `.husky`, a `.githooks` dir): git resolves such a path per worktree, so
  new worktrees silently lose the hooks. Plain `.git/hooks` needs no copying
  — git already shares it across all worktrees, and wt says so instead of
  copying. `--copy-hooks` / `--no-copy-hooks` override the config per
  invocation.
- `copy_files` — untracked files to copy into each new worktree, as paths or
  globs relative to the main worktree (`.env*`, `config/local.json`). A
  matched directory copies recursively; permissions are preserved; a pattern
  that matches nothing prints a note. A repo entry's list *replaces* the
  global one (it doesn't append), and an explicit `[]` turns copying off for
  that repo. `--copy-file` adds one-off entries; `--no-copy-files` skips the
  config list.
- `repos` — per-repository overrides, explained below.

Unknown keys are rejected so typos fail loudly.

### Per-repository overrides (`repos`)

`repos` is an array of entries, each tying one repository — identified by
the **filesystem path of its main worktree** — to settings that override the
top-level ones:

- `path` (required) — the path of the repo's main worktree, e.g.
  `~/code/myapp`. Before comparing, `~` is expanded and symlinks are
  resolved on both sides, and because wt finds the main worktree through
  git, the entry applies no matter which of the repo's worktrees you run wt
  from. The first matching entry wins.
- `name` (optional) — what `{repo}` expands to in `worktree_dir` templates
  for this repo. Default: the directory basename of the main worktree.
- any of the settings fields above: `worktree_dir`, `default_base`,
  `branch_prefix`, `prefix_separator`, `copy_hooks`, `copy_files`.

Settings resolve in three layers, field by field:

1. built-in defaults, overlaid with
2. the top-level fields in wt.json, overlaid with
3. the `repos` entry whose `path` matches.

An entry only needs the fields it wants to change; anything it omits falls
through to the layer below. With the example config above, running
`wt add fix-login` inside `~/code/myapp` (or any of its worktrees) resolves
to:

| field              | value                         | comes from |
| ------------------ | ----------------------------- | ---------- |
| `worktree_dir`     | `~/code/myapp-trees/{branch}` | repo entry |
| `default_base`     | `develop`                     | repo entry |
| `branch_prefix`    | `team`                        | repo entry |
| `prefix_separator` | `-`                           | repo entry |

so the created branch is `team-fix-login`, while any other repo gets
`~/worktrees/{repo}/{branch}`, `main`, and `peter` with the default `/`
separator (branches like `peter/fix-login`) from the top level. Note that an empty string in a repo entry does
not clear an inherited value — it's treated the same as omitting the field
(use `--no-prefix` to skip a prefix per invocation).

Repos without an entry need no configuration at all; `repos` is purely
opt-in per repo. An entry without a `path` is rejected when the config is
loaded.

## Development

```sh
go test ./...
go build .
```

Requires the `git` binary on PATH (all git operations shell out; worktree
porcelain output is a stable interface).

## Releasing

Releases are automated with [goreleaser](https://goreleaser.com) via GitHub
Actions: push a tag matching `v*` and the workflow runs the tests, builds
darwin/linux binaries for amd64/arm64, and publishes a GitHub release with
the archives. Validate the config locally with
`goreleaser release --snapshot --clean`.

Homebrew distribution (a tap plus a goreleaser-published cask) is planned
but not wired up yet.
