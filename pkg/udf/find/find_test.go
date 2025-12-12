package find

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindFiles(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir, err := os.MkdirTemp("", "pwrq-find-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	
	// Create test files and directories
	testFiles := []string{
		"file1.txt",
		"file2.txt",
		"subdir/file3.txt",
		"subdir/nested/file4.txt",
	}
	
	testDirs := []string{
		"subdir",
		"subdir/nested",
		"emptydir",
	}
	
	for _, dir := range testDirs {
		if err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755); err != nil {
			t.Fatalf("Failed to create dir %s: %v", dir, err)
		}
	}
	
	for _, file := range testFiles {
		filePath := filepath.Join(tmpDir, file)
		if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", file, err)
		}
	}
	
	tests := []struct {
		name    string
		args    []any
		wantErr bool
		check   func([]any) bool
	}{
		{
			name:    "find all",
			args:    []any{tmpDir},
			wantErr: false,
			check: func(results []any) bool {
				// Should find files and directories
				return len(results) > 0
			},
		},
		{
			name:    "find files only",
			args:    []any{tmpDir, "file"},
			wantErr: false,
			check: func(results []any) bool {
				// Should only find files
				for _, r := range results {
					path := r.(string)
					info, err := os.Stat(path)
					if err != nil {
						return false
					}
					if info.IsDir() {
						return false
					}
				}
				return len(results) > 0
			},
		},
		{
			name:    "find dirs only",
			args:    []any{tmpDir, "dir"},
			wantErr: false,
			check: func(results []any) bool {
				// Should only find directories
				for _, r := range results {
					path := r.(string)
					info, err := os.Stat(path)
					if err != nil {
						return false
					}
					if !info.IsDir() {
						return false
					}
				}
				return len(results) > 0
			},
		},
		{
			name:    "find with maxdepth 1",
			args:    []any{tmpDir, map[string]any{"maxdepth": float64(1)}},
			wantErr: false,
			check: func(results []any) bool {
				// Should only find items at depth 0 and 1
				return len(results) > 0
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := parseFindArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseFindArgs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			
			results, err := findFiles(opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("findFiles() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			
			if !tt.check(results) {
				t.Errorf("findFiles() results did not pass check: %v", results)
			}
		})
	}
}

func TestParseFindArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []any
		want    FindOptions
		wantErr bool
	}{
		{
			name: "simple path",
			args: []any{"/tmp"},
			want: FindOptions{
				Path:     "/tmp",
				Type:     "",
				MaxDepth: -1,
				MinDepth: 0,
			},
			wantErr: false,
		},
		{
			name: "path with type",
			args: []any{"/tmp", "file"},
			want: FindOptions{
				Path:     "/tmp",
				Type:     "file",
				MaxDepth: -1,
				MinDepth: 0,
			},
			wantErr: false,
		},
		{
			name: "path with maxdepth",
			args: []any{"/tmp", float64(2)},
			want: FindOptions{
				Path:     "/tmp",
				Type:     "",
				MaxDepth: 2,
				MinDepth: 0,
			},
			wantErr: false,
		},
		{
			name: "path with options object",
			args: []any{"/tmp", map[string]any{
				"type":     "file",
				"maxdepth": float64(3),
				"mindepth": float64(1),
			}},
			want: FindOptions{
				Path:     "/tmp",
				Type:     "file",
				MaxDepth: 3,
				MinDepth: 1,
			},
			wantErr: false,
		},
		{
			name:    "no arguments",
			args:    []any{},
			wantErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFindArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseFindArgs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got.Path != tt.want.Path {
					t.Errorf("parseFindArgs() Path = %v, want %v", got.Path, tt.want.Path)
				}
				if got.Type != tt.want.Type {
					t.Errorf("parseFindArgs() Type = %v, want %v", got.Type, tt.want.Type)
				}
				if got.MaxDepth != tt.want.MaxDepth {
					t.Errorf("parseFindArgs() MaxDepth = %v, want %v", got.MaxDepth, tt.want.MaxDepth)
				}
				if got.MinDepth != tt.want.MinDepth {
					t.Errorf("parseFindArgs() MinDepth = %v, want %v", got.MinDepth, tt.want.MinDepth)
				}
			}
		})
	}
}

