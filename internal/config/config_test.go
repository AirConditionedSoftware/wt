package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func boolPtr(b bool) *bool { return &b }

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
  "branch_prefix": "peter",
  "copy_hooks": true,
  "copy_files": [".env"],
  "repos": [
    {"name": "myapp", "path": "/code/myapp", "worktree_dir": "/special/{branch}", "default_base": "develop", "branch_prefix": "team", "prefix_separator": "_", "copy_hooks": false, "copy_files": [], "vscode_open": true, "vscode_workspace_file": true, "vscode_workspace_prefix": "acs-", "vscode_window_title": "myapp ${activeEditorShort}", "workspace_paths": [{"name": "docs", "path": "~/notes/myapp"}, {"path": "/shared/lib"}], "full_paths": true, "post_create": ["make setup"]},
    {"path": "/code/partial", "default_base": "trunk"}
  ]
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
		path     string
		want     Settings
		wantName string
	}{
		{"/code/myapp", Settings{WorktreeDir: "/special/{branch}", DefaultBase: "develop", BranchPrefix: "team", PrefixSeparator: "_", CopyHooks: boolPtr(false), CopyFiles: []string{}, VSCodeOpen: boolPtr(true), VSCodeWorkspaceFile: boolPtr(true), VSCodeWorkspacePrefix: "acs-", VSCodeWindowTitle: "myapp ${activeEditorShort}", WorkspacePaths: []WorkspacePath{{Name: "docs", Path: "~/notes/myapp"}, {Path: "/shared/lib"}}, FullPaths: boolPtr(true), PostCreate: []string{"make setup"}}, "myapp"},
		{"/code/partial", Settings{WorktreeDir: "/global/{repo}/{branch}", DefaultBase: "trunk", BranchPrefix: "peter", CopyHooks: boolPtr(true), CopyFiles: []string{".env"}}, ""},
		{"/code/unlisted", Settings{WorktreeDir: "/global/{repo}/{branch}", DefaultBase: "main", BranchPrefix: "peter", CopyHooks: boolPtr(true), CopyFiles: []string{".env"}}, ""},
	}
	for _, tt := range tests {
		got, name := cfg.ForPath(tt.path)
		if !reflect.DeepEqual(got, tt.want) || name != tt.wantName {
			t.Errorf("ForPath(%q) = %+v, %q; want %+v, %q", tt.path, got, name, tt.want, tt.wantName)
		}
	}
	if s, _ := cfg.ForPath("/code/myapp"); s.CopyHooksEnabled() {
		t.Error("myapp copy_hooks=false should override the global true")
	}
	if s, _ := cfg.ForPath("/code/unlisted"); !s.CopyHooksEnabled() {
		t.Error("unlisted repo should inherit the global copy_hooks=true")
	}
	if s, _ := cfg.ForPath("/code/myapp"); len(s.CopyFiles) != 0 {
		t.Errorf("myapp copy_files=[] should clear the inherited list, got %v", s.CopyFiles)
	}
	if s, _ := cfg.ForPath("/code/myapp"); !s.VSCodeOpenEnabled() || !s.VSCodeWorkspaceFileEnabled() {
		t.Error("myapp should have vscode_open and vscode_workspace_file enabled")
	}
	if s, _ := cfg.ForPath("/code/unlisted"); s.VSCodeOpenEnabled() || s.VSCodeWorkspaceFileEnabled() {
		t.Error("unlisted repo should not have vscode options enabled")
	}
}

func TestForPathBuiltinDefaults(t *testing.T) {
	cfg := &File{}
	got, name := cfg.ForPath("/nowhere")
	if got.WorktreeDir != DefaultWorktreeDir || got.DefaultBase != "" || name != "" {
		t.Errorf("ForPath() on empty config = %+v, %q", got, name)
	}
}

func TestLoadWorkspacePathMissingPathFails(t *testing.T) {
	p := filepath.Join(t.TempDir(), "wt.json")
	content := `{"repos": [{"path": "/x", "workspace_paths": [{"name": "docs"}]}]}`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvVar, p)
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "workspace_paths") {
		t.Errorf("Load() with pathless workspace_paths entry: err = %v; want missing-path error", err)
	}
}

func TestLoadRepoMissingPathFails(t *testing.T) {
	p := filepath.Join(t.TempDir(), "wt.json")
	if err := os.WriteFile(p, []byte(`{"repos": [{"name": "myapp"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvVar, p)
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "path") {
		t.Errorf("Load() with pathless repos entry: err = %v; want missing-path error", err)
	}
}

func TestEffectivePrefix(t *testing.T) {
	tests := []struct {
		prefix, sep, want string
	}{
		{"", "", ""},
		{"", "-", ""},
		{"peter", "", "peter/"},
		{"peter/", "", "peter/"},
		{"peter", "-", "peter-"},
		{"peter-", "-", "peter-"},
		{"team", "_", "team_"},
	}
	for _, tt := range tests {
		s := Settings{BranchPrefix: tt.prefix, PrefixSeparator: tt.sep}
		if got := s.EffectivePrefix(); got != tt.want {
			t.Errorf("EffectivePrefix(prefix=%q, sep=%q) = %q; want %q", tt.prefix, tt.sep, got, tt.want)
		}
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
