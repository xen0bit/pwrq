package filesystem

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseTestPathArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []any
		wantPath  string
		wantType  string
		wantValid bool
		wantErr   bool
	}{
		{
			name:    "no args",
			args:    []any{},
			wantErr: true,
		},
		{
			name:     "path only",
			args:     []any{"/tmp"},
			wantPath: "/tmp",
		},
		{
			name:     "path with Leaf type",
			args:     []any{"/tmp", "Leaf"},
			wantPath: "/tmp",
			wantType: "Leaf",
		},
		{
			name:     "path with Container type",
			args:     []any{"/tmp", "Container"},
			wantPath: "/tmp",
			wantType: "Container",
		},
		{
			name:      "PSObject wrapped path",
			args:      []any{map[string]any{"Path": "/tmp", "PathType": "Leaf", "IsValid": true}},
			wantPath:  "/tmp",
			wantType:  "Leaf",
			wantValid: true,
		},
		{
			name:     "mixed args with PSObject",
			args:     []any{"/tmp", map[string]any{"PathType": "Container"}},
			wantPath: "/tmp",
			wantType: "Container",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := parseTestPathArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTestPathArgs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if opts.Path != tt.wantPath {
				t.Errorf("Path = %v, want %v", opts.Path, tt.wantPath)
			}
			if opts.PathType != tt.wantType {
				t.Errorf("PathType = %v, want %v", opts.PathType, tt.wantType)
			}
			if opts.IsValid != tt.wantValid {
				t.Errorf("IsValid = %v, want %v", opts.IsValid, tt.wantValid)
			}
		})
	}
}

func TestIsValidPathSyntax(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"empty path", "", false},
		{"null byte", "/tmp\x00file", false},
		{"valid unix path", "/tmp/test/file.go", true},
		{"valid relative path", "tmp/test", true},
		{"UNC path", "\\\\server\\share", true},
		{"UNC forward slash", "//server/share", true},
		{"colon in middle", "/tmp:invalid", false},
		{"colon at start (drive letter)", "C:/tmp", true},
		{"angle brackets", "/tmp<file", false},
		{"pipe character", "/tmp|file", false},
		{"question mark", "/tmp?file", false},
		{"asterisk", "/tmp*file", false},
		{"quote", "/tmp\"file", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidPathSyntax(tt.path)
			if got != tt.want {
				t.Errorf("isValidPathSyntax(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestTestPath(t *testing.T) {
	// Create temporary test structure
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testSubDir := filepath.Join(tmpDir, "subdir")
	testSubFile := filepath.Join(testSubDir, "nested.txt")

	// Create test files
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	if err := os.Mkdir(testSubDir, 0755); err != nil {
		t.Fatalf("Failed to create test subdir: %v", err)
	}
	if err := os.WriteFile(testSubFile, []byte("nested"), 0644); err != nil {
		t.Fatalf("Failed to create test subfile: %v", err)
	}

	tests := []struct {
		name    string
		opts    TestPathOptions
		want    bool
		wantErr bool
	}{
		{
			name: "empty path returns false",
			opts: TestPathOptions{Path: ""},
			want: false,
		},
		{
			name: "existing file - any type",
			opts: TestPathOptions{Path: testFile},
			want: true,
		},
		{
			name: "existing file - Leaf type",
			opts: TestPathOptions{Path: testFile, PathType: "Leaf"},
			want: true,
		},
		{
			name: "existing file - Container type (should be false)",
			opts: TestPathOptions{Path: testFile, PathType: "Container"},
			want: false,
		},
		{
			name: "existing directory - any type",
			opts: TestPathOptions{Path: tmpDir},
			want: true,
		},
		{
			name: "existing directory - Leaf type",
			opts: TestPathOptions{Path: tmpDir, PathType: "Leaf"},
			want: true,
		},
		{
			name: "existing directory - Container type",
			opts: TestPathOptions{Path: tmpDir, PathType: "Container"},
			want: true,
		},
		{
			name: "non-existent path",
			opts: TestPathOptions{Path: filepath.Join(tmpDir, "nonexistent")},
			want: false,
		},
		{
			name: "IsValid mode - valid syntax",
			opts: TestPathOptions{Path: "/tmp/valid/path", IsValid: true},
			want: true,
		},
		{
			name: "IsValid mode - null byte",
			opts: TestPathOptions{Path: "/tmp\x00invalid", IsValid: true},
			want: false,
		},
		{
			name: "IsValid mode - empty path",
			opts: TestPathOptions{Path: "", IsValid: true},
			want: false,
		},
		{
			name: "IsValid mode - invalid chars",
			opts: TestPathOptions{Path: "/tmp<invalid", IsValid: true},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRaw, err := testPath(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("testPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			got, ok := gotRaw.(bool)
			if !ok {
				t.Errorf("testPath() returned non-bool type: %T", gotRaw)
				return
			}
			if got != tt.want {
				t.Errorf("testPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTestPathUNCPaths(t *testing.T) {
	// UNC path handling test - on Unix these may not exist but syntax should be valid
	opts := TestPathOptions{Path: "\\\\server\\share", IsValid: true}
	gotRaw, err := testPath(opts)
	if err != nil {
		t.Errorf("testPath() unexpected error = %v", err)
		return
	}
	got, ok := gotRaw.(bool)
	if !ok {
		t.Errorf("testPath() returned non-bool type: %T", gotRaw)
		return
	}
	// UNC syntax is valid, but path likely doesn't exist
	// We're testing IsValid mode, so syntax validity is what matters
	if !got {
		t.Errorf("testPath() UNC path syntax should be valid, got %v", got)
	}
}

func TestTestPathEdgeCases(t *testing.T) {
	tmpDir := t.TempDir()

	// Test with cleaned path
	dirtyPath := filepath.Join(tmpDir, "..", filepath.Base(tmpDir), "test")
	testFile := filepath.Join(tmpDir, "test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	opts := TestPathOptions{Path: dirtyPath}
	gotRaw, err := testPath(opts)
	if err != nil {
		t.Errorf("testPath() error = %v", err)
		return
	}
	got, ok := gotRaw.(bool)
	if !ok {
		t.Errorf("testPath() returned non-bool type: %T", gotRaw)
		return
	}
	if !got {
		t.Errorf("testPath() cleaned path should exist, got %v", got)
	}
}
