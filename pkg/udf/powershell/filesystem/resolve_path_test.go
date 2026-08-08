package filesystem

import (
	"os"
	"testing"
)

func TestParseResolvePathArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []any
		wantPath    string
		wantLiteral bool
	}{
		{
			name:     "no args",
			args:     []any{},
			wantPath: ".",
		},
		{
			name:     "path only",
			args:     []any{"/tmp"},
			wantPath: "/tmp",
		},
		{
			name: "named parameters",
			args: []any{map[string]any{
				"Path":    "/tmp",
				"Literal": true,
			}},
			wantPath:    "/tmp",
			wantLiteral: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := parseResolvePathArgs(tt.args)
			if err != nil {
				t.Fatalf("parseResolvePathArgs() error = %v", err)
			}
			if opts.Path != tt.wantPath {
				t.Errorf("Path = %v, want %v", opts.Path, tt.wantPath)
			}
			if opts.Literal != tt.wantLiteral {
				t.Errorf("Literal = %v, want %v", opts.Literal, tt.wantLiteral)
			}
		})
	}
}

func TestResolvePath(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		opts        ResolvePathOptions
		wantSuccess bool
	}{
		{
			name: "resolve existing directory",
			opts: ResolvePathOptions{
				Path: tmpDir,
			},
			wantSuccess: true,
		},
		{
			name: "resolve current directory",
			opts: ResolvePathOptions{
				Path: ".",
			},
			wantSuccess: true,
		},
		{
			name: "resolve non-existent path",
			opts: ResolvePathOptions{
				Path: "/nonexistent/path/that/does/not/exist",
			},
			wantSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := resolvePath(tt.opts)
			if tt.wantSuccess && err != nil {
				t.Fatalf("resolvePath() error = %v", err)
			}
			if !tt.wantSuccess && err == nil {
				t.Fatal("resolvePath() expected error but got none")
			}
			if tt.wantSuccess && len(results) == 0 {
				t.Fatal("resolvePath() expected results but got none")
			}
		})
	}
}

func TestResolvePathTildeExpansion(t *testing.T) {
	opts := ResolvePathOptions{
		Path: "~",
	}

	results, err := resolvePath(opts)
	if err != nil {
		t.Fatalf("resolvePath() error = %v", err)
	}

	if len(results) == 0 {
		t.Fatal("resolvePath() expected results but got none")
	}

	// Get home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error = %v", err)
	}

	// Verify the result matches home directory
	result := results[0].(map[string]any)
	if path, ok := result["Path"].(string); ok {
		if path != homeDir {
			t.Errorf("resolvePath(~) = %v, want %v", path, homeDir)
		}
	}
}
