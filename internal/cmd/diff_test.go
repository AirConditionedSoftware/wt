package cmd

import "testing"

func TestDiffCommands(t *testing.T) {
	tests := []struct {
		name     string
		old, new []string
		want     string
	}{
		{
			name: "identical lists have no markers",
			old:  []string{"direnv allow", "npm ci"},
			new:  []string{"direnv allow", "npm ci"},
			want: "    direnv allow\n    npm ci",
		},
		{
			name: "pure addition",
			old:  []string{"direnv allow"},
			new:  []string{"direnv allow", "npm ci"},
			want: "    direnv allow\n  + npm ci",
		},
		{
			name: "pure removal",
			old:  []string{"direnv allow", "npm ci"},
			new:  []string{"direnv allow"},
			want: "    direnv allow\n  - npm ci",
		},
		{
			name: "replacement",
			old:  []string{"direnv allow", "npm install"},
			new:  []string{"direnv allow", "npm ci"},
			want: "    direnv allow\n  - npm install\n  + npm ci",
		},
		{
			name: "first approval marks everything added",
			old:  nil,
			new:  []string{"direnv allow", "npm ci"},
			want: "  + direnv allow\n  + npm ci",
		},
		{
			name: "emptied list marks everything removed",
			old:  []string{"direnv allow", "npm ci"},
			new:  nil,
			want: "  - direnv allow\n  - npm ci",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := diffCommands(tt.old, tt.new); got != tt.want {
				t.Errorf("diffCommands(%q, %q) =\n%s\nwant:\n%s", tt.old, tt.new, got, tt.want)
			}
		})
	}
}
