package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AirConditionedSoftware/wt/internal/config"
	"github.com/AirConditionedSoftware/wt/internal/gitx"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a repo-local .wt.json",
	Long: `Create a starter .wt.json at the root of the repository's main worktree —
even when run from a linked worktree, since that is where wt reads it from.
The file holds per-repo settings that override the global config and its
repos entry; it can be committed to share with a team.

The created path is the only output on stdout, so $EDITOR "$(wt init)"
opens it directly.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		wts, err := gitx.ListWorktrees(".")
		if err != nil {
			return err
		}
		mainPath := wts[0].Path
		path := filepath.Join(mainPath, config.LocalFileName)
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists; edit it directly", displayPath(path))
		}

		starter := config.LocalConfig{Name: filepath.Base(mainPath)}
		data, err := json.MarshalIndent(starter, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
			return err
		}
		// Prove the starter loads through the real resolver.
		if _, err := config.Resolve(mainPath); err != nil {
			return err
		}

		fmt.Fprintf(os.Stderr, "Created %s\n", displayPath(path))
		fmt.Fprintln(os.Stderr, "Its values override the global config and its repos entry for this repo.")
		fmt.Fprintln(os.Stderr, "Fields: worktree_dir, default_base, branch_prefix, prefix_separator,")
		fmt.Fprintln(os.Stderr, "  copy_hooks, copy_files, workspace_paths, full_paths, post_create,")
		fmt.Fprintln(os.Stderr, "  vscode_* — see the README's Configuration section.")
		fmt.Fprintln(os.Stderr, "If committed, post_create commands from this file need one-time approval on wt add.")
		fmt.Println(path)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
