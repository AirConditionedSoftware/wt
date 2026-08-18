// Package config loads wt's JSON configuration. The file location comes from
// $WT_CONFIG, defaulting to ~/.wt/wt.json; a missing file at the default
// location means built-in defaults apply.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnvVar overrides the config file location.
const EnvVar = "WT_CONFIG"

// DefaultWorktreeDir places worktrees when no config sets worktree_dir.
const DefaultWorktreeDir = "~/worktrees/{repo}/{branch}"

// Settings are the options that can be set globally and overridden per repo.
type Settings struct {
	// WorktreeDir is a path template; {repo} and {branch} are substituted
	// and a leading ~ expands to the home directory.
	WorktreeDir string `json:"worktree_dir,omitempty"`
	// DefaultBase is the ref new branches start from when the branch exists
	// neither locally nor on origin. Empty means the current HEAD.
	DefaultBase string `json:"default_base,omitempty"`
}

// File is the full wt.json schema: top-level defaults plus per-repo overrides
// keyed by repo name (the directory basename of the main worktree).
type File struct {
	Settings
	Repos map[string]Settings `json:"repos,omitempty"`
}

// Path returns the config file location and whether it was set explicitly
// via $WT_CONFIG.
func Path() (path string, explicit bool, err error) {
	if p := os.Getenv(EnvVar); p != "" {
		return p, true, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false, err
	}
	return filepath.Join(home, ".wt", "wt.json"), false, nil
}

// Load reads the config file. A missing file at the default location is not
// an error; a missing file at an explicit $WT_CONFIG location is, so a typo'd
// path fails loudly instead of being silently ignored.
func Load() (*File, error) {
	path, explicit, err := Path()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) && !explicit {
		return &File{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	var cfg File
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	return &cfg, nil
}

// For returns the effective settings for repo: built-in defaults, overlaid
// with the file's top-level settings, overlaid with the repo's entry.
func (f *File) For(repo string) Settings {
	s := Settings{WorktreeDir: DefaultWorktreeDir}
	s.merge(f.Settings)
	if r, ok := f.Repos[repo]; ok {
		s.merge(r)
	}
	return s
}

func (s *Settings) merge(over Settings) {
	if over.WorktreeDir != "" {
		s.WorktreeDir = over.WorktreeDir
	}
	if over.DefaultBase != "" {
		s.DefaultBase = over.DefaultBase
	}
}

// WorktreePath expands the WorktreeDir template for repo and branch.
func (s Settings) WorktreePath(repo, branch string) (string, error) {
	p := s.WorktreeDir
	p = strings.ReplaceAll(p, "{repo}", repo)
	p = strings.ReplaceAll(p, "{branch}", SanitizeBranch(branch))
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return filepath.Clean(p), nil
}

// SanitizeBranch makes a branch name safe to use as a single path segment.
func SanitizeBranch(branch string) string {
	return strings.ReplaceAll(branch, "/", "-")
}
