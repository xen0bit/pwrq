package filesystem

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseNewItemArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []any
		wantPath   string
		wantItemType string
		wantForce  bool
	}{
		{
			name:       "no args",
			args:       []any{},
			wantPath:   ".",
			wantItemType: "file",
		},
		{
			name:       "path only",
			args:       []any{"/tmp/test.txt"},
			wantPath:   "/tmp/test.txt",
			wantItemType: "file",
		},
		{
			name: "named parameters",
			args: []any{map[string]any{
				"Path": "/tmp",
				"Name": "test.txt",
				"ItemType": "file",
				"Force": true,
			}},
			wantPath:   "/tmp",
			wantItemType: "file",
			wantForce:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := parseNewItemArgs(tt.args)
			if err != nil {
				t.Fatalf("parseNewItemArgs() error = %v", err)
			}
			if opts.Path != tt.wantPath {
				t.Errorf("Path = %v, want %v", opts.Path, tt.wantPath)
			}
			if opts.ItemType != tt.wantItemType {
				t.Errorf("ItemType = %v, want %v", opts.ItemType, tt.wantItemType)
			}
			if opts.Force != tt.wantForce {
				t.Errorf("Force = %v, want %v", opts.Force, tt.wantForce)
			}
		})
	}
}

func TestNewItem(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		opts        NewItemOptions
		wantSuccess bool
		checkExists bool
	}{
		{
			name: "create file",
			opts: NewItemOptions{
				Path: filepath.Join(tmpDir, "test.txt"),
				ItemType: "file",
			},
			wantSuccess: true,
			checkExists: true,
		},
		{
			name: "create directory",
			opts: NewItemOptions{
				Path: filepath.Join(tmpDir, "testdir"),
				ItemType: "directory",
			},
			wantSuccess: true,
			checkExists: true,
		},
		{
			name: "create file with content",
			opts: NewItemOptions{
				Path: filepath.Join(tmpDir, "content.txt"),
				ItemType: "file",
				Value: "Hello, World!",
			},
			wantSuccess: true,
			checkExists: true,
		},
		{
			name: "create with force (overwrite)",
			opts: NewItemOptions{
				Path: filepath.Join(tmpDir, "test.txt"),
				ItemType: "file",
				Force: true,
			},
			wantSuccess: true,
			checkExists: true,
		},
		{
			name: "create without force (should fail)",
			opts: NewItemOptions{
				Path: filepath.Join(tmpDir, "test.txt"),
				ItemType: "file",
				Force: false,
			},
			wantSuccess: false,
			checkExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := newItem(tt.opts)
			if tt.wantSuccess && err != nil {
				t.Fatalf("newItem() error = %v", err)
			}
			if !tt.wantSuccess && err == nil {
				t.Fatal("newItem() expected error but got none")
			}
			if tt.wantSuccess && result == nil {
				t.Fatal("newItem() expected result but got nil")
			}
			if tt.checkExists {
				if _, err := os.Stat(tt.opts.Path); os.IsNotExist(err) {
					t.Errorf("newItem() created item does not exist: %s", tt.opts.Path)
				}
			}
		})
	}
}

func TestNewItemWithContent(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "testcontent.txt")
	content := "Test file content\nLine 2"

	opts := NewItemOptions{
		Path: filePath,
		ItemType: "file",
		Value: content,
	}

	result, err := newItem(opts)
	if err != nil {
		t.Fatalf("newItem() error = %v", err)
	}

	if result == nil {
		t.Fatal("newItem() expected result but got nil")
	}

	// Verify file exists and has correct content
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	if string(data) != content {
		t.Errorf("file content = %q, want %q", string(data), content)
	}
}

func TestNewItemDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	dirPath := filepath.Join(tmpDir, "newdir", "subdir")

	opts := NewItemOptions{
		Path: dirPath,
		ItemType: "directory",
	}

	result, err := newItem(opts)
	if err != nil {
		t.Fatalf("newItem() error = %v", err)
	}

	if result == nil {
		t.Fatal("newItem() expected result but got nil")
	}

	// Verify directory exists
	info, err := os.Stat(dirPath)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}

	if !info.IsDir() {
		t.Errorf("created item is not a directory")
	}
}
