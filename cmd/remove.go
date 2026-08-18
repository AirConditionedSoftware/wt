package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/AirConditionedSoftware/wt/internal/gitx"
	"github.com/spf13/cobra"
)

var removeForce bool

var removeCmd = &cobra.Command{
	Use:     "remove <branch>",
	Aliases: []string{"rm"},
	Short:   "Remove the worktree for a branch",
	Long: `Remove the worktree that has the given branch checked out. The branch
itself is kept. A path may be given instead of a branch name.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return removeWorktree(args[0])
	},
}

func removeWorktree(name string) error {
	wts, err := gitx.ListWorktrees(".")
	if err != nil {
		return err
	}

	var target *gitx.Worktree
	for i := range wts {
		if wts[i].Branch == name {
			target = &wts[i]
			break
		}
	}
	if target == nil {
		if abs, err := filepath.Abs(name); err == nil {
			for i := range wts {
				if samePath(wts[i].Path, abs) {
					target = &wts[i]
					break
				}
			}
		}
	}
	if target == nil {
		return fmt.Errorf("no worktree found for %q (see wt list)", name)
	}

	if target.Path == wts[0].Path {
		return fmt.Errorf("refusing to remove the main worktree at %s", target.Path)
	}
	if cur, err := gitx.Toplevel("."); err == nil && samePath(cur, target.Path) {
		return fmt.Errorf("cannot remove the worktree you are in; cd out of %s first", target.Path)
	}

	rmArgs := []string{"worktree", "remove"}
	if removeForce {
		rmArgs = append(rmArgs, "--force")
	}
	rmArgs = append(rmArgs, target.Path)
	if _, err := gitx.Run(".", rmArgs...); err != nil {
		return err
	}

	if target.Branch != "" {
		fmt.Fprintf(os.Stderr, "Removed worktree %s (branch %q kept)\n", target.Path, target.Branch)
	} else {
		fmt.Fprintf(os.Stderr, "Removed worktree %s\n", target.Path)
	}
	return nil
}

func init() {
	removeCmd.Flags().BoolVarP(&removeForce, "force", "f", false, "remove even if the worktree is dirty or locked")
	rootCmd.AddCommand(removeCmd)
}
