package cmd

import (
	"fmt"
	"io/fs"
	"os"
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
			return fmt.Errorf("branch %q is already checked out at %s", branch, w.Path)
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
		return fmt.Errorf("target directory already exists: %s", target)
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
			return fmt.Errorf("worktree created at %s, but copying hooks failed: %w", target, err)
		}
	}

	fmt.Println(target)
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
		fmt.Fprintf(os.Stderr, "Hooks at %s are shared by all worktrees; nothing to copy\n", src)
		return nil
	}
	if _, err := os.Stat(src); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Hooks directory %s does not exist; nothing to copy\n", src)
		return nil
	}
	if err := copyDir(src, dst); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Copied hooks to %s\n", dst)
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
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

func init() {
	addCmd.Flags().StringVar(&addBase, "base", "", "base ref for a newly created branch (default: config default_base, then HEAD)")
	addCmd.Flags().StringVar(&addPath, "path", "", "override the config-derived worktree location")
	addCmd.Flags().BoolVar(&addNoPrefix, "no-prefix", false, "do not apply the configured branch_prefix")
	addCmd.Flags().BoolVar(&addCopyHooks, "copy-hooks", false, "copy the repo's git hooks into the new worktree")
	addCmd.Flags().BoolVar(&addNoCopyHooks, "no-copy-hooks", false, "do not copy hooks even if the config enables copy_hooks")
	rootCmd.AddCommand(addCmd)
}
