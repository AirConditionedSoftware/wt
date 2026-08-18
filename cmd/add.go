package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/piskandar/wt/internal/config"
	"github.com/piskandar/wt/internal/gitx"
	"github.com/spf13/cobra"
)

var (
	addBase string
	addPath string
)

var addCmd = &cobra.Command{
	Use:   "add <branch>",
	Short: "Create a worktree for a branch",
	Long: `Create a worktree for a branch. The location is derived from the config's
worktree_dir template unless --path is given.

If the branch exists locally it is checked out as-is; if it exists on origin
a local branch tracking it is created; otherwise a new branch is created from
--base, the config's default_base, or the current HEAD.

The created path is the only output on stdout, so shell integration like
cd "$(wt add my-branch)" works.`,
	Args: cobra.ExactArgs(1),
	RunE: runAdd,
}

func runAdd(cmd *cobra.Command, args []string) error {
	branch := args[0]

	wts, err := gitx.ListWorktrees(".")
	if err != nil {
		return err
	}
	repo := filepath.Base(wts[0].Path)

	for _, w := range wts {
		if w.Branch == branch {
			return fmt.Errorf("branch %q is already checked out at %s", branch, w.Path)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	settings := cfg.For(repo)

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

	switch {
	case gitx.LocalBranchExists(".", branch):
		fmt.Fprintf(os.Stderr, "Creating worktree for existing branch %q\n", branch)
		_, err = gitx.Run(".", "worktree", "add", target, branch)
	case gitx.RemoteBranchExists(".", branch):
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

	fmt.Println(target)
	return nil
}

func init() {
	addCmd.Flags().StringVar(&addBase, "base", "", "base ref for a newly created branch (default: config default_base, then HEAD)")
	addCmd.Flags().StringVar(&addPath, "path", "", "override the config-derived worktree location")
	rootCmd.AddCommand(addCmd)
}
