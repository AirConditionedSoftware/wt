package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
		if res, err := config.Resolve(wts[0].Path); err == nil {
			applyDisplayConfig(res.Settings)
		}
		if listJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(wts)
		}

		current, _ := gitx.Toplevel(".")
		width := 0
		for _, w := range wts {
			if l := len(branchLabel(w)); l > width {
				width = l
			}
		}

		// Two lines per worktree — branch and head, path indented below —
		// so the output fits a terminal regardless of path length.
		var b strings.Builder
		var styles []string
		for _, w := range wts {
			marker := " "
			if current != "" && samePath(current, w.Path) {
				marker = "*"
			}
			head := w.Head
			if len(head) > 8 {
				head = head[:8]
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
			line := strings.TrimRight(fmt.Sprintf("%s %-*s  %s", marker, width, branchLabel(w), head), " ")
			fmt.Fprintf(&b, "%s\n    %s\n", line, displayPath(w.Path))
			styles = append(styles, style, style)
		}
		return printStyled(os.Stdout, b.String(), styles)
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
