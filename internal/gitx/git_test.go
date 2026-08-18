package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func testRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	env := append(os.Environ(),
		"HOME="+dir,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=wt-test",
		"GIT_AUTHOR_EMAIL=wt@test.invalid",
		"GIT_COMMITTER_NAME=wt-test",
		"GIT_COMMITTER_EMAIL=wt@test.invalid",
	)
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "first commit")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "second commit with a longer subject line")
	return dir
}

func TestCommitInfos(t *testing.T) {
	dir := testRepo(t)
	head, err := Run(dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	prev, err := Run(dir, "rev-parse", "HEAD~1")
	if err != nil {
		t.Fatal(err)
	}

	infos, err := CommitInfos(dir, []string{head, prev, head})
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 2 {
		t.Fatalf("got %d infos; want 2 (duplicates collapsed): %+v", len(infos), infos)
	}
	for sha, wantSubject := range map[string]string{
		head: "second commit with a longer subject line",
		prev: "first commit",
	} {
		info, ok := infos[sha]
		if !ok {
			t.Fatalf("no info for %s", sha)
		}
		if info.Subject != wantSubject {
			t.Errorf("subject for %s = %q; want %q", sha, info.Subject, wantSubject)
		}
		if !strings.HasPrefix(sha, info.ShortHash) || info.ShortHash == "" {
			t.Errorf("short hash %q is not a prefix of %s", info.ShortHash, sha)
		}
		if info.When == "" {
			t.Errorf("empty relative date for %s", sha)
		}
	}
}

func TestCommitInfosEmpty(t *testing.T) {
	infos, err := CommitInfos(t.TempDir(), nil)
	if err != nil || len(infos) != 0 {
		t.Errorf("CommitInfos with no shas = %v, %v; want empty map, nil", infos, err)
	}
}

func TestCommitInfosBadSha(t *testing.T) {
	dir := testRepo(t)
	if _, err := CommitInfos(dir, []string{"0000000000000000000000000000000000000000"}); err == nil {
		t.Error("CommitInfos with unknown sha should fail")
	}
}
