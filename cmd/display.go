package cmd

import (
	"os"
	"strings"
)

// displayPath abbreviates the user's home directory to ~ for human-facing
// output (tables, prompts, stderr messages). Machine-facing output — the
// path wt add prints on stdout, --json — keeps full paths.
func displayPath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || home == "/" {
		return p
	}
	if p == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(p, home+string(os.PathSeparator)); ok {
		return "~" + string(os.PathSeparator) + rest
	}
	return p
}
