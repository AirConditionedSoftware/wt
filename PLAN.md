# `wt` — a git worktree manager CLI (Go)

## Context

Build a CLI tool called `wt` that makes git worktrees pleasant to use day-to-day:

- `wt list` — show all worktrees for the current repo
- `wt add <branch>` — create a worktree for a branch (existing local, existing remote, or brand new)
- Configuration lives in a JSON file: path taken from the `WT_CONFIG` env var, defaulting to `~/.wt/wt.json`. Worktree placement is **entirely config-driven** and can be overridden per repository.
- Distributed via Homebrew as a single static binary.

**Stack (user-confirmed): Go.** Single static binary with instant startup, goreleaser automates the Homebrew tap, and the tool is mostly "run git, parse output" — a great fit. Git operations shell out to the real `git` binary (never go-git — its worktree support is incomplete, and `git worktree list --porcelain` is a stable scripting interface).

## Project layout

```
wt/
├── main.go                      # calls cmd.Execute()
├── cmd/
│   ├── root.go                  # cobra root command, version
│   ├── list.go                  # wt list [--json]
│   └── add.go                   # wt add <branch> [--base <ref>] [--path <dir>]
├── internal/
│   ├── config/
│   │   ├── config.go            # load/resolve config, path templating
│   │   └── config_test.go
│   └── gitx/
│       ├── git.go               # thin exec wrapper around git
│       ├── worktree.go          # porcelain parser, worktree ops
│       └── worktree_test.go
├── .goreleaser.yaml
├── .github/workflows/release.yml
└── go.mod                       # module github.com/<you>/wt (confirm GitHub username at scaffold time)
```

Dependencies: `spf13/cobra` for the CLI (subcommands, help, shell completions for free). Everything else is stdlib (`encoding/json`, `os/exec`, `text/tabwriter`).

## Configuration (`internal/config`)

Resolution order for the file path: `$WT_CONFIG` if set → `~/.wt/wt.json`. A missing file is **not** an error — built-in defaults apply.

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

- `worktree_dir` is a **template**: `{repo}` = basename of the main worktree's directory, `{branch}` = branch name with `/` sanitized to `-` (so `feature/login` → `feature-login`). `~` expands to the home dir.
- `repos` is keyed by repo name (`{repo}` as defined above); a repo entry overrides any top-level field. Effective config = built-in defaults ← top-level ← repo entry.
- Built-in default when no config exists: `worktree_dir: "~/worktrees/{repo}/{branch}"`, `default_base: ""` (meaning: branch off current HEAD).
- Unknown JSON keys are rejected (`json.Decoder.DisallowUnknownFields`) so typos fail loudly.

## Git layer (`internal/gitx`)

- `Run(dir, args...)` wrapper over `os/exec` that returns stdout / wraps stderr into errors.
- Parse `git worktree list --porcelain` into `[]Worktree{Path, Head, Branch, Bare, Detached, Locked, Prunable}`. The first entry is always the main worktree — that's how `{repo}` is derived, and it works even when `wt` is invoked from inside another worktree.
- Branch inspection via `git rev-parse --verify --quiet refs/heads/<b>` (local) and `refs/remotes/origin/<b>` (remote).

## Commands

**`wt list`** — table via `text/tabwriter`: `BRANCH  PATH  HEAD(short)  [flags]` with a `*` marker on the worktree you're currently in; `--json` flag emits the parsed structs for scripting. Errors clearly if not inside a git repo.

**`wt add <branch>`** — resolve target path from effective config, then:
1. Local branch exists → `git worktree add <path> <branch>`
2. Else remote branch exists → `git worktree add --track -b <branch> <path> origin/<branch>` (fetch first)
3. Else → new branch: `git worktree add -b <branch> <path> <base>` where base = `--base` flag → config `default_base` → current HEAD
- `--path` overrides the config-derived location for a one-off.
- Refuses politely if the branch is already checked out in another worktree (shows its path) or the target dir already exists.
- **stdout discipline:** the final created path is the only thing on stdout; all progress/messages go to stderr — so `cd "$(wt add foo)"` works from a shell alias.

## Testing

- Table-driven unit tests for the porcelain parser (fixtures: normal, detached, bare, locked, prunable entries) and config resolution (env var override, missing file, per-repo merge, template expansion, branch sanitization).
- Integration test in `cmd`: build a scratch repo in `t.TempDir()` with real `git`, then exercise `wt add` (all three branch cases) and `wt list` end-to-end.

## Distribution

- `goreleaser`: builds for `darwin/arm64`, `darwin/amd64`, `linux/arm64`, `linux/amd64`, `CGO_ENABLED=0`, version injected via ldflags; `brews:` section pushes a formula to a `homebrew-tap` repo → users run `brew install <you>/tap/wt`.
- GitHub Actions workflow: on tag push `v*`, run tests then `goreleaser release`. (Needs a `TAP_GITHUB_TOKEN` secret with write access to the tap repo — setup step for you, documented in README.)
- Validate locally with `goreleaser release --snapshot --clean` — no repos or tokens needed for that.

## Build order

1. `git init`, `go mod init`, cobra scaffold, `gitx` porcelain parser + `wt list`
2. Config package (env var, defaults, per-repo merge, templating)
3. `wt add` with the three branch cases
4. Tests (unit + integration), README
5. `.goreleaser.yaml` + release workflow (snapshot-verified locally)

## Verification

- `go test ./...`
- Manual: create a scratch repo, run `wt list`, `wt add` for a new / local / remote branch, confirm placement honors `WT_CONFIG` pointing at a test config with a per-repo override
- `goreleaser release --snapshot --clean` produces installable binaries

## Deliberately deferred (future)

`wt prune`, copying files into new worktrees (e.g. `.env`), post-create hooks (`npm install`), interactive picker. The config schema and command structure leave room for all of these. (`wt remove` was added after the initial build.)
