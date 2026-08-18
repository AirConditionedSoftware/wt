package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AirConditionedSoftware/wt/internal/gitx"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var removeForce bool

var removeCmd = &cobra.Command{
	Use:     "remove [branch...]",
	Aliases: []string{"rm"},
	Short:   "Remove worktrees",
	Long: `Remove the worktrees that have the given branches checked out. The branches
themselves are kept. Paths may be given instead of branch names.

With no arguments, an interactive picker lists the removable worktrees and
lets you select one or more to delete.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRemove(args)
	},
}

func runRemove(args []string) error {
	if len(args) == 0 {
		return removeInteractive()
	}
	for _, name := range args {
		if err := removeWorktree(name); err != nil {
			return err
		}
	}
	return nil
}

// removeInteractive shows a multi-select of every worktree except the main
// one and the one the user is standing in.
func removeInteractive() error {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New("interactive selection needs a terminal; pass branch names instead (see wt list)")
	}

	wts, err := gitx.ListWorktrees(".")
	if err != nil {
		return err
	}
	cur, _ := gitx.Toplevel(".")

	var candidates []gitx.Worktree
	for i, w := range wts {
		if i == 0 || w.Bare {
			continue
		}
		if cur != "" && samePath(cur, w.Path) {
			continue
		}
		candidates = append(candidates, w)
	}
	if len(candidates) == 0 {
		fmt.Fprintln(os.Stderr, "No removable worktrees (the main worktree and the one you're in don't count).")
		return nil
	}

	shas := make([]string, 0, len(candidates))
	for _, w := range candidates {
		shas = append(shas, w.Head)
	}
	// Commit info is decoration; fall back to bare branch labels if it fails
	// (e.g. a worktree on an unborn branch).
	infos, _ := gitx.CommitInfos(".", shas)

	width := 0
	for _, w := range candidates {
		if b := branchLabel(w); len(b) > width {
			width = len(b)
		}
	}
	opts := make([]huh.Option[string], 0, len(candidates))
	for _, w := range candidates {
		label := fmt.Sprintf("%-*s", width, branchLabel(w))
		if info, ok := infos[w.Head]; ok {
			label = fmt.Sprintf("%s  %s %s (%s)", label, info.ShortHash, truncate(info.Subject, 60), info.When)
		}
		opts = append(opts, huh.NewOption(label, w.Path))
	}

	var selected []string
	form := huh.NewForm(huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Select worktrees to remove").
			Options(opts...).
			Value(&selected),
	))
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return errors.New("aborted")
		}
		return err
	}
	if len(selected) == 0 {
		fmt.Fprintln(os.Stderr, "Nothing selected.")
		return nil
	}

	for _, path := range selected {
		if err := removeWorktree(path); err != nil {
			return err
		}
	}
	return nil
}

func branchLabel(w gitx.Worktree) string {
	if w.Branch != "" {
		return w.Branch
	}
	return "(detached)"
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
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
