package cmd

import (
	"fmt"

	"github.com/AirConditionedSoftware/wt/internal/gitx"
)

// worktreeEntry builds the one-line worktree summary shared by wt list and
// the interactive pickers: padded branch (styled), then dimmed short head,
// optional commit subject, dimmed relative age, and locked/prunable tags.
// The tags are words, so they survive piping and --no-color.
func worktreeEntry(w gitx.Worktree, width int, infos map[string]gitx.CommitInfo, branchStyle string, withSubject bool) string {
	line := colorText(fmt.Sprintf("%-*s", width, branchLabel(w)), branchStyle)

	head := w.Head
	if len(head) > 8 {
		head = head[:8]
	}
	if head != "" {
		line += colorText("  "+head, ansiDim)
		info, ok := infos[w.Head]
		if ok && withSubject && info.Subject != "" {
			line += " " + truncate(info.Subject, 48)
		}
		if ok && info.When != "" {
			line += colorText(" ("+info.When+")", ansiDim)
		}
	}
	if w.Locked {
		line += colorText("  locked", ansiCyan)
	}
	if w.Prunable {
		line += colorText("  prunable", ansiYellow)
	}
	return line
}

// worktreeInfos fetches commit display info for the worktrees, skipping
// headless (bare) entries. Best-effort: on error the entries just render
// without ages.
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

// branchWidth returns the padding width for aligned branch columns.
func branchWidth(wts []gitx.Worktree) int {
	width := 0
	for _, w := range wts {
		if l := len(branchLabel(w)); l > width {
			width = l
		}
	}
	return width
}
