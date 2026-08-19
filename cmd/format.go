package cmd

import (
	"path/filepath"
	"strconv"

	"github.com/AirConditionedSoftware/wt/internal/gitx"
)

// worktreeFacts are the live-state details the rich entry format shows.
type worktreeFacts struct {
	changes    int // pending files: staged, unstaged, and untracked
	changesOK  bool
	mergeKnown bool // merge status applies (not the default branch) and was computable
	merged     bool
}

// gatherFacts runs the per-worktree git queries behind the entry format.
// Everything is best-effort: a failing query just omits its segment.
func gatherFacts(w gitx.Worktree, defBranch string) worktreeFacts {
	var f worktreeFacts
	if w.Bare {
		return f
	}
	if n, err := gitx.ChangeCount(w.Path); err == nil {
		f.changes, f.changesOK = n, true
	}
	if defBranch != "" && w.Branch != "" && w.Branch != defBranch && w.Head != "" {
		ref := "refs/heads/" + defBranch
		if !gitx.LocalBranchExists(".", defBranch) {
			ref = "refs/remotes/origin/" + defBranch
		}
		f.mergeKnown = true
		f.merged = gitx.IsAncestor(".", w.Head, ref)
	}
	return f
}

// worktreeLines renders the two-line rich entry shared by list and the
// pickers:
//
//	name [branch]
//	hash subject (age) | N unstaged | ✓ merged into main
//
// Name is bright, the branch green, commit metadata gray, merge status
// green when merged and yellow when not; locked/prunable tags close the
// line. Any segment whose data is unavailable is omitted.
func worktreeLines(w gitx.Worktree, infos map[string]gitx.CommitInfo, defBranch string, f worktreeFacts, subjectLimit int) (string, string) {
	name := colorText(filepath.Base(w.Path), ansiBold)
	switch {
	case w.Branch != "":
		name += " " + colorText("["+w.Branch+"]", ansiGreen)
	case w.Bare:
		name += " " + colorText("[bare]", ansiGray)
	case w.Detached:
		name += " " + colorText("[detached]", ansiGray)
	}

	var meta string
	sep := colorText(" | ", ansiGray)
	if head := w.Head; head != "" {
		if len(head) > 8 {
			head = head[:8]
		}
		commit := head
		if info, ok := infos[w.Head]; ok {
			if info.Subject != "" {
				commit += " " + truncate(info.Subject, subjectLimit)
			}
			if info.When != "" {
				commit += " (" + info.When + ")"
			}
		}
		meta = colorText(commit, ansiGray)
	}
	if f.changesOK {
		if meta != "" {
			meta += sep
		}
		meta += strconv.Itoa(f.changes) + " unstaged"
	}
	if f.mergeKnown {
		if meta != "" {
			meta += sep
		}
		if f.merged {
			meta += colorText("✓ merged into "+defBranch, ansiGreen)
		} else {
			meta += colorText("✗ not merged into "+defBranch, ansiYellow)
		}
	}
	if w.Locked {
		if meta != "" {
			meta += sep
		}
		meta += colorText("locked", ansiCyan)
	}
	if w.Prunable {
		if meta != "" {
			meta += sep
		}
		meta += colorText("prunable", ansiYellow)
	}
	return name, meta
}

// worktreeOption renders the two-line rich entry as a picker label — the
// name/branch line with the metadata line underneath, matching wt list.
func worktreeOption(w gitx.Worktree, infos map[string]gitx.CommitInfo, defBranch string, subjectLimit int) string {
	line1, line2 := worktreeLines(w, infos, defBranch, gatherFacts(w, defBranch), subjectLimit)
	if line2 == "" {
		return line1
	}
	return line1 + "\n  " + line2
}

// worktreeInfos fetches commit display info for the worktrees, skipping
// headless (bare) entries. Best-effort: on error the entries just render
// without subjects and ages.
func worktreeInfos(wts []gitx.Worktree) map[string]gitx.CommitInfo {
	var shas []string
	for _, w := range wts {
		if w.Head != "" {
			shas = append(shas, w.Head)
		}
	}
	infos, _ := gitx.CommitInfos(".", shas)
	return infos
}
