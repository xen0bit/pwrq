package filesystem

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseMoveItemArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []any
		wantPath    string
		wantDest    string
		wantForce   bool
	}{
		{
			name:        "no args",
			args:        []any{},
			wantPath:    ".",
			wantDest:    "",
			wantForce:   false,
		},
		{
			name:        "path and destination",
			args:        []any{"/tmp/src", "/tmp/dst"},
			wantPath:    "/tmp/src",
			wantDest:    "/tmp/dst",
			wantForce:   false,
		},
		{
			name: "named parameters",
			args: []any{map[string]any{
				"Path": "/tmp/src",
				"Destination": "/tmp/dst",
				"Force": true,
			}},
			wantPath:    "/tmp/src",
			wantDest:    "/tmp/dst",
			wantForce:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := parseMoveItemArgs(tt.args)
			if err != nil {
				t.Fatalf("parseMoveItemArgs() error = %v", err)
			}
			if opts.Path != tt.wantPath {
				t.Errorf("Path = %v, want %v", opts.Path, tt.wantPath)
			}
			if opts.Destination != tt.wantDest {
				t.Errorf("Destination = %v, want %v", opts.Destination, tt.wantDest)
			}
			if opts.Force != tt.wantForce {
				t.Errorf("Force = %v, want %v", opts.Force, tt.wantForce)
			}
		})
	}
}

func TestMoveItem(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "source.txt")
	dstFile := filepath.Join(tmpDir, "dest.txt")

	// Create source file
	content := "Test content for move"
	if err := os.WriteFile(srcFile, []byte(content), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	tests := []struct {
		name        string
		opts        MoveItemOptions
		wantSuccess bool
		checkSrc    bool // should source still exist?
		checkDest   bool // should destination exist?
	}{
		{
			name: "move file",
			opts: MoveItemOptions{
				Path: srcFile,
				Destination: dstFile,
			},
			wantSuccess: true,
			checkSrc: false,
			checkDest: true,
		},
		{
			name: "move with force (overwrite)",
			opts: MoveItemOptions{
				Path: dstFile,
				Destination: srcFile,
				Force: true,
			},
			wantSuccess: true,
			checkSrc: false,
			checkDest: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := moveItem(tt.opts)
			if tt.wantSuccess && err != nil {
				t.Fatalf("moveItem() error = %v", err)
			}
			if !tt.wantSuccess && err == nil {
				t.Fatal("moveItem() expected error but got none")
			}
			if tt.wantSuccess && result == nil {
				t.Fatal("moveItem() expected result but got nil")
			}
			if tt.checkSrc {
				if _, err := os.Stat(tt.opts.Path); os.IsNotExist(err) {
					t.Errorf("moveItem() source should still exist: %s", tt.opts.Path)
				}
			} else if tt.wantSuccess {
				if _, err := os.Stat(tt.opts.Path); err == nil {
					t.Errorf("moveItem() source should not exist after move: %s", tt.opts.Path)
				}
			}
			if tt.checkDest {
				if _, err := os.Stat(tt.opts.Destination); os.IsNotExist(err) {
					t.Errorf("moveItem() destination does not exist: %s", tt.opts.Destination)
				}
			}
		})
	}
}

func TestMoveItemDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "srcdir")
	dstDir := filepath.Join(tmpDir, "dstdir")

	// Create source directory with files
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	opts := MoveItemOptions{
		Path: srcDir,
		Destination: dstDir,
	}

	result, err := moveItem(opts)
	if err != nil {
		t.Fatalf("moveItem() error = %v", err)
	}

	if result == nil {
		t.Fatal("moveItem() expected result but got nil")
	}

	// Verify source no longer exists
	if _, err := os.Stat(srcDir); err == nil {
		t.Errorf("source directory should not exist after move")
	}

	// Verify destination exists
	info, err := os.Stat(dstDir)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}

	if !info.IsDir() {
		t.Errorf("destination is not a directory")
	}

	// Verify file was moved
	if _, err := os.Stat(filepath.Join(dstDir, "file1.txt")); os.IsNotExist(err) {
		t.Errorf("file1.txt was not moved")
	}
}

func TestMoveItemIntoDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "source.txt")
	dstDir := filepath.Join(tmpDir, "destdir")

	// Create source file
	content := "Test content"
	if err := os.WriteFile(srcFile, []byte(content), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	// Create destination directory
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	// Move file into directory
	opts := MoveItemOptions{
		Path: srcFile,
		Destination: dstDir,
	}

	result, err := moveItem(opts)
	if err != nil {
		t.Fatalf("moveItem() error = %v", err)
	}

	if result == nil {
		t.Fatal("moveItem() expected result but got nil")
	}

	// Verify file was moved into directory with correct name
	expectedDst := filepath.Join(dstDir, "source.txt")
	if _, err := os.Stat(expectedDst); os.IsNotExist(err) {
		t.Errorf("file was not moved into directory: %s", expectedDst)
	}

	// Verify source no longer exists
	if _, err := os.Stat(srcFile); err == nil {
		t.Errorf("source should not exist after move")
	}

	// Verify content
	data, err := os.ReadFile(expectedDst)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	if string(data) != content {
		t.Errorf("moved content = %q, want %q", string(data), content)
	}
}
