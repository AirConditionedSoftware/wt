package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootRemove bool

var rootCmd = &cobra.Command{
	Use:   "wt",
	Short: "Manage git worktrees",
	Long: `wt manages git worktrees: list the worktrees of the current repository and
create new ones in locations driven by ~/.wt/wt.json (or $WT_CONFIG).`,
	SilenceUsage:  true,
	SilenceErrors: true,
	Args:          cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if rootRemove {
			if len(args) != 1 {
				return fmt.Errorf("-r requires exactly one branch argument")
			}
			return removeWorktree(args[0])
		}
		if len(args) > 0 {
			return fmt.Errorf("unknown command %q for \"wt\"", args[0])
		}
		return cmd.Help()
	},
}

func init() {
	rootCmd.Flags().BoolVarP(&rootRemove, "remove", "r", false, "remove the worktree for the given branch (shorthand for wt remove)")
	rootCmd.Flags().BoolVarP(&removeForce, "force", "f", false, "with -r: remove even if the worktree is dirty or locked")
}

func Execute(version string) {
	rootCmd.Version = version
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "wt: "+err.Error())
		os.Exit(1)
	}
}
