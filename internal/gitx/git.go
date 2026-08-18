// Package gitx shells out to the git binary. It deliberately avoids go-git:
// its worktree support is incomplete, while `git worktree list --porcelain`
// is a stable scripting interface.
package gitx

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Run executes git with args in dir and returns stdout with the trailing
// newline removed. On failure the error carries git's stderr.
func Run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimRight(stdout.String(), "\n"), nil
}

// Toplevel returns the root of the worktree containing dir.
func Toplevel(dir string) (string, error) {
	return Run(dir, "rev-parse", "--show-toplevel")
}

// LocalBranchExists reports whether refs/heads/<branch> exists.
func LocalBranchExists(dir, branch string) bool {
	_, err := Run(dir, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// RemoteBranchExists reports whether origin has branch, fetching once if the
// remote-tracking ref is not yet known locally. A failed fetch (e.g. no
// origin remote) just means the branch is treated as not existing remotely.
func RemoteBranchExists(dir, branch string) bool {
	ref := "refs/remotes/origin/" + branch
	if _, err := Run(dir, "rev-parse", "--verify", "--quiet", ref); err == nil {
		return true
	}
	if _, err := Run(dir, "fetch", "--quiet", "origin"); err != nil {
		return false
	}
	_, err := Run(dir, "rev-parse", "--verify", "--quiet", ref)
	return err == nil
}
