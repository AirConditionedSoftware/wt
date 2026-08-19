package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/AirConditionedSoftware/wt/internal/config"
	"github.com/AirConditionedSoftware/wt/internal/gitx"
	"github.com/spf13/cobra"
)

var listJSON bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List worktrees of the current repository",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		wts, err := gitx.ListWorktrees(".")
		if err != nil {
			return err
		}
		// Display preferences only — a broken config must not break list.
		if cfg, err := config.Load(); err == nil {
			s, _ := cfg.ForPath(wts[0].Path)
			applyDisplayConfig(s)
		}
		if listJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(wts)
		}

		current, _ := gitx.Toplevel(".")
		var buf bytes.Buffer
		tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
		styles := []string{ansiBold}
		fmt.Fprintln(tw, "  BRANCH\tPATH\tHEAD\tSTATUS")
		for _, w := range wts {
			marker := " "
			if current != "" && samePath(current, w.Path) {
				marker = "*"
			}
			branch := w.Branch
			switch {
			case w.Bare:
				branch = "(bare)"
			case w.Detached:
				branch = "(detached)"
			}
			head := w.Head
			if len(head) > 8 {
				head = head[:8]
			}
			var status []string
			if w.Locked {
				status = append(status, "locked")
			}
			if w.Prunable {
				status = append(status, "prunable")
			}
			style := ""
			switch {
			case w.Prunable:
				style = ansiYellow
			case w.Locked:
				style = ansiCyan
			case marker == "*":
				style = ansiGreen
			}
			styles = append(styles, style)
			fmt.Fprintf(tw, "%s %s\t%s\t%s\t%s\n", marker, branch, displayPath(w.Path), head, strings.Join(status, ","))
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		return printStyled(os.Stdout, buf.String(), styles)
	},
}

// samePath compares two paths after resolving symlinks (e.g. /tmp vs
// /private/tmp on macOS).
func samePath(a, b string) bool {
	if r, err := filepath.EvalSymlinks(a); err == nil {
		a = r
	}
	if r, err := filepath.EvalSymlinks(b); err == nil {
		b = r
	}
	return a == b
}

func init() {
	listCmd.Flags().BoolVar(&listJSON, "json", false, "output as JSON")
	rootCmd.AddCommand(listCmd)
}
