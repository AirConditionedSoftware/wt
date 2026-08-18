package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathExplicit(t *testing.T) {
	t.Setenv(EnvVar, "/somewhere/custom.json")
	p, explicit, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if p != "/somewhere/custom.json" || !explicit {
		t.Errorf("Path() = %q, explicit=%v; want /somewhere/custom.json, true", p, explicit)
	}
}

func TestPathDefault(t *testing.T) {
	t.Setenv(EnvVar, "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	p, explicit, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".wt", "wt.json")
	if p != want || explicit {
		t.Errorf("Path() = %q, explicit=%v; want %q, false", p, explicit, want)
	}
}

func TestLoadMissingDefaultIsEmpty(t *testing.T) {
	t.Setenv(EnvVar, "")
	t.Setenv("HOME", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() with no config file: %v", err)
	}
	if cfg.WorktreeDir != "" || cfg.DefaultBase != "" || len(cfg.Repos) != 0 {
		t.Errorf("Load() = %+v; want empty config", cfg)
	}
}

func TestLoadMissingExplicitFails(t *testing.T) {
	t.Setenv(EnvVar, filepath.Join(t.TempDir(), "nope.json"))
	if _, err := Load(); err == nil {
		t.Error("Load() with missing $WT_CONFIG file should fail")
	}
}

func TestLoadUnknownFieldFails(t *testing.T) {
	p := filepath.Join(t.TempDir(), "wt.json")
	if err := os.WriteFile(p, []byte(`{"worktre_dir": "/x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvVar, p)
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "worktre_dir") {
		t.Errorf("Load() with unknown field: err = %v; want unknown-field error", err)
	}
}

func TestLoadAndFor(t *testing.T) {
	p := filepath.Join(t.TempDir(), "wt.json")
	content := `{
  "worktree_dir": "/global/{repo}/{branch}",
  "default_base": "main",
  "repos": {
    "myapp": {"worktree_dir": "/special/{branch}", "default_base": "develop"},
    "partial": {"default_base": "trunk"}
  }
}`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvVar, p)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		repo string
		want Settings
	}{
		{"myapp", Settings{WorktreeDir: "/special/{branch}", DefaultBase: "develop"}},
		{"partial", Settings{WorktreeDir: "/global/{repo}/{branch}", DefaultBase: "trunk"}},
		{"unlisted", Settings{WorktreeDir: "/global/{repo}/{branch}", DefaultBase: "main"}},
	}
	for _, tt := range tests {
		if got := cfg.For(tt.repo); got != tt.want {
			t.Errorf("For(%q) = %+v; want %+v", tt.repo, got, tt.want)
		}
	}
}

func TestForBuiltinDefaults(t *testing.T) {
	cfg := &File{}
	got := cfg.For("anything")
	if got.WorktreeDir != DefaultWorktreeDir || got.DefaultBase != "" {
		t.Errorf("For() on empty config = %+v", got)
	}
}

func TestWorktreePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tests := []struct {
		tmpl, repo, branch, want string
	}{
		{"/trees/{repo}/{branch}", "myapp", "fix-login", "/trees/myapp/fix-login"},
		{"/trees/{repo}/{branch}", "myapp", "feature/login", "/trees/myapp/feature-login"},
		{"~/trees/{branch}", "myapp", "x", filepath.Join(home, "trees", "x")},
	}
	for _, tt := range tests {
		got, err := Settings{WorktreeDir: tt.tmpl}.WorktreePath(tt.repo, tt.branch)
		if err != nil {
			t.Fatalf("WorktreePath(%q, %q, %q): %v", tt.tmpl, tt.repo, tt.branch, err)
		}
		if got != tt.want {
			t.Errorf("WorktreePath(%q, %q, %q) = %q; want %q", tt.tmpl, tt.repo, tt.branch, got, tt.want)
		}
	}
}
