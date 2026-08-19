# Contributing to wt

Thanks for taking an interest — issues and pull requests are welcome.

## Development setup

Go 1.25+ and git are the only requirements.

```sh
go build .        # produces ./wt
go test ./...     # unit + end-to-end tests
go vet ./...
gofmt -l .        # should print nothing
```

The end-to-end tests (`cmd/e2e_test.go`) build the binary and drive it
against scratch repositories in temp directories — they need a real `git` on
PATH but never touch your own repositories or config.

## Project layout

- `cmd/` — cobra commands (`add`, `list`, `remove`, `open`, `prune`, `du`,
  `config`)
- `internal/gitx/` — thin wrapper that shells out to git and parses
  `git worktree list --porcelain`
- `internal/config/` — wt.json loading, per-repo resolution, path templating

Conventions worth keeping:

- All git operations shell out to the `git` binary; porcelain output is the
  interface. No go-git.
- stdout discipline: `wt add` prints only the created path on stdout so
  `cd "$(wt add x)"` works; everything else goes to stderr.
- Config fields merge in three layers (built-in defaults ← top-level ←
  matching `repos` entry). New settings should follow the existing merge
  pattern — use a pointer type when a per-repo `false` must override a
  global `true`.
- Interactive prompts (huh) need a non-TTY fallback that errors clearly
  instead of hanging.

## Pull requests

- CI runs gofmt, `go vet`, and the tests on Linux and macOS; keep them
  green.
- Every push to `main` automatically publishes a patch release, so changes
  land through PRs and get released on merge.
- Update `README.md` when you add or change commands, flags, or config
  fields — the README is the reference documentation.

## Releases

Handled by GitHub Actions + goreleaser: merges to `main` auto-increment the
patch version; maintainers push a `v*` tag manually for a minor or major
bump. See the Releasing section of the README.
