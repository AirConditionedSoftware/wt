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

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Print the config file location and content",
	Long: `Print the config file location and its content. The location goes to stderr
and the content to stdout, so wt config | jq works. If no file exists at the
default location, the built-in defaults are printed instead.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, explicit, err := config.Path()
		if err != nil {
			return err
		}
		src := "default location"
		if explicit {
			src = "from $" + config.EnvVar
		}

		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			if explicit {
				return fmt.Errorf("config %s (%s) does not exist", displayPath(path), src)
			}
			fmt.Fprintf(os.Stderr, "%s (%s) does not exist; built-in defaults apply:\n", displayPath(path), src)
			def := config.File{Settings: config.Settings{WorktreeDir: config.DefaultWorktreeDir}}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(def)
		}
		if err != nil {
			return err
		}

		fmt.Fprintf(os.Stderr, "%s (%s)\n", displayPath(path), src)
		os.Stdout.Write(data)
		if len(data) > 0 && data[len(data)-1] != '\n' {
			fmt.Println()
		}

		// A repo's own .wtrc is part of its effective config, so print it
		// as a second document (the stdout stream stays jq-parseable).
		// Outside a repository there is nothing to look for.
		if wts, err := gitx.ListWorktrees("."); err == nil && len(wts) > 0 {
			mainPath := wts[0].Path
			localPath := filepath.Join(mainPath, config.LocalFileName)
			if local, err := os.ReadFile(localPath); err == nil {
				fmt.Fprintf(os.Stderr, "%s (repo-local)\n", displayPath(localPath))
				os.Stdout.Write(local)
				if len(local) > 0 && local[len(local)-1] != '\n' {
					fmt.Println()
				}
				// Resolve parses the repo-local file, so wt config validates
				// it just like the global one.
				if _, err := config.Resolve(mainPath); err != nil {
					return err
				}
			}
		}

		// Surface parse errors so wt config doubles as a validity check.
		if _, err := config.Load(); err != nil {
			return err
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
}
