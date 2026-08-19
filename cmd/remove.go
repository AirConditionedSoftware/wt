package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AirConditionedSoftware/wt/internal/config"
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

	// Commit info is decoration; entries fall back to bare branch labels
	// if it fails (e.g. a worktree on an unborn branch).
	infos := worktreeInfos(candidates)
	width := branchWidth(candidates)
	opts := make([]huh.Option[string], 0, len(candidates))
	for _, w := range candidates {
		opts = append(opts, huh.NewOption(worktreeEntry(w, width, infos, ansiBold, true), w.Path))
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

// confirmForceRemoval asks whether a dirty worktree should be removed
// anyway. Prompted per worktree, so multi-removals decide each one.
func confirmForceRemoval(w gitx.Worktree) (bool, error) {
	ok := false
	form := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title(fmt.Sprintf("%s has modified or untracked files. Remove anyway?", branchLabel(w))).
			Description(displayPath(w.Path)).
			Affirmative("Force remove").
			Negative("Skip").
			Value(&ok),
	))
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return false, errors.New("aborted")
		}
		return false, err
	}
	return ok, nil
}

func branchLabel(w gitx.Worktree) string {
	switch {
	case w.Branch != "":
		return w.Branch
	case w.Bare:
		return "(bare)"
	default:
		return "(detached)"
	}
}

// findWorktree matches name against worktree branches first, then paths.
func findWorktree(wts []gitx.Worktree, name string) *gitx.Worktree {
	for i := range wts {
		if wts[i].Branch == name {
			return &wts[i]
		}
	}
	if abs, err := filepath.Abs(name); err == nil {
		for i := range wts {
			if samePath(wts[i].Path, abs) {
				return &wts[i]
			}
		}
	}
	return nil
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

	// Display preferences and (later) workspace-file cleanup; a broken
	// config must not block removal.
	res, cfgErr := config.Resolve(wts[0].Path)
	if cfgErr == nil {
		applyDisplayConfig(res.Settings)
	}

	target := findWorktree(wts, name)
	if target == nil {
		return fmt.Errorf("no worktree found for %q (see wt list)", name)
	}

	if target.Path == wts[0].Path {
		return fmt.Errorf("refusing to remove the main worktree at %s", displayPath(target.Path))
	}
	if cur, err := gitx.Toplevel("."); err == nil && samePath(cur, target.Path) {
		return fmt.Errorf("cannot remove the worktree you are in; cd out of %s first", displayPath(target.Path))
	}

	force := removeForce
	if !force {
		// A dirty-worktree check failure (e.g. the directory is gone) just
		// means there is nothing to warn about; let git decide below.
		if dirty, err := gitx.IsDirty(target.Path); err == nil && dirty {
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return fmt.Errorf("worktree %s has modified or untracked files; re-run with --force", displayPath(target.Path))
			}
			ok, err := confirmForceRemoval(*target)
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintf(os.Stderr, "Skipped %s\n", displayPath(target.Path))
				return nil
			}
			force = true
		}
	}

	rmArgs := []string{"worktree", "remove"}
	if force {
		rmArgs = append(rmArgs, "--force")
	}
	rmArgs = append(rmArgs, target.Path)
	if _, err := gitx.Run(".", rmArgs...); err != nil {
		return err
	}

	// The wt-generated workspace file lives next to the worktree; clean it
	// up too so it doesn't orphan. Only when wt manages workspace files for
	// this repo — never delete files wt didn't create.
	if cfgErr == nil && res.VSCodeWorkspaceFileEnabled() {
		if ws := workspaceFilePath(res.Settings, target.Path, target.Branch); ws != "" {
			if err := os.Remove(ws); err == nil {
				fmt.Fprintf(os.Stderr, "Removed workspace file %s\n", displayPath(ws))
			}
		}
	}

	if target.Branch != "" {
		fmt.Fprintf(os.Stderr, "Removed worktree %s (branch %q kept)\n", displayPath(target.Path), target.Branch)
	} else {
		fmt.Fprintf(os.Stderr, "Removed worktree %s\n", displayPath(target.Path))
	}
	return nil
}

func init() {
	removeCmd.Flags().BoolVarP(&removeForce, "force", "f", false, "remove even if the worktree is dirty or locked")
	rootCmd.AddCommand(removeCmd)
}
