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
local JSON config (`~/.wt/wt.json` or `$WT_CONFIG`), and creates worktrees,
copies files, and writes `.code-workspace` files where that config points.
Reports of particular interest:

- config- or branch-name-driven writes escaping their intended locations
  (path traversal in `worktree_dir`, `copy_files`, `workspace_paths`, or
  branch names)
- argument injection into the `git` or `code` invocations
- the removal flows deleting anything other than the selected worktree and
  its wt-generated workspace file
- `post_create` executing anything not written verbatim in the user's own
  config: commands come only from `~/.wt/wt.json` / `$WT_CONFIG` (never
  from the repository), and worktree metadata is passed as `WT_*`
  environment variables, never interpolated into the shell command — a
  bypass of either property is a vulnerability
