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
	cmd := exec.Command(wtBin, args...)
	cmd.Dir = dir
	cmd.Env = append(gitEnv(home), "WT_CONFIG="+cfg)
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
		if _, _, err := wt(t, home, cfg, repo, "remove", "dirty"); err == nil {
			t.Fatal("expected error removing a dirty worktree without --force")
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
