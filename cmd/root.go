package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "wt",
	Short: "Manage git worktrees",
	Long: `wt manages git worktrees: list the worktrees of the current repository and
create new ones in locations driven by ~/.wt/wt.json (or $WT_CONFIG).`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute(version string) {
	rootCmd.Version = version
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "wt: "+err.Error())
		os.Exit(1)
	}
}
