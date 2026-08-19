package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// TrustFileName is the approval store for repo-sourced post_create commands.
// It lives at ~/.wt/trust.json regardless of $WT_CONFIG: it is per-user
// machine state, not configuration, and must not travel with a config file.
const TrustFileName = "trust.json"

// trustFile is the ~/.wt/trust.json schema, keyed by normalized main
// worktree path.
type trustFile struct {
	Repos map[string]trustRecord `json:"repos"`
}

// trustRecord is one repository's approval: the exact commands approved and
// when. Approval is re-required whenever the commands change.
type trustRecord struct {
	PostCreate []string `json:"post_create"`
	ApprovedAt string   `json:"approved_at"`
}

func trustPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".wt", TrustFileName), nil
}

// readTrust loads the store best-effort: a missing, unreadable or corrupt
// file reads as empty, so a damaged store re-prompts instead of wedging wt.
func readTrust() trustFile {
	path, err := trustPath()
	if err != nil {
		return trustFile{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return trustFile{}
	}
	var tf trustFile
	if err := json.Unmarshal(data, &tf); err != nil {
		return trustFile{}
	}
	return tf
}

// ApprovedPostCreate returns the post_create commands the user approved for
// the repository whose main worktree is at mainPath, and whether an approval
// record exists at all. It never fails: with no usable record the caller
// prompts for approval.
func ApprovedPostCreate(mainPath string) ([]string, bool) {
	rec, ok := readTrust().Repos[normalizePath(mainPath)]
	if !ok {
		return nil, false
	}
	return rec.PostCreate, true
}

// ApprovePostCreate records cmds as approved for the repository whose main
// worktree is at mainPath, replacing any previous approval. The store is
// written atomically (temp file plus rename) with mode 0600.
func ApprovePostCreate(mainPath string, cmds []string) error {
	path, err := trustPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("trust %s: %w", path, err)
	}

	tf := readTrust()
	if tf.Repos == nil {
		tf.Repos = make(map[string]trustRecord)
	}
	// Stored as given, so reading back compares equal to what was approved.
	tf.Repos[normalizePath(mainPath)] = trustRecord{
		PostCreate: cmds,
		ApprovedAt: time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(tf, "", "  ")
	if err != nil {
		return fmt.Errorf("trust %s: %w", path, err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, ".trust-*.json")
	if err != nil {
		return fmt.Errorf("trust %s: %w", path, err)
	}
	// Removing the temp file is a no-op once the rename has consumed it.
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("trust %s: %w", path, err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("trust %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("trust %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("trust %s: %w", path, err)
	}
	return nil
}
