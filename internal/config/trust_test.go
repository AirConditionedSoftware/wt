package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// trustHome isolates the trust store in a temp HOME and returns its path.
func trustHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return filepath.Join(home, ".th", TrustFileName)
}

func TestApprovePostCreateRoundTrip(t *testing.T) {
	path := trustHome(t)
	main := t.TempDir()

	tests := []struct {
		name string
		cmds []string
	}{
		{"one command", []string{"npm ci"}},
		{"several commands", []string{"direnv allow", "npm ci"}},
		{"empty list", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ApprovePostCreate(main, tt.cmds); err != nil {
				t.Fatalf("ApprovePostCreate: %v", err)
			}
			got, ok := ApprovedPostCreate(main)
			if !ok {
				t.Fatal("ApprovedPostCreate() found no record after approving")
			}
			if !reflect.DeepEqual(got, tt.cmds) {
				t.Errorf("ApprovedPostCreate() = %#v; want %#v", got, tt.cmds)
			}
		})
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("trust file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("trust file mode = %v; want 0600", perm)
	}

	var tf trustFile
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &tf); err != nil {
		t.Fatalf("trust file is not valid JSON: %v", err)
	}
	rec, ok := tf.Repos[normalizePath(main)]
	if !ok {
		t.Fatalf("trust file keyed by %v; want the normalized main path %q", tf.Repos, normalizePath(main))
	}
	if _, err := time.Parse(time.RFC3339, rec.ApprovedAt); err != nil {
		t.Errorf("approved_at = %q; want RFC3339: %v", rec.ApprovedAt, err)
	}
}

func TestApprovedPostCreateNoRecord(t *testing.T) {
	trustHome(t)
	other := t.TempDir()
	main := t.TempDir()

	if got, ok := ApprovedPostCreate(main); ok || got != nil {
		t.Errorf("ApprovedPostCreate() with no trust file = %#v, %v; want nil, false", got, ok)
	}
	if err := ApprovePostCreate(other, []string{"npm ci"}); err != nil {
		t.Fatal(err)
	}
	if got, ok := ApprovedPostCreate(main); ok || got != nil {
		t.Errorf("ApprovedPostCreate() for an unapproved repo = %#v, %v; want nil, false", got, ok)
	}
}

func TestApprovePostCreateOverwrites(t *testing.T) {
	trustHome(t)
	main := t.TempDir()
	other := t.TempDir()

	if err := ApprovePostCreate(other, []string{"make setup"}); err != nil {
		t.Fatal(err)
	}
	if err := ApprovePostCreate(main, []string{"npm install"}); err != nil {
		t.Fatal(err)
	}
	if err := ApprovePostCreate(main, []string{"npm ci"}); err != nil {
		t.Fatal(err)
	}

	got, ok := ApprovedPostCreate(main)
	if !ok || !reflect.DeepEqual(got, []string{"npm ci"}) {
		t.Errorf("ApprovedPostCreate() after re-approving = %#v, %v; want [npm ci], true", got, ok)
	}
	if got, ok := ApprovedPostCreate(other); !ok || !reflect.DeepEqual(got, []string{"make setup"}) {
		t.Errorf("ApprovedPostCreate() for the other repo = %#v, %v; want [make setup], true", got, ok)
	}
}

func TestApprovedPostCreateCorruptFile(t *testing.T) {
	path := trustHome(t)
	main := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"repos": {`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, ok := ApprovedPostCreate(main); ok || got != nil {
		t.Errorf("ApprovedPostCreate() with a corrupt trust file = %#v, %v; want nil, false", got, ok)
	}
	// A corrupt store must not wedge approvals: writing repairs it.
	if err := ApprovePostCreate(main, []string{"npm ci"}); err != nil {
		t.Fatalf("ApprovePostCreate over a corrupt trust file: %v", err)
	}
	if got, ok := ApprovedPostCreate(main); !ok || !reflect.DeepEqual(got, []string{"npm ci"}) {
		t.Errorf("ApprovedPostCreate() after repair = %#v, %v; want [npm ci], true", got, ok)
	}
}

func TestTrustFileIgnoresWTConfig(t *testing.T) {
	path := trustHome(t)
	t.Setenv(EnvVar, filepath.Join(t.TempDir(), "elsewhere.json"))
	main := t.TempDir()

	if err := ApprovePostCreate(main, []string{"npm ci"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("trust file should live at ~/.th/%s regardless of $%s: %v", TrustFileName, EnvVar, err)
	}
}
