package filesystem

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseGetChildItemArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []any
		wantPath string
		wantFilter string
		wantRecurse bool
		wantInclude []string
		wantExclude []string
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
			name:     "path and filter",
			args:     []any{"/tmp", "*.go"},
			wantPath: "/tmp",
			wantFilter: "*.go",
		},
		{
			name: "named parameters with include",
			args: []any{map[string]any{
				"Path": "/tmp",
				"Include": []any{"*.go", "*.md"},
				"Exclude": []any{"*.tmp"},
			}},
			wantPath: "/tmp",
			wantInclude: []string{"*.go", "*.md"},
			wantExclude: []string{"*.tmp"},
		},
		{
			name: "recurse flag",
			args: []any{map[string]any{
				"Path": "/tmp",
				"Recurse": true,
			}},
			wantPath: "/tmp",
			wantRecurse: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := parseGetChildItemArgs(tt.args)
			if err != nil {
				t.Fatalf("parseGetChildItemArgs() error = %v", err)
			}
			if opts.Path != tt.wantPath {
				t.Errorf("Path = %v, want %v", opts.Path, tt.wantPath)
			}
			if opts.Filter != tt.wantFilter {
				t.Errorf("Filter = %v, want %v", opts.Filter, tt.wantFilter)
			}
			if opts.Recurse != tt.wantRecurse {
				t.Errorf("Recurse = %v, want %v", opts.Recurse, tt.wantRecurse)
			}
			if len(tt.wantInclude) > 0 {
				if len(opts.Include) != len(tt.wantInclude) {
					t.Errorf("Include length = %v, want %v", len(opts.Include), len(tt.wantInclude))
				}
			}
			if len(tt.wantExclude) > 0 {
				if len(opts.Exclude) != len(tt.wantExclude) {
					t.Errorf("Exclude length = %v, want %v", len(opts.Exclude), len(tt.wantExclude))
				}
			}
		})
	}
}

func TestGetChildItems(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()

	// Create some test files
	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("content1"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(tmpDir, ".hidden"), []byte("hidden"), 0644)

	// Create a subdirectory
	subDir := filepath.Join(tmpDir, "subdir")
	os.MkdirAll(subDir, 0755)
	os.WriteFile(filepath.Join(subDir, "file3.txt"), []byte("content3"), 0644)

	tests := []struct {
		name        string
		opts        GetChildItemOptions
		wantMinCount int
		wantMaxCount int
	}{
		{
			name: "list root directory",
			opts: GetChildItemOptions{
				Path: tmpDir,
			},
			wantMinCount: 3, // file1.txt, file2.go, .hidden, subdir (but .hidden is hidden)
			wantMaxCount: 4,
		},
		{
			name: "filter by extension",
			opts: GetChildItemOptions{
				Path: tmpDir,
				Filter: "*.go",
			},
			wantMinCount: 1,
			wantMaxCount: 1,
		},
		{
			name: "force includes hidden",
			opts: GetChildItemOptions{
				Path: tmpDir,
				Force: true,
			},
			wantMinCount: 4,
			wantMaxCount: 4,
		},
		{
			name: "recursive",
			opts: GetChildItemOptions{
				Path: tmpDir,
				Recurse: true,
			},
			wantMinCount: 4, // includes subdir/file3.txt
			wantMaxCount: 5,
		},
		{
			name: "directory only",
			opts: GetChildItemOptions{
				Path: tmpDir,
				Directory: true,
			},
			wantMinCount: 1, // only subdir
			wantMaxCount: 1,
		},
		{
			name: "file only",
			opts: GetChildItemOptions{
				Path: tmpDir,
				File: true,
			},
			wantMinCount: 2, // file1.txt, file2.go (not .hidden)
			wantMaxCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := getChildItems(tt.opts)
			if err != nil {
				t.Fatalf("getChildItems() error = %v", err)
			}
			if len(results) < tt.wantMinCount {
				t.Errorf("got %d results, want at least %d", len(results), tt.wantMinCount)
			}
			if len(results) > tt.wantMaxCount {
				t.Errorf("got %d results, want at most %d", len(results), tt.wantMaxCount)
			}
		})
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"*.go", "file.go", true},
		{"*.go", "file.txt", false},
		{"file?.txt", "file1.txt", true},
		{"file?.txt", "file10.txt", false},
		{"*.txt", "file.txt", true},
		{"*.txt", "file.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.name, func(t *testing.T) {
			got := matchPattern(tt.pattern, tt.name)
			if got != tt.want {
				t.Errorf("matchPattern(%q, %q) = %v, want %v", tt.pattern, tt.name, got, tt.want)
			}
		})
	}
}
