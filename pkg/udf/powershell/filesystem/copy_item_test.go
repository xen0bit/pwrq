package filesystem

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseCopyItemArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []any
		wantPath    string
		wantDest    string
		wantRecurse bool
		wantForce   bool
	}{
		{
			name:        "no args",
			args:        []any{},
			wantPath:    ".",
			wantDest:    "",
			wantRecurse: false,
		},
		{
			name:        "path and destination",
			args:        []any{"/tmp/src", "/tmp/dst"},
			wantPath:    "/tmp/src",
			wantDest:    "/tmp/dst",
			wantRecurse: false,
		},
		{
			name: "named parameters",
			args: []any{map[string]any{
				"Path": "/tmp/src",
				"Destination": "/tmp/dst",
				"Recurse": true,
				"Force": true,
			}},
			wantPath:    "/tmp/src",
			wantDest:    "/tmp/dst",
			wantRecurse: true,
			wantForce:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := parseCopyItemArgs(tt.args)
			if err != nil {
				t.Fatalf("parseCopyItemArgs() error = %v", err)
			}
			if opts.Path != tt.wantPath {
				t.Errorf("Path = %v, want %v", opts.Path, tt.wantPath)
			}
			if opts.Destination != tt.wantDest {
				t.Errorf("Destination = %v, want %v", opts.Destination, tt.wantDest)
			}
			if opts.Recurse != tt.wantRecurse {
				t.Errorf("Recurse = %v, want %v", opts.Recurse, tt.wantRecurse)
			}
			if opts.Force != tt.wantForce {
				t.Errorf("Force = %v, want %v", opts.Force, tt.wantForce)
			}
		})
	}
}

func TestCopyItem(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "source.txt")
	dstFile := filepath.Join(tmpDir, "dest.txt")

	// Create source file
	content := "Test content for copy"
	if err := os.WriteFile(srcFile, []byte(content), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	tests := []struct {
		name        string
		opts        CopyItemOptions
		wantSuccess bool
		checkDest   bool
	}{
		{
			name: "copy file",
			opts: CopyItemOptions{
				Path: srcFile,
				Destination: dstFile,
			},
			wantSuccess: true,
			checkDest: true,
		},
		{
			name: "copy with force (overwrite)",
			opts: CopyItemOptions{
				Path: srcFile,
				Destination: dstFile,
				Force: true,
			},
			wantSuccess: true,
			checkDest: true,
		},
		{
			name: "copy without force (should fail)",
			opts: CopyItemOptions{
				Path: srcFile,
				Destination: dstFile,
				Force: false,
			},
			wantSuccess: false,
			checkDest: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := copyItem(tt.opts)
			if tt.wantSuccess && err != nil {
				t.Fatalf("copyItem() error = %v", err)
			}
			if !tt.wantSuccess && err == nil {
				t.Fatal("copyItem() expected error but got none")
			}
			if tt.wantSuccess && result == nil {
				t.Fatal("copyItem() expected result but got nil")
			}
			if tt.checkDest {
				if _, err := os.Stat(tt.opts.Destination); os.IsNotExist(err) {
					t.Errorf("copyItem() destination does not exist: %s", tt.opts.Destination)
				}
			}
		})
	}
}

func TestCopyItemDirectory(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(srcDir, "file2.txt"), []byte("content2"), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	// Test recursive copy
	opts := CopyItemOptions{
		Path: srcDir,
		Destination: dstDir,
		Recurse: true,
	}

	result, err := copyItem(opts)
	if err != nil {
		t.Fatalf("copyItem() error = %v", err)
	}

	if result == nil {
		t.Fatal("copyItem() expected result but got nil")
	}

	// Verify destination directory exists
	info, err := os.Stat(dstDir)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}

	if !info.IsDir() {
		t.Errorf("destination is not a directory")
	}

	// Verify files were copied
	if _, err := os.Stat(filepath.Join(dstDir, "file1.txt")); os.IsNotExist(err) {
		t.Errorf("file1.txt was not copied")
	}
	if _, err := os.Stat(filepath.Join(dstDir, "file2.txt")); os.IsNotExist(err) {
		t.Errorf("file2.txt was not copied")
	}
}

func TestCopyItemIntoDirectory(t *testing.T) {
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

	// Copy file into directory
	opts := CopyItemOptions{
		Path: srcFile,
		Destination: dstDir,
	}

	result, err := copyItem(opts)
	if err != nil {
		t.Fatalf("copyItem() error = %v", err)
	}

	if result == nil {
		t.Fatal("copyItem() expected result but got nil")
	}

	// Verify file was copied into directory with correct name
	expectedDst := filepath.Join(dstDir, "source.txt")
	if _, err := os.Stat(expectedDst); os.IsNotExist(err) {
		t.Errorf("file was not copied into directory: %s", expectedDst)
	}

	// Verify content
	data, err := os.ReadFile(expectedDst)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	if string(data) != content {
		t.Errorf("copied content = %q, want %q", string(data), content)
	}
}
