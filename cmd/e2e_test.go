package cmd_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var wtBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "wt-e2e")
	if err != nil {
		panic(err)
	}
	wtBin = filepath.Join(dir, "wt")
	root, err := filepath.Abs("..")
	if err != nil {
		panic(err)
	}
	build := exec.Command("go", "build", "-o", wtBin, ".")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		panic("building wt: " + err.Error() + "\n" + string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// gitEnv isolates git from the developer's real global/system config.
func gitEnv(home string) []string {
	return append(os.Environ(),
		"HOME="+home,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=wt-test",
		"GIT_AUTHOR_EMAIL=wt@test.invalid",
		"GIT_COMMITTER_NAME=wt-test",
		"GIT_COMMITTER_EMAIL=wt@test.invalid",
	)
}

func git(t *testing.T, home, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv(home)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func wt(t *testing.T, home, cfg, dir string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	return wtWithEnv(t, home, cfg, dir, nil, args...)
}

func wtWithEnv(t *testing.T, home, cfg, dir string, extraEnv []string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := exec.Command(wtBin, args...)
	cmd.Dir = dir
	cmd.Env = append(append(gitEnv(home), "WT_CONFIG="+cfg), extraEnv...)
	var so, se strings.Builder
	cmd.Stdout = &so
	cmd.Stderr = &se
	err = cmd.Run()
	return strings.TrimSpace(so.String()), se.String(), err
}

func TestEndToEnd(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()

	// A source repo that will act as origin, with a branch that only exists
	// there.
	origin := filepath.Join(work, "origin-src")
	if err := os.MkdirAll(origin, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, home, origin, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(origin, "file.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, home, origin, "add", ".")
	git(t, home, origin, "commit", "-m", "init")
	git(t, home, origin, "branch", "remote-only")

	git(t, home, work, "clone", origin, "myapp")
	repo := filepath.Join(work, "myapp")

	trees := filepath.Join(work, "trees")
	cfg := filepath.Join(work, "wt.json")
	cfgJSON := `{
  "worktree_dir": "` + trees + `/{repo}/{branch}",
  "repos": [{"name": "myapp", "path": "` + repo + `", "default_base": "main"}]
}`
	if err := os.WriteFile(cfg, []byte(cfgJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("add new branch", func(t *testing.T) {
		out, _, err := wt(t, home, cfg, repo, "add", "feature/login")
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(trees, "myapp", "feature-login")
		if out != want {
			t.Errorf("stdout = %q; want %q", out, want)
		}
		if got := git(t, home, want, "rev-parse", "--abbrev-ref", "HEAD"); got != "feature/login" {
			t.Errorf("checked-out branch = %q; want feature/login", got)
		}
	})

	t.Run("add existing local branch", func(t *testing.T) {
		git(t, home, repo, "branch", "local-b")
		out, _, err := wt(t, home, cfg, repo, "add", "local-b")
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(trees, "myapp", "local-b")
		if out != want {
			t.Errorf("stdout = %q; want %q", out, want)
		}
	})

	t.Run("add remote branch tracks origin", func(t *testing.T) {
		out, _, err := wt(t, home, cfg, repo, "add", "remote-only")
		if err != nil {
			t.Fatal(err)
		}
		upstream := git(t, home, out, "rev-parse", "--abbrev-ref", "remote-only@{upstream}")
		if upstream != "origin/remote-only" {
			t.Errorf("upstream = %q; want origin/remote-only", upstream)
		}
	})

	t.Run("add already checked out branch fails", func(t *testing.T) {
		_, stderr, err := wt(t, home, cfg, repo, "add", "main")
		if err == nil {
			t.Fatal("expected error for already checked out branch")
		}
		if !strings.Contains(stderr, "already checked out") {
			t.Errorf("stderr = %q; want mention of already checked out", stderr)
		}
	})

	t.Run("list", func(t *testing.T) {
		out, _, err := wt(t, home, cfg, repo, "list")
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"main", "feature/login", "local-b", "remote-only"} {
			if !strings.Contains(out, want) {
				t.Errorf("list output missing %q:\n%s", want, out)
			}
		}
		if strings.Contains(out, "\x1b") {
			t.Errorf("list output contains ANSI escapes without a terminal:\n%q", out)
		}
	})

	t.Run("list json", func(t *testing.T) {
		out, _, err := wt(t, home, cfg, repo, "list", "--json")
		if err != nil {
			t.Fatal(err)
		}
		var wts []map[string]any
		if err := json.Unmarshal([]byte(out), &wts); err != nil {
			t.Fatalf("list --json produced invalid JSON: %v\n%s", err, out)
		}
		if len(wts) != 4 {
			t.Errorf("list --json returned %d worktrees; want 4", len(wts))
		}
	})

	t.Run("du lists sizes per worktree", func(t *testing.T) {
		out, _, err := wt(t, home, cfg, repo, "du")
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"BRANCH", "main", "TOTAL"} {
			if !strings.Contains(out, want) {
				t.Errorf("du output missing %q:\n%s", want, out)
			}
		}
		out2, _, err := wt(t, home, cfg, repo, "du", "--unit", "KB")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out2, "KB") {
			t.Errorf("du --unit KB output has no KB sizes:\n%s", out2)
		}
		if _, _, err := wt(t, home, cfg, repo, "du", "--unit", "TB"); err == nil {
			t.Error("du with invalid unit should fail")
		}
	})

	t.Run("list outside a repo fails", func(t *testing.T) {
		if _, _, err := wt(t, home, cfg, t.TempDir(), "list"); err == nil {
			t.Error("expected error outside a git repository")
		}
	})

	t.Run("remove worktree keeps branch", func(t *testing.T) {
		out, stderr, err := wt(t, home, cfg, repo, "remove", "local-b")
		if err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		if out != "" {
			t.Errorf("stdout = %q; want empty", out)
		}
		path := filepath.Join(trees, "myapp", "local-b")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("worktree dir %s still exists", path)
		}
		git(t, home, repo, "rev-parse", "--verify", "refs/heads/local-b")
	})

	t.Run("remove via -r flag", func(t *testing.T) {
		if _, stderr, err := wt(t, home, cfg, repo, "-r", "remote-only"); err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		path := filepath.Join(trees, "myapp", "remote-only")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("worktree dir %s still exists", path)
		}
	})

	t.Run("remove main worktree fails", func(t *testing.T) {
		_, stderr, err := wt(t, home, cfg, repo, "remove", "main")
		if err == nil {
			t.Fatal("expected error removing the main worktree")
		}
		if !strings.Contains(stderr, "main worktree") {
			t.Errorf("stderr = %q; want mention of main worktree", stderr)
		}
	})

	t.Run("remove worktree you are in fails", func(t *testing.T) {
		inside := filepath.Join(trees, "myapp", "feature-login")
		_, stderr, err := wt(t, home, cfg, inside, "remove", "feature/login")
		if err == nil {
			t.Fatal("expected error removing the current worktree")
		}
		if !strings.Contains(stderr, "worktree you are in") {
			t.Errorf("stderr = %q; want mention of current worktree", stderr)
		}
	})

	t.Run("remove unknown branch fails", func(t *testing.T) {
		_, stderr, err := wt(t, home, cfg, repo, "remove", "no-such-branch")
		if err == nil {
			t.Fatal("expected error for unknown worktree")
		}
		if !strings.Contains(stderr, "no worktree found") {
			t.Errorf("stderr = %q; want no-worktree-found error", stderr)
		}
	})

	t.Run("remove multiple branches at once", func(t *testing.T) {
		for _, b := range []string{"multi-a", "multi-b"} {
			if _, _, err := wt(t, home, cfg, repo, "add", b); err != nil {
				t.Fatal(err)
			}
		}
		if _, stderr, err := wt(t, home, cfg, repo, "remove", "multi-a", "multi-b"); err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		for _, b := range []string{"multi-a", "multi-b"} {
			path := filepath.Join(trees, "myapp", b)
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("worktree dir %s still exists", path)
			}
		}
	})

	t.Run("remove with no args needs a terminal", func(t *testing.T) {
		_, stderr, err := wt(t, home, cfg, repo, "remove")
		if err == nil {
			t.Fatal("expected error for interactive remove without a terminal")
		}
		if !strings.Contains(stderr, "terminal") {
			t.Errorf("stderr = %q; want mention of terminal", stderr)
		}
	})

	t.Run("remove dirty worktree needs force", func(t *testing.T) {
		out, _, err := wt(t, home, cfg, repo, "add", "dirty")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, "untracked.txt"), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, stderr, err := wt(t, home, cfg, repo, "remove", "dirty")
		if err == nil {
			t.Fatal("expected error removing a dirty worktree without --force")
		}
		if !strings.Contains(stderr, "modified or untracked") || !strings.Contains(stderr, "--force") {
			t.Errorf("stderr = %q; want dirty-worktree message pointing at --force", stderr)
		}
		if _, stderr, err := wt(t, home, cfg, repo, "remove", "--force", "dirty"); err != nil {
			t.Fatalf("remove --force: %v\n%s", err, stderr)
		}
	})

	cfgPrefix := filepath.Join(work, "wt-prefix.json")
	cfgPrefixJSON := `{
  "worktree_dir": "` + trees + `/{repo}/{branch}",
  "branch_prefix": "peter/"
}`
	if err := os.WriteFile(cfgPrefix, []byte(cfgPrefixJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("add applies configured branch prefix", func(t *testing.T) {
		out, _, err := wt(t, home, cfgPrefix, repo, "add", "widget")
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(trees, "myapp", "peter-widget")
		if out != want {
			t.Errorf("stdout = %q; want %q", out, want)
		}
		if got := git(t, home, out, "rev-parse", "--abbrev-ref", "HEAD"); got != "peter/widget" {
			t.Errorf("checked-out branch = %q; want peter/widget", got)
		}
	})

	t.Run("add finds branch created with prefix earlier", func(t *testing.T) {
		if _, stderr, err := wt(t, home, cfgPrefix, repo, "remove", "peter/widget"); err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		_, stderr, err := wt(t, home, cfgPrefix, repo, "add", "widget")
		if err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		if !strings.Contains(stderr, "existing branch") {
			t.Errorf("stderr = %q; want reuse of existing branch", stderr)
		}
		path := filepath.Join(trees, "myapp", "peter-widget")
		if got := git(t, home, path, "rev-parse", "--abbrev-ref", "HEAD"); got != "peter/widget" {
			t.Errorf("checked-out branch = %q; want peter/widget", got)
		}
	})

	t.Run("add --no-prefix skips prefix", func(t *testing.T) {
		out, _, err := wt(t, home, cfgPrefix, repo, "add", "--no-prefix", "plain")
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(trees, "myapp", "plain")
		if out != want {
			t.Errorf("stdout = %q; want %q", out, want)
		}
		if got := git(t, home, out, "rev-parse", "--abbrev-ref", "HEAD"); got != "plain" {
			t.Errorf("checked-out branch = %q; want plain", got)
		}
	})

	t.Run("prefix not applied to existing branch", func(t *testing.T) {
		git(t, home, repo, "branch", "preexisting")
		out, _, err := wt(t, home, cfgPrefix, repo, "add", "preexisting")
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(trees, "myapp", "preexisting")
		if out != want {
			t.Errorf("stdout = %q; want %q", out, want)
		}
		if got := git(t, home, out, "rev-parse", "--abbrev-ref", "HEAD"); got != "preexisting" {
			t.Errorf("checked-out branch = %q; want preexisting", got)
		}
	})

	t.Run("config prints location and content", func(t *testing.T) {
		out, stderr, err := wt(t, home, cfgPrefix, repo, "config")
		if err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		if !strings.Contains(stderr, cfgPrefix) || !strings.Contains(stderr, "WT_CONFIG") {
			t.Errorf("stderr = %q; want config path and $WT_CONFIG mention", stderr)
		}
		if out != strings.TrimSpace(cfgPrefixJSON) {
			t.Errorf("stdout = %q; want file content %q", out, cfgPrefixJSON)
		}
	})

	t.Run("config without a file prints defaults", func(t *testing.T) {
		out, stderr, err := wt(t, home, "", repo, "config")
		if err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		if !strings.Contains(stderr, "built-in defaults") {
			t.Errorf("stderr = %q; want built-in defaults notice", stderr)
		}
		var def map[string]any
		if err := json.Unmarshal([]byte(out), &def); err != nil {
			t.Fatalf("stdout is not JSON: %v\n%s", err, out)
		}
		if def["worktree_dir"] != "~/worktrees/{repo}/{branch}" {
			t.Errorf("defaults = %v; want default worktree_dir", def)
		}
	})

	t.Run("config with invalid file fails after printing it", func(t *testing.T) {
		bad := filepath.Join(work, "bad.json")
		if err := os.WriteFile(bad, []byte(`{"worktre_dir": "/x"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		out, _, err := wt(t, home, bad, repo, "config")
		if err == nil {
			t.Fatal("expected error for invalid config")
		}
		if !strings.Contains(out, "worktre_dir") {
			t.Errorf("stdout = %q; want the file content printed anyway", out)
		}
	})

	t.Run("bare prefix gets default slash separator", func(t *testing.T) {
		cfgBare := filepath.Join(work, "wt-bare-prefix.json")
		cfgJSON := `{
  "worktree_dir": "` + trees + `/{repo}/{branch}",
  "branch_prefix": "peter"
}`
		if err := os.WriteFile(cfgBare, []byte(cfgJSON), 0o644); err != nil {
			t.Fatal(err)
		}
		out, _, err := wt(t, home, cfgBare, repo, "add", "doohickey")
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(trees, "myapp", "peter-doohickey")
		if out != want {
			t.Errorf("stdout = %q; want %q", out, want)
		}
		if got := git(t, home, out, "rev-parse", "--abbrev-ref", "HEAD"); got != "peter/doohickey" {
			t.Errorf("checked-out branch = %q; want peter/doohickey", got)
		}
	})

	t.Run("custom prefix separator per repo", func(t *testing.T) {
		cfgSep := filepath.Join(work, "wt-sep.json")
		cfgJSON := `{
  "worktree_dir": "` + trees + `/{repo}/{branch}",
  "branch_prefix": "peter",
  "repos": [{"path": "` + repo + `", "prefix_separator": "-"}]
}`
		if err := os.WriteFile(cfgSep, []byte(cfgJSON), 0o644); err != nil {
			t.Fatal(err)
		}
		out, _, err := wt(t, home, cfgSep, repo, "add", "gizmo")
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(trees, "myapp", "peter-gizmo")
		if out != want {
			t.Errorf("stdout = %q; want %q", out, want)
		}
		if got := git(t, home, out, "rev-parse", "--abbrev-ref", "HEAD"); got != "peter-gizmo" {
			t.Errorf("checked-out branch = %q; want peter-gizmo", got)
		}
	})

	t.Run("repo name overrides {repo} in templates", func(t *testing.T) {
		cfgName := filepath.Join(work, "wt-name.json")
		cfgJSON := `{
  "worktree_dir": "` + trees + `/{repo}/{branch}",
  "repos": [{"name": "renamed", "path": "` + repo + `"}]
}`
		if err := os.WriteFile(cfgName, []byte(cfgJSON), 0o644); err != nil {
			t.Fatal(err)
		}
		out, _, err := wt(t, home, cfgName, repo, "add", "nametest")
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(trees, "renamed", "nametest")
		if out != want {
			t.Errorf("stdout = %q; want %q", out, want)
		}
	})

	t.Run("copy hooks with relative hooksPath", func(t *testing.T) {
		hooksDir := filepath.Join(repo, ".githooks")
		if err := os.MkdirAll(hooksDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(hooksDir, "pre-commit"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		git(t, home, repo, "config", "core.hooksPath", ".githooks")

		out, stderr, err := wt(t, home, cfgPrefix, repo, "add", "--copy-hooks", "hooked")
		if err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		info, err := os.Stat(filepath.Join(out, ".githooks", "pre-commit"))
		if err != nil {
			t.Fatalf("hook not copied into worktree: %v", err)
		}
		if info.Mode().Perm()&0o100 == 0 {
			t.Errorf("copied hook lost its executable bit: %v", info.Mode())
		}
	})

	t.Run("copy_hooks config and --no-copy-hooks override", func(t *testing.T) {
		cfgHooks := filepath.Join(work, "wt-hooks.json")
		cfgJSON := `{
  "worktree_dir": "` + trees + `/{repo}/{branch}",
  "repos": [{"path": "` + repo + `", "copy_hooks": true}]
}`
		if err := os.WriteFile(cfgHooks, []byte(cfgJSON), 0o644); err != nil {
			t.Fatal(err)
		}
		out, _, err := wt(t, home, cfgHooks, repo, "add", "hooked-cfg")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(out, ".githooks", "pre-commit")); err != nil {
			t.Errorf("copy_hooks config did not copy hooks: %v", err)
		}
		out2, _, err := wt(t, home, cfgHooks, repo, "add", "--no-copy-hooks", "hooked-off")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(out2, ".githooks")); !os.IsNotExist(err) {
			t.Error("--no-copy-hooks should have prevented the copy")
		}
	})

	t.Run("copy_files config copies untracked files", func(t *testing.T) {
		for name, content := range map[string]string{
			".env":       "SECRET=1\n",
			".env.local": "LOCAL=1\n",
		} {
			if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.MkdirAll(filepath.Join(repo, "local-conf"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, "local-conf", "dev.json"), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		cfgFiles := filepath.Join(work, "wt-files.json")
		cfgJSON := `{
  "worktree_dir": "` + trees + `/{repo}/{branch}",
  "repos": [{"path": "` + repo + `", "copy_files": [".env*", "local-conf"]}]
}`
		if err := os.WriteFile(cfgFiles, []byte(cfgJSON), 0o644); err != nil {
			t.Fatal(err)
		}

		out, stderr, err := wt(t, home, cfgFiles, repo, "add", "with-files")
		if err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		for _, f := range []string{".env", ".env.local", "local-conf/dev.json"} {
			if _, err := os.Stat(filepath.Join(out, f)); err != nil {
				t.Errorf("%s not copied: %v", f, err)
			}
		}

		out2, _, err := wt(t, home, cfgFiles, repo, "add", "--no-copy-files", "without-files")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(out2, ".env")); !os.IsNotExist(err) {
			t.Error("--no-copy-files should have prevented the copy")
		}
	})

	t.Run("copy-file flag without config", func(t *testing.T) {
		out, _, err := wt(t, home, cfgPrefix, repo, "add", "--copy-file", ".env", "flag-files")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(out, ".env")); err != nil {
			t.Errorf(".env not copied via --copy-file: %v", err)
		}
		if _, err := os.Stat(filepath.Join(out, ".env.local")); !os.IsNotExist(err) {
			t.Error(".env.local should not have been copied")
		}
	})

	t.Run("copy hooks with shared default hooks", func(t *testing.T) {
		git(t, home, repo, "config", "--unset", "core.hooksPath")
		_, stderr, err := wt(t, home, cfgPrefix, repo, "add", "--copy-hooks", "hooked-shared")
		if err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		if !strings.Contains(stderr, "shared") {
			t.Errorf("stderr = %q; want note that hooks are already shared", stderr)
		}
	})

	t.Run("vscode workspace file with custom prefix and title", func(t *testing.T) {
		cfgVS := filepath.Join(work, "wt-vscode.json")
		cfgJSON := `{
  "worktree_dir": "` + trees + `/{repo}/{branch}",
  "repos": [{"path": "` + repo + `", "vscode_workspace_file": true, "vscode_workspace_prefix": "acs-", "vscode_window_title": "myapp — ${activeEditorShort}",
    "workspace_paths": [{"name": "docs", "path": "~/notes"}, {"path": "/shared/lib"}]}]
}`
		if err := os.WriteFile(cfgVS, []byte(cfgJSON), 0o644); err != nil {
			t.Fatal(err)
		}
		out, stderr, err := wt(t, home, cfgVS, repo, "add", "ws-test")
		if err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		wsPath := filepath.Join(filepath.Dir(out), "acs-ws-test.code-workspace")
		data, err := os.ReadFile(wsPath)
		if err != nil {
			t.Fatalf("workspace file not written: %v", err)
		}
		var ws struct {
			Folders []struct {
				Name string `json:"name"`
				Path string `json:"path"`
			} `json:"folders"`
			Settings map[string]string `json:"settings"`
		}
		if err := json.Unmarshal(data, &ws); err != nil {
			t.Fatalf("workspace file is not valid JSON: %v\n%s", err, data)
		}
		if len(ws.Folders) != 3 || ws.Folders[0].Path != out {
			t.Fatalf("folders = %+v; want worktree plus two workspace_paths entries", ws.Folders)
		}
		if ws.Folders[1].Name != "docs" || ws.Folders[1].Path != filepath.Join(home, "notes") {
			t.Errorf("folders[1] = %+v; want name docs and ~ expanded to %s/notes", ws.Folders[1], home)
		}
		if ws.Folders[2].Name != "" || ws.Folders[2].Path != "/shared/lib" {
			t.Errorf("folders[2] = %+v; want nameless /shared/lib passed through", ws.Folders[2])
		}
		if got := ws.Settings["window.title"]; got != "myapp — ${activeEditorShort}" {
			t.Errorf("window.title = %q; want the configured value verbatim", got)
		}
		if _, err := os.Stat(filepath.Join(out, "acs-ws-test.code-workspace")); !os.IsNotExist(err) {
			t.Error("workspace file must not be inside the worktree")
		}

		if _, stderr, err := wt(t, home, cfgVS, repo, "remove", "ws-test"); err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		if _, err := os.Stat(wsPath); !os.IsNotExist(err) {
			t.Error("sibling workspace file should be cleaned up with the worktree")
		}
	})

	t.Run("vscode workspace title defaults to repo name", func(t *testing.T) {
		cfgVS := filepath.Join(work, "wt-vscode-default.json")
		cfgJSON := `{
  "worktree_dir": "` + trees + `/{repo}/{branch}",
  "repos": [{"path": "` + repo + `", "vscode_workspace_file": true}]
}`
		if err := os.WriteFile(cfgVS, []byte(cfgJSON), 0o644); err != nil {
			t.Fatal(err)
		}
		out, _, err := wt(t, home, cfgVS, repo, "add", "ws-default")
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(filepath.Dir(out), "ws-default.code-workspace"))
		if err != nil {
			t.Fatalf("workspace file not written: %v", err)
		}
		var ws struct {
			Settings map[string]string `json:"settings"`
		}
		if err := json.Unmarshal(data, &ws); err != nil {
			t.Fatal(err)
		}
		if got := ws.Settings["window.title"]; got != "myapp" {
			t.Errorf("window.title = %q; want repo name \"myapp\"", got)
		}
	})

	t.Run("open requires vscode_open", func(t *testing.T) {
		_, stderr, err := wt(t, home, cfg, repo, "open", "main")
		if err == nil {
			t.Fatal("expected error when vscode_open is not enabled")
		}
		if !strings.Contains(stderr, "vscode_open") {
			t.Errorf("stderr = %q; want note about enabling vscode_open", stderr)
		}
	})

	t.Run("open launches code on the right target", func(t *testing.T) {
		stub := t.TempDir()
		codeLog := filepath.Join(stub, "code.log")
		script := "#!/bin/sh\necho \"$@\" > " + codeLog + "\n"
		if err := os.WriteFile(filepath.Join(stub, "code"), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		stubPath := []string{"PATH=" + stub + string(os.PathListSeparator) + os.Getenv("PATH")}

		cfgOpen := filepath.Join(work, "wt-open.json")
		cfgJSON := `{
  "worktree_dir": "` + trees + `/{repo}/{branch}",
  "repos": [{"path": "` + repo + `", "vscode_open": true, "vscode_workspace_file": true, "vscode_workspace_prefix": "acs-"}]
}`
		if err := os.WriteFile(cfgOpen, []byte(cfgJSON), 0o644); err != nil {
			t.Fatal(err)
		}

		// git reports symlink-resolved paths (/private/var vs /var on
		// macOS), so compare resolved forms.
		resolve := func(p string) string {
			if r, err := filepath.EvalSymlinks(p); err == nil {
				return r
			}
			return p
		}

		// A worktree without a workspace file opens as a folder.
		_, stderr, err := wtWithEnv(t, home, cfgOpen, repo, stubPath, "open", "feature/login")
		if err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		logged, err := os.ReadFile(codeLog)
		if err != nil {
			t.Fatalf("stub code was not invoked: %v", err)
		}
		wantFolder := filepath.Join(trees, "myapp", "feature-login")
		if got := strings.TrimSpace(string(logged)); resolve(got) != resolve(wantFolder) {
			t.Errorf("code opened %q; want the folder %q", got, wantFolder)
		}

		// A worktree with a wt-generated workspace file opens that file.
		out, _, err := wtWithEnv(t, home, cfgOpen, repo, stubPath, "add", "--no-open", "open-ws")
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := wtWithEnv(t, home, cfgOpen, repo, stubPath, "open", "open-ws"); err != nil {
			t.Fatal(err)
		}
		logged, err = os.ReadFile(codeLog)
		if err != nil {
			t.Fatal(err)
		}
		wantWS := filepath.Join(filepath.Dir(out), "acs-open-ws.code-workspace")
		if got := strings.TrimSpace(string(logged)); resolve(got) != resolve(wantWS) {
			t.Errorf("code opened %q; want the workspace file %q", got, wantWS)
		}
	})

	t.Run("full paths flag and config", func(t *testing.T) {
		inHome := filepath.Join(home, "trees", "homed")
		out, _, err := wt(t, home, cfg, repo, "add", "--path", inHome, "homed")
		if err != nil {
			t.Fatal(err)
		}
		if out != inHome {
			t.Errorf("wt add stdout = %q; want absolute %q", out, inHome)
		}

		l, _, err := wt(t, home, cfg, repo, "list")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(l, "~/trees/homed") {
			t.Errorf("list should abbreviate the home directory:\n%s", l)
		}

		lf, _, err := wt(t, home, cfg, repo, "list", "--full-paths")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(lf, "~/trees/homed") {
			t.Errorf("list --full-paths should not abbreviate:\n%s", lf)
		}

		cfgFull := filepath.Join(work, "wt-fullpaths.json")
		cfgJSON := `{
  "worktree_dir": "` + trees + `/{repo}/{branch}",
  "repos": [{"path": "` + repo + `", "full_paths": true}]
}`
		if err := os.WriteFile(cfgFull, []byte(cfgJSON), 0o644); err != nil {
			t.Fatal(err)
		}
		lc, _, err := wt(t, home, cfgFull, repo, "list")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(lc, "~/trees/homed") {
			t.Errorf("list with full_paths config should not abbreviate:\n%s", lc)
		}

		if _, _, err := wt(t, home, cfg, repo, "remove", "homed"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("completion", func(t *testing.T) {
		out, _, err := wt(t, home, cfg, repo, "completion", "zsh")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "#compdef") {
			t.Errorf("zsh completion script missing #compdef header:\n%.120s", out)
		}
		_, stderr, err := wt(t, home, cfg, repo, "completion")
		if err == nil {
			t.Error("wizard without a terminal should fail")
		} else if !strings.Contains(stderr, "terminal") {
			t.Errorf("stderr = %q; want terminal hint", stderr)
		}
		if _, _, err := wt(t, home, cfg, repo, "completion", "tcsh"); err == nil {
			t.Error("unsupported shell should fail")
		}
	})

	t.Run("post_create runs commands in the new worktree", func(t *testing.T) {
		cfgPC := filepath.Join(work, "wt-postcreate.json")
		cfgJSON := `{
  "worktree_dir": "` + trees + `/{repo}/{branch}",
  "repos": [{"path": "` + repo + `", "post_create": [
    "echo ran > marker.txt",
    "printf %s \"$WT_BRANCH\" > branch.txt",
    "printf %s \"$WT_REPO\" > repo.txt"
  ]}]
}`
		if err := os.WriteFile(cfgPC, []byte(cfgJSON), 0o644); err != nil {
			t.Fatal(err)
		}
		out, stderr, err := wt(t, home, cfgPC, repo, "add", "pc-test")
		if err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		if _, err := os.Stat(filepath.Join(out, "marker.txt")); err != nil {
			t.Errorf("post_create did not run in the worktree: %v", err)
		}
		if b, _ := os.ReadFile(filepath.Join(out, "branch.txt")); string(b) != "pc-test" {
			t.Errorf("WT_BRANCH = %q; want pc-test", b)
		}
		if b, _ := os.ReadFile(filepath.Join(out, "repo.txt")); string(b) != "myapp" {
			t.Errorf("WT_REPO = %q; want myapp", b)
		}

		out2, _, err := wt(t, home, cfgPC, repo, "add", "--no-post-create", "pc-skip")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(out2, "marker.txt")); !os.IsNotExist(err) {
			t.Error("--no-post-create should have skipped the commands")
		}
	})

	t.Run("post_create is safe against shell metacharacters in branch names", func(t *testing.T) {
		cfgPC := filepath.Join(work, "wt-postcreate-inj.json")
		cfgJSON := `{
  "worktree_dir": "` + trees + `/{repo}/{branch}",
  "repos": [{"path": "` + repo + `", "post_create": ["printf %s \"$WT_BRANCH\" > branch.txt"]}]
}`
		if err := os.WriteFile(cfgPC, []byte(cfgJSON), 0o644); err != nil {
			t.Fatal(err)
		}
		// A legal git ref name (no spaces allowed) that creates BOOM via
		// redirection if it is ever shell-interpolated.
		evil := "inj-$(>BOOM)"
		out, stderr, err := wt(t, home, cfgPC, repo, "add", evil)
		if err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		if _, err := os.Stat(filepath.Join(out, "BOOM")); !os.IsNotExist(err) {
			t.Error("branch name was shell-interpolated: BOOM file exists")
		}
		if b, _ := os.ReadFile(filepath.Join(out, "branch.txt")); string(b) != evil {
			t.Errorf("WT_BRANCH = %q; want the literal branch name %q", b, evil)
		}
	})

	t.Run("post_create failure reports the worktree survived", func(t *testing.T) {
		cfgPC := filepath.Join(work, "wt-postcreate-fail.json")
		cfgJSON := `{
  "worktree_dir": "` + trees + `/{repo}/{branch}",
  "repos": [{"path": "` + repo + `", "post_create": ["exit 7"]}]
}`
		if err := os.WriteFile(cfgPC, []byte(cfgJSON), 0o644); err != nil {
			t.Fatal(err)
		}
		_, stderr, err := wt(t, home, cfgPC, repo, "add", "pc-fail")
		if err == nil {
			t.Fatal("expected error when a post_create command fails")
		}
		if !strings.Contains(stderr, "worktree created") || !strings.Contains(stderr, "post_create") {
			t.Errorf("stderr = %q; want worktree-created and post_create mention", stderr)
		}
		if _, err := os.Stat(filepath.Join(trees, "myapp", "pc-fail")); err != nil {
			t.Errorf("worktree should survive a failed post_create: %v", err)
		}
	})

	// The repo-local config lives in the main worktree and is deleted again
	// by each subtest, so it cannot leak into the ones that follow.
	localCfg := filepath.Join(repo, ".wtrc")

	t.Run(".wtrc overrides global repos entry", func(t *testing.T) {
		cfgLocal := filepath.Join(work, "wt-local.json")
		cfgJSON := `{
  "worktree_dir": "` + trees + `/{repo}/{branch}",
  "repos": [{"name": "myapp", "path": "` + repo + `", "worktree_dir": "` + trees + `/trees-A/{branch}", "branch_prefix": "team"}]
}`
		if err := os.WriteFile(cfgLocal, []byte(cfgJSON), 0o644); err != nil {
			t.Fatal(err)
		}
		localJSON := `{
  "name": "localname",
  "worktree_dir": "` + trees + `/local-{repo}/{branch}",
  "branch_prefix": "local"
}`
		if err := os.WriteFile(localCfg, []byte(localJSON), 0o644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(localCfg)

		out, stderr, err := wt(t, home, cfgLocal, repo, "add", "wtj-test")
		if err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		want := filepath.Join(trees, "local-localname", "local-wtj-test")
		if out != want {
			t.Errorf("stdout = %q; want %q from .wtrc", out, want)
		}
		if got := git(t, home, out, "rev-parse", "--abbrev-ref", "HEAD"); got != "local/wtj-test" {
			t.Errorf("checked-out branch = %q; want local/wtj-test", got)
		}
		if _, stderr, err := wt(t, home, cfgLocal, repo, "remove", "local/wtj-test"); err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
	})

	t.Run("repo post_create requires approval", func(t *testing.T) {
		trustFile := filepath.Join(home, ".wt", "trust.json")
		defer func() {
			os.Remove(localCfg)
			os.Remove(trustFile)
		}()
		writeLocal := func(command string) {
			t.Helper()
			if err := os.WriteFile(localCfg, []byte(`{"post_create": ["`+command+`"]}`), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		const approvedCmd = "echo ran > pc-marker.txt"
		writeLocal(approvedCmd)

		// Unapproved: the worktree is created, the commands are not run.
		out, stderr, err := wt(t, home, cfg, repo, "add", "pc-unapproved")
		if err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		if !strings.Contains(stderr, "not approved") {
			t.Errorf("stderr = %q; want a not-approved warning", stderr)
		}
		if _, err := os.Stat(filepath.Join(out, "pc-marker.txt")); !os.IsNotExist(err) {
			t.Error("unapproved repo post_create ran")
		}
		if _, err := os.Stat(out); err != nil {
			t.Errorf("worktree should still be created: %v", err)
		}

		// Approving the exact command list makes it run without prompting.
		mainPath, err := filepath.EvalSymlinks(repo)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(trustFile), 0o700); err != nil {
			t.Fatal(err)
		}
		trustJSON := `{"repos":{"` + mainPath + `":{"post_create":["` + approvedCmd + `"],"approved_at":"2026-01-01T00:00:00Z"}}}`
		if err := os.WriteFile(trustFile, []byte(trustJSON), 0o600); err != nil {
			t.Fatal(err)
		}
		out, stderr, err = wt(t, home, cfg, repo, "add", "pc-trusted")
		if err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		if _, err := os.Stat(filepath.Join(out, "pc-marker.txt")); err != nil {
			t.Errorf("approved repo post_create did not run: %v", err)
		}

		// Changing the commands invalidates the approval.
		writeLocal("echo changed > pc-marker.txt")
		out, stderr, err = wt(t, home, cfg, repo, "add", "pc-changed")
		if err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		if !strings.Contains(stderr, "not approved") {
			t.Errorf("stderr = %q; want a not-approved warning after the commands changed", stderr)
		}
		if _, err := os.Stat(filepath.Join(out, "pc-marker.txt")); !os.IsNotExist(err) {
			t.Error("changed repo post_create ran without re-approval")
		}
	})

	t.Run(".wtrc with repos key fails add but not list", func(t *testing.T) {
		if err := os.WriteFile(localCfg, []byte(`{"repos": []}`), 0o644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(localCfg)

		_, stderr, err := wt(t, home, cfg, repo, "add", "wtj-invalid")
		if err == nil {
			t.Fatal("expected error for a .wtrc with a repos key")
		}
		if !strings.Contains(stderr, ".wtrc") {
			t.Errorf("stderr = %q; want the repo-local file named", stderr)
		}
		if _, stderr, err := wt(t, home, cfg, repo, "list"); err != nil {
			t.Errorf("list should survive a broken .wtrc: %v\n%s", err, stderr)
		}
	})

	t.Run("wt config prints repo-local file", func(t *testing.T) {
		localJSON := `{"branch_prefix": "local"}`
		if err := os.WriteFile(localCfg, []byte(localJSON+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(localCfg)

		// git reports symlink-resolved paths, and so does the header.
		resolved, err := filepath.EvalSymlinks(localCfg)
		if err != nil {
			t.Fatal(err)
		}
		out, stderr, err := wt(t, home, cfg, repo, "config")
		if err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		if !strings.Contains(stderr, resolved) || !strings.Contains(stderr, "repo-local") {
			t.Errorf("stderr = %q; want the .wtrc path marked repo-local", stderr)
		}
		if !strings.Contains(out, localJSON) {
			t.Errorf("stdout = %q; want the repo-local content %q", out, localJSON)
		}
	})

	t.Run("init scaffolds a repo-local .wtrc", func(t *testing.T) {
		out, stderr, err := wt(t, home, cfg, repo, "init")
		if err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		wtjson := filepath.Join(repo, ".wtrc")
		t.Cleanup(func() { os.Remove(wtjson) })
		data, err := os.ReadFile(wtjson)
		if err != nil {
			t.Fatalf(".wtrc not created at the main worktree root: %v", err)
		}
		var lc struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(data, &lc); err != nil || lc.Name != "myapp" {
			t.Errorf(".wtrc = %s (err %v); want name myapp", data, err)
		}
		if filepath.Base(out) != ".wtrc" {
			t.Errorf("stdout = %q; want the created file path", out)
		}

		if _, stderr, err := wt(t, home, cfg, repo, "init"); err == nil {
			t.Error("second init should refuse to overwrite")
		} else if !strings.Contains(stderr, "already exists") {
			t.Errorf("stderr = %q; want already-exists error", stderr)
		}

		// From a linked worktree, init still targets the main worktree root.
		os.Remove(wtjson)
		linked, _, err := wt(t, home, cfg, repo, "add", "init-linked")
		if err != nil {
			t.Fatal(err)
		}
		if _, stderr, err := wt(t, home, cfg, linked, "init"); err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		if _, err := os.Stat(wtjson); err != nil {
			t.Errorf(".wtrc missing from main worktree after init in linked worktree: %v", err)
		}
		if _, err := os.Stat(filepath.Join(linked, ".wtrc")); !os.IsNotExist(err) {
			t.Error(".wtrc must not be created inside the linked worktree")
		}
	})

	t.Run("prune cleans up stale worktrees", func(t *testing.T) {
		out, _, err := wt(t, home, cfg, repo, "add", "doomed")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(out); err != nil {
			t.Fatal(err)
		}

		_, stderr, err := wt(t, home, cfg, repo, "prune", "--dry-run")
		if err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		if !strings.Contains(stderr, "doomed") || !strings.Contains(stderr, "dry run") {
			t.Errorf("dry-run stderr = %q; want the stale worktree listed", stderr)
		}
		if listOut, _, err := wt(t, home, cfg, repo, "list"); err != nil || !strings.Contains(listOut, "doomed") {
			t.Errorf("dry run should not have pruned; list = %q, %v", listOut, err)
		}

		_, stderr, err = wt(t, home, cfg, repo, "prune")
		if err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		if !strings.Contains(stderr, "Pruned 1 stale worktree entry") {
			t.Errorf("prune stderr = %q; want prune confirmation", stderr)
		}
		if listOut, _, err := wt(t, home, cfg, repo, "list"); err != nil || strings.Contains(listOut, "doomed") {
			t.Errorf("stale worktree still listed after prune: %q, %v", listOut, err)
		}
		git(t, home, repo, "rev-parse", "--verify", "refs/heads/doomed")

		_, stderr, err = wt(t, home, cfg, repo, "prune")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stderr, "Nothing to prune") {
			t.Errorf("second prune stderr = %q; want nothing to prune", stderr)
		}
	})

	t.Run("repo branch prefix overrides global", func(t *testing.T) {
		cfgRepoPrefix := filepath.Join(work, "wt-repo-prefix.json")
		cfgJSON := `{
  "worktree_dir": "` + trees + `/{repo}/{branch}",
  "branch_prefix": "peter/",
  "repos": [{"path": "` + repo + `", "branch_prefix": "team"}]
}`
		if err := os.WriteFile(cfgRepoPrefix, []byte(cfgJSON), 0o644); err != nil {
			t.Fatal(err)
		}
		out, _, err := wt(t, home, cfgRepoPrefix, repo, "add", "gadget")
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(trees, "myapp", "team-gadget")
		if out != want {
			t.Errorf("stdout = %q; want %q", out, want)
		}
		if got := git(t, home, out, "rev-parse", "--abbrev-ref", "HEAD"); got != "team/gadget" {
			t.Errorf("checked-out branch = %q; want team/gadget", got)
		}
	})
}
