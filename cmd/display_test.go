package cmd

import "testing"

func TestDisplayPath(t *testing.T) {
	t.Setenv("HOME", "/Users/dev")
	tests := []struct{ in, want string }{
		{"/Users/dev", "~"},
		{"/Users/dev/worktrees/myapp/fix-login", "~/worktrees/myapp/fix-login"},
		{"/Users/developer/code", "/Users/developer/code"},
		{"/opt/homebrew/bin", "/opt/homebrew/bin"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := displayPath(tt.in); got != tt.want {
			t.Errorf("displayPath(%q) = %q; want %q", tt.in, got, tt.want)
		}
	}
}
