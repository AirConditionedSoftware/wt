# wt

[![release](https://img.shields.io/github/v/release/AirConditionedSoftware/wt)](https://github.com/AirConditionedSoftware/wt/releases)
[![build](https://github.com/AirConditionedSoftware/wt/actions/workflows/release.yml/badge.svg)](https://github.com/AirConditionedSoftware/wt/actions/workflows/release.yml)
[![license](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A small CLI for managing git worktrees. Worktree placement is driven by a JSON
config file and can be overridden per repository.

```console
$ wt add feature/login
Creating worktree with new branch "feature/login" from main
/Users/you/worktrees/myapp/feature-login

$ wt list
* main           1a2b3c4d
    ~/code/myapp
  feature/login  1a2b3c4d
    ~/worktrees/myapp/feature-login
```

## Install

```sh
brew install AirConditionedSoftware/tap/wt
```

Or grab a binary from
[GitHub Releases](https://github.com/AirConditionedSoftware/wt/releases), or
install with Go:

```sh
go install github.com/AirConditionedSoftware/wt@latest
```

## Usage

- `wt list [--json]` — list all worktrees of the current repository, two
  lines per worktree (branch and short HEAD, path indented below) so long
  paths never blow out the width. The `*` marks the worktree you're in;
  locked and prunable worktrees carry inline tags (and flags in `--json`).
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
  - `--open` opens the new worktree in VS Code (the `.code-workspace` file
    if one was written, the folder otherwise); `--no-open` overrides a
    `vscode_open` config that enables it
  - `--no-post-create` skips the config's `post_create` commands
- `wt remove [branch...] [--force]` (aliases: `wt rm`, `wt -r`) — remove the
  worktrees that have the given branches checked out; the branches themselves
  are kept. Paths work too. With no arguments, an interactive picker lets you
  select one or more worktrees to delete, showing each branch's last commit
  and how long ago it was made. Refuses to remove the main worktree or the
  one you're in. A worktree with modified or untracked files prompts you to
  force or skip it — per worktree, so multi-removals decide each one;
  `--force`/`-f` skips the prompt, and non-interactive use errors and asks
  for `--force` instead of hanging. When `vscode_workspace_file` is enabled,
  the wt-generated `.code-workspace` sibling is deleted along with the
  worktree.
- `wt open [branch]` — open a worktree in VS Code: with no argument an
  interactive picker lists the worktrees, with a branch (or path) it opens
  that one directly. Opens the wt-generated `.code-workspace` file when the
  worktree has one, the folder otherwise. Only available when `vscode_open`
  is enabled for the repo — without it the command tells you what to set.
- `wt prune [--dry-run]` — clean up git's bookkeeping for stale worktrees,
  i.e. entries whose directories were deleted manually (tagged `prunable`
  in `wt list`). Prints what it prunes; branches and existing directories
  are untouched.
- `wt du [--unit KB|MB|GB]` (alias: `wt disk`) — disk space used by each
  worktree, largest first, plus a total. Sizes count the working files (the
  shared repository database in `.git` isn't attributed to any worktree);
  by default each row picks a readable unit, `--unit`/`-u` forces one.
- `wt config` — print the config file location (stderr) and its content
  (stdout, so `wt config | jq` works). Prints the built-in defaults if no
  file exists, and fails loudly if the file is invalid. Run inside a repo
  that has a `.wtrc`, it prints and validates that file too, after the
  global one.
- `wt init [flags]` — create a starter repo-local `.wtrc` at the main
  worktree root (refuses to overwrite an existing one) and print its path,
  so `$EDITOR "$(wt init)"` opens it. Flags pre-fill fields — `--name`,
  `--worktree-dir`, `--base`, `--prefix`, `--separator`, the repeatable
  `--copy-file` and `--post-create`, and the booleans `--copy-hooks`,
  `--open`, `--workspace-file` (written as `true` only when passed; an
  explicit `--copy-hooks=false` writes `false`, which overrides a global
  `true`) — as in
  `wt init --prefix peter --separator - --base develop`. See "Repo-local
  config" below.
- `wt completion` — interactive wizard that sets up shell completion: pick
  your shell (preselected from `$SHELL`), get the line to add to its startup
  file copied to your clipboard, and the steps to finish.
  `wt completion <bash|zsh|fish|powershell>` prints the raw script that the
  installed line sources.

Output is colored when stdout is a terminal: in `wt list`, branch names are
bold (green for the worktree you're in), hashes and paths are dimmed, and
`locked`/`prunable` show as cyan/yellow inline tags — the tags are words,
so they survive piping. `wt du` colors its header and TOTAL row. Disable
with `--no-color` or the [`NO_COLOR`](https://no-color.org) environment
variable; piped output is always plain text.

Paths under your home directory display as `~/...`; show absolute paths with
the global `--full-paths` flag or the `full_paths` config setting.
Machine-facing output — the path `wt add` prints on stdout and everything
`--json` — always uses full paths.

`wt add` prints only the created path on stdout (everything else goes to
stderr), so you can hop straight into a new worktree with a shell function:

```sh
wtcd() { cd "$(wt add "$1")"; }
```

## Configuration

Config lives at `~/.wt/wt.json`, or wherever `$WT_CONFIG` points. A missing
file at the default location just means defaults; a missing file at an
explicit `$WT_CONFIG` path is an error.

The full schema — every option shown, everything optional. All settings can
be set at the top level (applying to every repo) and overridden per repo:

```json
{
  "worktree_dir": "~/worktrees/{repo}/{branch}",
  "default_base": "main",
  "branch_prefix": "peter",
  "prefix_separator": "/",
  "copy_hooks": false,
  "copy_files": [".env*"],
  "vscode_open": false,
  "vscode_workspace_file": false,
  "vscode_workspace_prefix": "ws-",
  "vscode_window_title": "${rootName} — ${activeEditorShort}",
  "workspace_paths": [
    { "name": "notes", "path": "~/notes" }
  ],
  "full_paths": false,
  "post_create": ["direnv allow"],
  "update_check": false,
  "repos": [
    {
      "name": "myapp",
      "path": "~/code/myapp",
      "worktree_dir": "~/code/myapp-trees/{branch}",
      "default_base": "develop",
      "branch_prefix": "team",
      "prefix_separator": "-",
      "copy_hooks": true,
      "copy_files": [".env*", "config/local.json"],
      "vscode_open": true,
      "vscode_workspace_file": true,
      "vscode_workspace_prefix": "acs-",
      "vscode_window_title": "myapp — ${activeEditorShort}${separator}${branchName}",
      "workspace_paths": [
        { "name": "docs", "path": "~/notes/myapp" },
        { "path": "~/code/shared-lib" }
      ],
      "full_paths": true,
      "post_create": ["npm install", "direnv allow"]
    }
  ]
}
```

A repository can also carry a `.wtrc` of its own, accepting the same
settings fields plus `name` — see "Repo-local config" below.

One nuance when overriding: an explicit `false` in a repo entry *does*
override a top-level `true` for the boolean fields (`copy_hooks`,
`vscode_open`, `vscode_workspace_file`, `full_paths`), and `"copy_files": []`
clears an inherited list — but an empty *string* is treated as unset and
falls through.

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
- `vscode_open` — open each new worktree in VS Code after creation (needs
  the `code` CLI on PATH; a missing CLI is a warning, not a failure). Opens
  the `.code-workspace` file when one is written, the folder otherwise.
  `--open` / `--no-open` override per invocation. Also gates the `wt open`
  command.
- `vscode_workspace_file` — write a
  `<vscode_workspace_prefix><branch>.code-workspace` file for each new
  worktree, containing a `folders` array with the worktree path and
  `settings["window.title"]`. The file is created *next to* the worktree
  directory (a sibling, not inside it), so it never shows up as an untracked
  file in git; `wt remove` cleans it up along with the worktree.
- `vscode_workspace_prefix` — prefix for the workspace file's name, e.g.
  `"acs-"` → `acs-fix-login.code-workspace`. Default: none (just the
  sanitized branch name).
- `vscode_window_title` — the `window.title` value written into the
  workspace file, taken **verbatim**, so VS Code title variables like
  `${activeEditorShort}` or `${dirty}` pass straight through. Default: the
  repo name.
- `workspace_paths` — extra folders to include in generated workspace files,
  as `{name, path}` objects (`name` is the display name in VS Code and
  optional; `path` is required, with `~` expanding to your home directory).
  They're appended to the `folders` array after the worktree itself, so the
  workspace spans multiple folders. A repo entry's list replaces the global
  one.
- `full_paths` — show absolute paths in tables, prompts, and messages
  instead of abbreviating your home directory to `~`. Same effect as the
  global `--full-paths` flag. (`wt add`'s stdout path and `--json` output
  always use full paths regardless.)
- `post_create` — shell commands run inside each newly created worktree, in
  order via `sh -c`, after hooks and files are copied and before VS Code
  opens (so `npm install` finishes first). Each command is printed before it
  runs; the first failure stops the rest and reports it, but the worktree
  survives. Skip once with `--no-post-create`. A repo entry's list
  *replaces* the global one; `[]` disables. **Security posture**: these are
  arbitrary commands by design. Commands in this file are user-owned and
  run as written; commands that come from a repo's `.wtrc` run only
  after you approve them (see "Repo-local config" below), so a cloned repo
  can't inject commands silently. Worktree metadata reaches the commands as
  environment variables (`WT_WORKTREE`, `WT_MAIN`, `WT_REPO`, `WT_BRANCH`)
  rather than being interpolated into the command string, so branch names
  containing shell metacharacters are inert. Do note that a command like
  `npm install` executes the checked-out branch's own install scripts — the
  same risk as running it yourself.
- `update_check` — opt in to `wt --version` querying GitHub for the latest
  release and mentioning (on stderr) when a newer one exists. Off by
  default; wt makes no network calls beyond git otherwise. Failures are
  silent — a version query never breaks because the network did. Top-level
  only: this is the one setting a repo's `.wtrc` cannot set, so a cloned
  repository can never switch on network calls.
- `repos` — per-repository overrides, explained below.

Unknown keys are rejected so typos fail loudly.

### Command-line flags with config equivalents

Flags always win over the config for that one invocation:

| flag                             | config field    | notes                                                                                    |
| -------------------------------- | --------------- | ---------------------------------------------------------------------------------------- |
| `wt add --base <ref>`            | `default_base`  | one-off base for the new branch                                                           |
| `wt add --path <dir>`            | `worktree_dir`  | flag is a literal one-off location; the config field is a `{repo}`/`{branch}` template    |
| `wt add --no-prefix`             | `branch_prefix` | skips the configured prefix once                                                          |
| `wt add --copy-hooks` / `--no-copy-hooks` | `copy_hooks` | force hook copying on or off once                                                |
| `wt add --copy-file <glob>` / `--no-copy-files` | `copy_files` | `--copy-file` (repeatable) adds one-off entries; `--no-copy-files` skips the config list |
| `wt add --open` / `--no-open`    | `vscode_open`   | force opening in VS Code on or off once                                                   |
| `wt add --no-post-create`        | `post_create`   | skip the configured commands once                                                          |
| `--full-paths` (any command)     | `full_paths`    | show absolute paths instead of `~`                                                        |

`--no-color` and `wt remove --force` are flag-only; `vscode_workspace_file`,
`vscode_workspace_prefix`, `vscode_window_title`, `workspace_paths`, and
`prefix_separator` are config-only.

### Repo-local config (`.wtrc`)

A repository can keep its own config in a `.wtrc` at the root of its
**main worktree**. It is never read from a linked worktree, but it applies
whenever wt runs from any of the repo's worktrees — wt finds the main
worktree through git. No file means nothing changes. `wt init` scaffolds
one (writing to the main worktree root even when run from a linked
worktree) and prints its path, so `$EDITOR "$(wt init)"` opens it directly.
Its flags pre-fill fields, e.g.
`wt init --prefix peter --separator - --base develop`.

It holds the same settings fields as the global config, plus `name` (what
`{repo}` expands to). `repos` and `path` are rejected, because the file
already *is* repo-specific; unknown keys fail loudly as usual, and a broken
`.wtrc` is an error like a broken global config. Since it lives in the
repo, it can be committed and shared with a team:

```json
{
  "name": "myapp",
  "branch_prefix": "team",
  "prefix_separator": "-",
  "vscode_workspace_file": true,
  "post_create": ["npm ci"]
}
```

A ready-made starting point lives at [examples/.wtrc](examples/.wtrc) — copy
it to the repo root (`cp examples/.wtrc .wtrc`) and trim what you don't need.

`.wtrc` is the **top layer**: its values override both the global
top-level fields and the repo's global `repos` entry, field by field, with
the same merge rules as everywhere else — an empty string falls through to
the layer below, a list replaces the list below it, and an explicit `false`
overrides an inherited `true`.

Because a committed `.wtrc` arrives with the repository, `post_create`
commands that come from it run only after you approve them. `wt add`
prompts the first time, showing the commands; approvals are remembered in
`~/.wt/trust.json`, keyed by the repo's main worktree. Any later change to
the repo's `post_create` — including one that arrives with a `git pull` —
prompts again, showing a diff of the approved commands against the new
ones:

```
post_create in ~/code/myapp/.wtrc changed:

    direnv allow
  - npm install
  + npm ci

Allow these commands to run after wt add? [Allow and remember / Skip this time]
```

Declining skips `post_create` for that run and nothing else — the worktree
is still created — and records nothing, so the next run asks again.
Non-interactive runs can't ask, so they skip unapproved commands with a
warning and carry on. `post_create` in the global config is user-owned and
never prompts.

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
  `branch_prefix`, `prefix_separator`, `copy_hooks`, `copy_files`,
  `workspace_paths`, `full_paths`, `post_create`, and the `vscode_*`
  fields.

Settings resolve in four layers, field by field:

1. built-in defaults, overlaid with
2. the top-level fields in wt.json, overlaid with
3. the `repos` entry whose `path` matches, overlaid with
4. the repo's own `.wtrc`, if it has one.

A layer only needs the fields it wants to change; anything it omits falls
through to the layer below. With the example config above — and the
`.wtrc` from the previous section sitting in `~/code/myapp` — running
`wt add fix-login` inside `~/code/myapp` (or any of its worktrees) resolves
to:

| field              | value                         | comes from |
| ------------------ | ----------------------------- | ---------- |
| `worktree_dir`     | `~/code/myapp-trees/{branch}` | repo entry |
| `default_base`     | `develop`                     | repo entry |
| `branch_prefix`    | `team`                        | `.wtrc` |
| `prefix_separator` | `-`                           | `.wtrc` |
| `post_create`      | `["npm ci"]`                  | `.wtrc` |

`.wtrc` has the last word on the bottom three rows: it repeats the
entry's `branch_prefix` and `prefix_separator`, and its `post_create`
replaces the entry's `["npm install", "direnv allow"]` — a repo-sourced
list, so it needs approval before it runs. The created branch is
`team-fix-login` (and the copy and `vscode_*` behaviors come from the repo
entry too), while any other repo gets `~/worktrees/{repo}/{branch}`,
`main`, and `peter` with the default `/` separator (branches like
`peter/fix-login`) from the top level. Note that an empty string in a repo
entry does not clear an inherited value — it's treated the same as omitting
the field (use `--no-prefix` to skip a prefix per invocation).

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
Actions. Every push to `main` runs the tests and, when code shipped in the
binaries has changed since the last release (`*.go`, `go.mod`, `go.sum`, or
`.goreleaser.yaml` — docs-only pushes don't release), bumps the patch
version from the latest `v*` tag (`v0.2.0` → `v0.2.1`), pushes the new tag,
and publishes a GitHub release with darwin/linux binaries for amd64/arm64. For a minor or
major bump, push a tag yourself (`git tag v0.3.0 && git push origin main
v0.3.0`) — the workflow releases exactly that version and later `main`
pushes continue from it. Validate the goreleaser config locally with
`goreleaser release --snapshot --clean`.

Each release also updates the Homebrew cask in
[AirConditionedSoftware/homebrew-tap](https://github.com/AirConditionedSoftware/homebrew-tap),
pushed by goreleaser using the `TAP_GITHUB_TOKEN` repository secret — a
token with write access to the tap repository.

## Contributing

Issues and pull requests are welcome — see
[CONTRIBUTING.md](CONTRIBUTING.md) for the project layout and conventions.
`go test ./...` runs the unit and end-to-end tests (a real `git` on PATH is
all they need). Keep in mind that every push to `main` publishes a release,
so changes should land through pull requests.

## AI assistance

wt was built in collaboration with Claude (Anthropic's Claude Code): the
design and feature decisions are human, most of the code was written by the
model, and everything is human-reviewed, covered by the test suite, and
verified end-to-end before release. Commits carry `Co-Authored-By` trailers
reflecting this.

## License

[MIT](LICENSE)
