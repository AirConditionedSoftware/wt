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
  "repos": {"myapp": {"default_base": "main"}}
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

	t.Run("list outside a repo fails", func(t *testing.T) {
		if _, _, err := wt(t, home, cfg, t.TempDir(), "list"); err == nil {
			t.Error("expected error outside a git repository")
		}
	})
}
