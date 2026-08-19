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
			return runRemove(args)
		}
		if len(args) > 0 {
			return fmt.Errorf("unknown command %q for \"wt\"", args[0])
		}
		return cmd.Help()
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output (also disabled when NO_COLOR is set or stdout is not a terminal)")
	rootCmd.Flags().BoolVarP(&rootRemove, "remove", "r", false, "remove worktrees (shorthand for wt remove); with no branch, select interactively")
	rootCmd.Flags().BoolVarP(&removeForce, "force", "f", false, "with -r: remove even if the worktree is dirty or locked")
}

func Execute(version string) {
	rootCmd.Version = version
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "wt: "+err.Error())
		os.Exit(1)
	}
}
