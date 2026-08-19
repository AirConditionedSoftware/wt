# Security Policy

## Supported versions

Only the latest release of wt receives security fixes. Releases are cut
automatically from `main`, so fixes ship quickly once merged.

## Reporting a vulnerability

Please report vulnerabilities privately through GitHub's vulnerability
reporting at
<https://github.com/AirConditionedSoftware/wt/security/advisories/new> —
do not open a public issue. You should hear back within a week.

## Scope notes

wt is a local developer tool: it shells out to the `git` binary, reads a
local JSON config (`~/.wt/wt.json` or `$WT_CONFIG`, plus an optional
`.wtrc` in a repo's main worktree), and creates worktrees,
copies files, and writes `.code-workspace` files where that config points.
Reports of particular interest:

- config- or branch-name-driven writes escaping their intended locations
  (path traversal in `worktree_dir`, `copy_files`, `workspace_paths`, or
  branch names)
- argument injection into the `git` or `code` invocations
- the removal flows deleting anything other than the selected worktree and
  its wt-generated workspace file
- `post_create` executing commands the user never approved. Commands come
  either from the user's own config file (`~/.wt/wt.json` / `$WT_CONFIG`)
  or from a repository's `.wtrc`; repo-sourced commands run only after
  explicit interactive approval recorded in `~/.wt/trust.json`, and any
  change to those commands invalidates the approval and re-prompts with a
  diff. Running unapproved repo-sourced commands, or bypassing the change
  detection (e.g. an equivalent-but-differently-encoded command list), is
  a vulnerability. Worktree metadata is passed as `WT_*` environment
  variables, never interpolated into the shell command — a bypass of that
  property is a vulnerability too
