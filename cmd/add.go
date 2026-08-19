package cmd

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/AirConditionedSoftware/wt/internal/config"
	"github.com/AirConditionedSoftware/wt/internal/gitx"
	"github.com/spf13/cobra"
)

var (
	addBase        string
	addPath        string
	addNoPrefix    bool
	addCopyHooks   bool
	addNoCopyHooks bool
	addCopyFile    []string
	addNoCopyFiles bool
	addOpen        bool
	addNoOpen      bool
)

type branchKind int

const (
	kindNew branchKind = iota
	kindLocal
	kindRemote
)

var addCmd = &cobra.Command{
	Use:   "add <branch>",
	Short: "Create a worktree for a branch",
	Long: `Create a worktree for a branch. The location is derived from the config's
worktree_dir template unless --path is given.

If the branch exists locally it is checked out as-is; if it exists on origin
a local branch tracking it is created; otherwise a new branch is created from
--base, the config's default_base, or the current HEAD. New branches get the
config's branch_prefix if one is set (wt add fix-login -> peter/fix-login);
--no-prefix skips it.

The created path is the only output on stdout, so shell integration like
cd "$(wt add my-branch)" works.`,
	Args: cobra.ExactArgs(1),
	RunE: runAdd,
}

func runAdd(cmd *cobra.Command, args []string) error {
	arg := args[0]

	wts, err := gitx.ListWorktrees(".")
	if err != nil {
		return err
	}
	mainPath := wts[0].Path

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	settings, repo := cfg.ForPath(mainPath)
	if repo == "" {
		repo = filepath.Base(mainPath)
	}
	applyDisplayConfig(settings)

	prefix := settings.EffectivePrefix()
	if addNoPrefix || (prefix != "" && strings.HasPrefix(arg, prefix)) {
		prefix = ""
	}

	// An existing branch — with or without the prefix — is used as-is; the
	// prefix only names branches that don't exist yet.
	branch := arg
	kind := kindNew
	switch {
	case gitx.LocalBranchExists(".", arg):
		kind = kindLocal
	case prefix != "" && gitx.LocalBranchExists(".", prefix+arg):
		branch, kind = prefix+arg, kindLocal
	case gitx.RemoteBranchExists(".", arg):
		kind = kindRemote
	case prefix != "" && gitx.RemoteBranchExists(".", prefix+arg):
		branch, kind = prefix+arg, kindRemote
	default:
		branch = prefix + arg
	}

	for _, w := range wts {
		if w.Branch == branch {
			return fmt.Errorf("branch %q is already checked out at %s", branch, displayPath(w.Path))
		}
	}

	target := addPath
	if target == "" {
		if target, err = settings.WorktreePath(repo, branch); err != nil {
			return err
		}
	}
	if abs, err := filepath.Abs(target); err == nil {
		target = abs
	}
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("target directory already exists: %s", displayPath(target))
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	switch kind {
	case kindLocal:
		fmt.Fprintf(os.Stderr, "Creating worktree for existing branch %q\n", branch)
		_, err = gitx.Run(".", "worktree", "add", target, branch)
	case kindRemote:
		fmt.Fprintf(os.Stderr, "Creating worktree for %q tracking origin/%s\n", branch, branch)
		_, err = gitx.Run(".", "worktree", "add", "--track", "-b", branch, target, "origin/"+branch)
	default:
		base := addBase
		if base == "" {
			base = settings.DefaultBase
		}
		if base == "" {
			base = "HEAD"
		}
		fmt.Fprintf(os.Stderr, "Creating worktree with new branch %q from %s\n", branch, base)
		_, err = gitx.Run(".", "worktree", "add", "-b", branch, target, base)
	}
	if err != nil {
		return err
	}

	wantHooks := settings.CopyHooksEnabled()
	if addCopyHooks {
		wantHooks = true
	}
	if addNoCopyHooks {
		wantHooks = false
	}
	if wantHooks {
		if err := copyHooksTo(mainPath, target); err != nil {
			return fmt.Errorf("worktree created at %s, but copying hooks failed: %w", displayPath(target), err)
		}
	}

	patterns := settings.CopyFiles
	if addNoCopyFiles {
		patterns = nil
	}
	patterns = append(append([]string{}, patterns...), addCopyFile...)
	if len(patterns) > 0 {
		if err := copyFilesTo(mainPath, target, patterns); err != nil {
			return fmt.Errorf("worktree created at %s, but copying files failed: %w", displayPath(target), err)
		}
	}

	openTarget := target
	if settings.VSCodeWorkspaceFileEnabled() {
		wsPath, err := writeWorkspaceFile(target, settings, repo, branch)
		if err != nil {
			return fmt.Errorf("worktree created at %s, but writing the workspace file failed: %w", displayPath(target), err)
		}
		fmt.Fprintf(os.Stderr, "Wrote %s\n", displayPath(wsPath))
		openTarget = wsPath
	}

	wantOpen := settings.VSCodeOpenEnabled()
	if addOpen {
		wantOpen = true
	}
	if addNoOpen {
		wantOpen = false
	}
	if wantOpen {
		openInVSCode(openTarget)
	}

	fmt.Println(target)
	return nil
}

// workspaceFilePath returns where the wt-generated .code-workspace for a
// worktree lives: next to the worktree directory (a sibling, so it never
// shows up as an untracked file inside it). "" for branchless worktrees.
func workspaceFilePath(settings config.Settings, worktreePath, branch string) string {
	if branch == "" {
		return ""
	}
	name := settings.VSCodeWorkspacePrefix + config.SanitizeBranch(branch) + ".code-workspace"
	return filepath.Join(filepath.Dir(worktreePath), name)
}

// writeWorkspaceFile writes a .code-workspace file next to the worktree with
// a folders entry for it and window.title set to the configured value (taken
// verbatim, VS Code title variables included) or the repo name.
func writeWorkspaceFile(worktreePath string, settings config.Settings, repo, branch string) (string, error) {
	title := settings.VSCodeWindowTitle
	if title == "" {
		title = repo
	}
	wsPath := workspaceFilePath(settings, worktreePath, branch)
	if wsPath == "" {
		return "", fmt.Errorf("cannot derive a workspace file name for %s", displayPath(worktreePath))
	}
	folders := []map[string]string{{"path": worktreePath}}
	for _, wp := range settings.WorkspacePaths {
		p, err := config.ExpandTilde(wp.Path)
		if err != nil {
			return "", err
		}
		entry := map[string]string{"path": p}
		if wp.Name != "" {
			entry["name"] = wp.Name
		}
		folders = append(folders, entry)
	}
	ws := map[string]any{
		"folders":  folders,
		"settings": map[string]string{"window.title": title},
	}
	data, err := json.MarshalIndent(ws, "", "  ")
	if err != nil {
		return "", err
	}
	return wsPath, os.WriteFile(wsPath, append(data, '\n'), 0o644)
}

// openInVSCode launches VS Code on path. Failing to open is a warning, not
// an error — the worktree itself is fine.
func openInVSCode(path string) {
	codeBin, err := exec.LookPath("code")
	if err != nil {
		fmt.Fprintln(os.Stderr, `VS Code CLI "code" not found on PATH; not opening`)
		return
	}
	if out, err := exec.Command(codeBin, path).CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "opening in VS Code failed: %v\n%s", err, out)
		return
	}
	fmt.Fprintf(os.Stderr, "Opened %s in VS Code\n", displayPath(path))
}

// copyFilesTo copies untracked files (e.g. .env) from the main worktree into
// the new one. Patterns are filepath.Glob globs relative to the main
// worktree; a matched directory copies recursively.
func copyFilesTo(mainPath, worktreePath string, patterns []string) error {
	copied := 0
	for _, pat := range patterns {
		if !filepath.IsLocal(pat) {
			return fmt.Errorf("copy_files pattern %q must stay inside the repository", pat)
		}
		matches, err := filepath.Glob(filepath.Join(mainPath, pat))
		if err != nil {
			return fmt.Errorf("copy_files pattern %q: %w", pat, err)
		}
		if len(matches) == 0 {
			fmt.Fprintf(os.Stderr, "copy_files: no match for %q\n", pat)
			continue
		}
		for _, src := range matches {
			rel, err := filepath.Rel(mainPath, src)
			if err != nil {
				return err
			}
			dst := filepath.Join(worktreePath, rel)
			info, err := os.Stat(src)
			if err != nil {
				return err
			}
			if info.IsDir() {
				err = copyDir(src, dst)
			} else {
				err = copyFile(src, dst, info.Mode().Perm())
			}
			if err != nil {
				return err
			}
			copied++
		}
	}
	if copied > 0 {
		fmt.Fprintf(os.Stderr, "Copied %d file(s) into %s\n", copied, displayPath(worktreePath))
	}
	return nil
}

// copyHooksTo copies the main worktree's effective hooks directory into the
// new worktree. With the default .git/hooks both worktrees resolve to the
// same directory (git shares it via the common git dir), so there is nothing
// to copy; a core.hooksPath inside the worktree resolves per worktree and
// needs the copy.
func copyHooksTo(mainPath, worktreePath string) error {
	src, err := gitx.HooksDir(mainPath)
	if err != nil {
		return err
	}
	dst, err := gitx.HooksDir(worktreePath)
	if err != nil {
		return err
	}
	if samePath(src, dst) {
		fmt.Fprintf(os.Stderr, "Hooks at %s are shared by all worktrees; nothing to copy\n", displayPath(src))
		return nil
	}
	if _, err := os.Stat(src); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Hooks directory %s does not exist; nothing to copy\n", displayPath(src))
		return nil
	}
	if err := copyDir(src, dst); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Copied hooks to %s\n", displayPath(dst))
	return nil
}

// copyDir copies regular files recursively, preserving permissions (hooks
// must stay executable) and skipping git's *.sample placeholders.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() || strings.HasSuffix(p, ".sample") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(p, target, info.Mode().Perm())
	})
}

func copyFile(src, dst string, perm fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, perm)
}

func init() {
	addCmd.Flags().StringVar(&addBase, "base", "", "base ref for a newly created branch (default: config default_base, then HEAD)")
	addCmd.Flags().StringVar(&addPath, "path", "", "override the config-derived worktree location")
	addCmd.Flags().BoolVar(&addNoPrefix, "no-prefix", false, "do not apply the configured branch_prefix")
	addCmd.Flags().BoolVar(&addCopyHooks, "copy-hooks", false, "copy the repo's git hooks into the new worktree")
	addCmd.Flags().BoolVar(&addNoCopyHooks, "no-copy-hooks", false, "do not copy hooks even if the config enables copy_hooks")
	addCmd.Flags().StringArrayVar(&addCopyFile, "copy-file", nil, "extra file or glob (relative to the main worktree) to copy into the new worktree; repeatable")
	addCmd.Flags().BoolVar(&addNoCopyFiles, "no-copy-files", false, "skip the config's copy_files list")
	addCmd.Flags().BoolVar(&addOpen, "open", false, "open the new worktree in VS Code")
	addCmd.Flags().BoolVar(&addNoOpen, "no-open", false, "do not open in VS Code even if the config enables vscode_open")
	rootCmd.AddCommand(addCmd)
}
