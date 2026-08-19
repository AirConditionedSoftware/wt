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

// DefaultPrefixSeparator joins branch_prefix to the branch name when no
// config sets prefix_separator.
const DefaultPrefixSeparator = "/"

// Settings are the options that can be set globally and overridden per repo.
type Settings struct {
	// WorktreeDir is a path template; {repo} and {branch} are substituted
	// and a leading ~ expands to the home directory.
	WorktreeDir string `json:"worktree_dir,omitempty"`
	// DefaultBase is the ref new branches start from when the branch exists
	// neither locally nor on origin. Empty means the current HEAD.
	DefaultBase string `json:"default_base,omitempty"`
	// BranchPrefix is prepended to branch names that wt add creates,
	// joined with PrefixSeparator: "peter" -> "peter/fix-login".
	BranchPrefix string `json:"branch_prefix,omitempty"`
	// PrefixSeparator joins BranchPrefix to the branch name. Empty means
	// DefaultPrefixSeparator.
	PrefixSeparator string `json:"prefix_separator,omitempty"`
	// CopyHooks copies the repo's git hooks into newly created worktrees.
	// Only relevant when core.hooksPath points inside the worktree (e.g.
	// husky's .husky); plain .git/hooks is already shared by all worktrees.
	// A pointer so a per-repo false can override a global true.
	CopyHooks *bool `json:"copy_hooks,omitempty"`
	// CopyFiles are paths or globs, relative to the main worktree, of
	// untracked files (e.g. ".env") to copy into newly created worktrees.
	// A repo entry's list replaces the global one; [] disables copying.
	CopyFiles []string `json:"copy_files,omitempty"`
	// VSCodeOpen opens the worktree in VS Code after wt add creates it.
	VSCodeOpen *bool `json:"vscode_open,omitempty"`
	// VSCodeWorkspaceFile writes a .code-workspace file into each new
	// worktree, named "<vscode_workspace_prefix><branch>.code-workspace".
	VSCodeWorkspaceFile *bool `json:"vscode_workspace_file,omitempty"`
	// VSCodeWorkspacePrefix is prepended to the workspace file's name.
	VSCodeWorkspacePrefix string `json:"vscode_workspace_prefix,omitempty"`
	// VSCodeWindowTitle is written verbatim as the workspace file's
	// settings["window.title"], so VS Code title variables like
	// ${activeEditorShort} pass through. Empty means the repo name.
	VSCodeWindowTitle string `json:"vscode_window_title,omitempty"`
	// WorkspacePaths are extra folders added to generated .code-workspace
	// files after the worktree itself. A repo entry's list replaces the
	// global one.
	WorkspacePaths []WorkspacePath `json:"workspace_paths,omitempty"`
	// FullPaths shows absolute paths in human-facing output instead of
	// abbreviating the home directory to ~. Same effect as --full-paths.
	FullPaths *bool `json:"full_paths,omitempty"`
	// PostCreate commands run inside a newly created worktree, in order,
	// via sh -c. They come only from this user-owned config file — never
	// from the repository — and worktree metadata is passed as WT_*
	// environment variables rather than interpolated into the command.
	// A repo entry's list replaces the global one; [] disables.
	PostCreate []string `json:"post_create,omitempty"`
}

// WorkspacePath is one extra folder for generated .code-workspace files.
type WorkspacePath struct {
	// Name is the folder's display name in VS Code. Optional.
	Name string `json:"name,omitempty"`
	// Path is the folder's location; a leading ~ expands to the home
	// directory, anything else is written as given.
	Path string `json:"path"`
}

// RepoConfig is one repos entry: settings for the repository whose main
// worktree lives at Path.
type RepoConfig struct {
	// Name is what {repo} expands to in templates for this repo. Empty
	// means the directory basename of the main worktree.
	Name string `json:"name,omitempty"`
	// Path is the filesystem path of the repo's main worktree.
	Path string `json:"path"`
	Settings
}

// File is the full wt.json schema: top-level defaults plus per-repo
// overrides matched by the main worktree's path.
type File struct {
	Settings
	Repos []RepoConfig `json:"repos,omitempty"`
}

// LocalFileName is the repo-local config file, read from the root of a
// repository's main worktree only — never from a linked worktree.
const LocalFileName = ".wtrc"

// LocalConfig is the .wtrc schema: the same settings as a repos entry,
// for the repository the file lives in. It is parsed with unknown fields
// rejected, so "repos" and "path" — meaningless in a file that is itself
// the repo — fail loudly.
type LocalConfig struct {
	// Name is what {repo} expands to for this repo. It overrides the name
	// of a matching global repos entry. Empty means the entry's name, or
	// the directory basename of the main worktree.
	Name string `json:"name,omitempty"`
	Settings
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
	if err := validateWorkspacePaths("config "+path, "", cfg.WorkspacePaths); err != nil {
		return nil, err
	}
	for i, r := range cfg.Repos {
		if r.Path == "" {
			return nil, fmt.Errorf("config %s: repos[%d] is missing \"path\"", path, i)
		}
		if err := validateWorkspacePaths("config "+path, fmt.Sprintf("repos[%d] ", i), r.WorkspacePaths); err != nil {
			return nil, err
		}
	}
	return &cfg, nil
}

// validateWorkspacePaths reports entries missing "path". desc names the file
// ("config <path>" or "repo config <path>"), where locates it within the file.
func validateWorkspacePaths(desc, where string, wps []WorkspacePath) error {
	for i, wp := range wps {
		if wp.Path == "" {
			return fmt.Errorf("%s: %sworkspace_paths[%d] is missing \"path\"", desc, where, i)
		}
	}
	return nil
}

// loadLocal reads <mainPath>/.wtrc. A missing file is not an error; the
// returned path is reported either way, for provenance. Broken JSON and
// unknown fields fail loudly, like a broken global config.
func loadLocal(mainPath string) (*LocalConfig, string, error) {
	path := filepath.Join(mainPath, LocalFileName)
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, path, nil
	}
	if err != nil {
		return nil, path, fmt.Errorf("repo config %s: %w", path, err)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	var local LocalConfig
	if err := dec.Decode(&local); err != nil {
		return nil, path, fmt.Errorf("repo config %s: %w", path, err)
	}
	if err := validateWorkspacePaths("repo config "+path, "", local.WorkspacePaths); err != nil {
		return nil, path, err
	}
	return &local, path, nil
}

// Resolved is the effective configuration for one repository, plus where the
// values came from.
type Resolved struct {
	Settings
	// RepoName is the .wtrc name, else the global repos entry's name,
	// else "" (the caller falls back to the directory basename).
	RepoName string
	// LocalFile is the path of the .wtrc that was loaded, "" if none.
	LocalFile string
	// PostCreateFromRepo reports that the effective PostCreate came from
	// .wtrc rather than the user-owned global config, and so needs
	// approval before it runs.
	PostCreateFromRepo bool
}

// Resolve returns the effective settings for the repository whose main
// worktree is at mainPath, layering built-in defaults, the global config's
// top-level settings, its matching repos entry, and finally the repo's own
// .wtrc. Each layer overrides field by field: empty strings fall through,
// lists and booleans that are set replace the layer below.
func Resolve(mainPath string) (Resolved, error) {
	cfg, err := Load()
	if err != nil {
		return Resolved{}, err
	}
	settings, name := cfg.ForPath(mainPath)
	resolved := Resolved{Settings: settings, RepoName: name}

	local, path, err := loadLocal(mainPath)
	if err != nil {
		return Resolved{}, err
	}
	if local == nil {
		return resolved, nil
	}
	resolved.LocalFile = path
	resolved.Settings.merge(local.Settings)
	if local.Name != "" {
		resolved.RepoName = local.Name
	}
	// An explicit [] clears the inherited commands and still counts as
	// repo-sourced, so the approval gate sees the repo's decision.
	resolved.PostCreateFromRepo = local.PostCreate != nil
	return resolved, nil
}

// ForPath returns the effective settings for the repository whose main
// worktree is at mainPath: built-in defaults, overlaid with the file's
// top-level settings, overlaid with the first repos entry whose path matches.
// The second return is the matching entry's name ("" when unnamed or no
// entry matches).
func (f *File) ForPath(mainPath string) (Settings, string) {
	s := Settings{WorktreeDir: DefaultWorktreeDir}
	s.merge(f.Settings)
	target := normalizePath(mainPath)
	for _, r := range f.Repos {
		if normalizePath(r.Path) == target {
			s.merge(r.Settings)
			return s, r.Name
		}
	}
	return s, ""
}

func (s *Settings) merge(over Settings) {
	if over.WorktreeDir != "" {
		s.WorktreeDir = over.WorktreeDir
	}
	if over.DefaultBase != "" {
		s.DefaultBase = over.DefaultBase
	}
	if over.BranchPrefix != "" {
		s.BranchPrefix = over.BranchPrefix
	}
	if over.PrefixSeparator != "" {
		s.PrefixSeparator = over.PrefixSeparator
	}
	if over.CopyHooks != nil {
		s.CopyHooks = over.CopyHooks
	}
	if over.CopyFiles != nil {
		s.CopyFiles = over.CopyFiles
	}
	if over.VSCodeOpen != nil {
		s.VSCodeOpen = over.VSCodeOpen
	}
	if over.VSCodeWorkspaceFile != nil {
		s.VSCodeWorkspaceFile = over.VSCodeWorkspaceFile
	}
	if over.VSCodeWorkspacePrefix != "" {
		s.VSCodeWorkspacePrefix = over.VSCodeWorkspacePrefix
	}
	if over.VSCodeWindowTitle != "" {
		s.VSCodeWindowTitle = over.VSCodeWindowTitle
	}
	if over.WorkspacePaths != nil {
		s.WorkspacePaths = over.WorkspacePaths
	}
	if over.FullPaths != nil {
		s.FullPaths = over.FullPaths
	}
	if over.PostCreate != nil {
		s.PostCreate = over.PostCreate
	}
}

// FullPathsEnabled reports whether full_paths is set and true.
func (s Settings) FullPathsEnabled() bool {
	return s.FullPaths != nil && *s.FullPaths
}

// CopyHooksEnabled reports whether copy_hooks is set and true.
func (s Settings) CopyHooksEnabled() bool {
	return s.CopyHooks != nil && *s.CopyHooks
}

// VSCodeOpenEnabled reports whether vscode_open is set and true.
func (s Settings) VSCodeOpenEnabled() bool {
	return s.VSCodeOpen != nil && *s.VSCodeOpen
}

// VSCodeWorkspaceFileEnabled reports whether vscode_workspace_file is set
// and true.
func (s Settings) VSCodeWorkspaceFileEnabled() bool {
	return s.VSCodeWorkspaceFile != nil && *s.VSCodeWorkspaceFile
}

// EffectivePrefix returns BranchPrefix with the separator applied, e.g.
// "peter" -> "peter/". A prefix already ending in the separator is not
// doubled. Empty if no prefix is configured.
func (s Settings) EffectivePrefix() string {
	if s.BranchPrefix == "" {
		return ""
	}
	sep := s.PrefixSeparator
	if sep == "" {
		sep = DefaultPrefixSeparator
	}
	return strings.TrimSuffix(s.BranchPrefix, sep) + sep
}

// WorktreePath expands the WorktreeDir template for repo and branch.
func (s Settings) WorktreePath(repo, branch string) (string, error) {
	p := s.WorktreeDir
	p = strings.ReplaceAll(p, "{repo}", repo)
	p = strings.ReplaceAll(p, "{branch}", SanitizeBranch(branch))
	p, err := ExpandTilde(p)
	if err != nil {
		return "", err
	}
	return filepath.Clean(p), nil
}

// SanitizeBranch makes a branch name safe to use as a single path segment.
func SanitizeBranch(branch string) string {
	return strings.ReplaceAll(branch, "/", "-")
}

func ExpandTilde(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, strings.TrimPrefix(p, "~")), nil
	}
	return p, nil
}

// normalizePath makes paths comparable: ~ expanded, absolute, symlinks
// resolved. Best-effort — a path that can't be resolved is used as-is.
func normalizePath(p string) string {
	if e, err := ExpandTilde(p); err == nil {
		p = e
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	return filepath.Clean(p)
}
