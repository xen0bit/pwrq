package filesystem

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseJoinPathArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []any
		wantPaths   []string
		wantResolve bool
		wantErr     bool
	}{
		{
			name:    "no args",
			args:    []any{},
			wantErr: true,
		},
		{
			name:      "single path",
			args:      []any{"/tmp"},
			wantPaths: []string{"/tmp"},
		},
		{
			name:      "multiple paths",
			args:      []any{"/tmp", "test", "file.go"},
			wantPaths: []string{"/tmp", "test", "file.go"},
		},
		{
			name:        "with resolve flag",
			args:        []any{"/tmp", "test", true},
			wantPaths:   []string{"/tmp", "test"},
			wantResolve: true,
		},
		{
			name:    "null argument",
			args:    []any{nil},
			wantErr: true,
		},
		{
			name:    "empty string",
			args:    []any{""},
			wantErr: true,
		},
		{
			name:    "non-string argument",
			args:    []any{123},
			wantErr: true,
		},
		{
			name:      "cmdlet output path binds by property name",
			args:      []any{map[string]any{"PwrqValue": "/tmp", "PwrqType": "string"}},
			wantPaths: []string{"/tmp"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths, resolve, err := parseJoinPathArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseJoinPathArgs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if len(paths) != len(tt.wantPaths) {
				t.Errorf("got %d paths, want %d", len(paths), len(tt.wantPaths))
			}
			for i, p := range paths {
				if i < len(tt.wantPaths) && p != tt.wantPaths[i] {
					t.Errorf("path[%d] = %v, want %v", i, p, tt.wantPaths[i])
				}
			}
			if resolve != tt.wantResolve {
				t.Errorf("resolve = %v, want %v", resolve, tt.wantResolve)
			}
		})
	}
}

func TestJoinPath(t *testing.T) {
	tests := []struct {
		name         string
		paths        []string
		resolve      bool
		wantContains string
		wantAbs      bool
		wantErr      bool
	}{
		{
			name:         "simple join",
			paths:        []string{"/tmp", "test"},
			wantContains: "tmp",
		},
		{
			name:         "multiple segments",
			paths:        []string{"/tmp", "test", "file.go"},
			wantContains: "file.go",
		},
		{
			name:         "absolute path resets",
			paths:        []string{"/tmp", "/usr", "local"},
			wantContains: "usr",
		},
		{
			name:         "relative with dot",
			paths:        []string{"/tmp", ".", "test"},
			wantContains: "tmp",
		},
		{
			name:         "relative with dotdot",
			paths:        []string{"/tmp/test", "..", "other"},
			wantContains: "other",
		},
		{
			name:         "UNC path",
			paths:        []string{"\\\\server\\share", "folder"},
			wantContains: "server",
		},
		{
			name:    "resolve to absolute",
			paths:   []string{"tmp", "test"},
			resolve: true,
			wantAbs: true,
		},
		{
			name:    "no paths",
			paths:   []string{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := joinPath(tt.paths, tt.resolve)
			if (err != nil) != tt.wantErr {
				t.Errorf("joinPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if tt.wantContains != "" && !contains(result, tt.wantContains) {
				t.Errorf("result = %v, want to contain %v", result, tt.wantContains)
			}
			if tt.wantAbs && !isAbs(result) {
				t.Errorf("result = %v, want absolute path", result)
			}
		})
	}
}

func TestJoinPathErrors(t *testing.T) {
	tests := []struct {
		name          string
		pipelineInput any
		args          []any
		wantErr       bool
	}{
		{
			name:          "null in args",
			pipelineInput: nil,
			args:          []any{nil},
			wantErr:       true,
		},
		{
			name:          "non-string arg",
			pipelineInput: nil,
			args:          []any{123},
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Combine pipeline input with args like the registered function does
			allArgs := tt.args
			if tt.pipelineInput != nil {
				allArgs = append([]any{tt.pipelineInput}, tt.args...)
			}

			_, _, err := parseJoinPathArgs(allArgs)
			if (err != nil) != tt.wantErr {
				t.Errorf("expected error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSplitPathComponents(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantDir  string
		wantBase string
		wantExt  string
		wantName string
	}{
		{
			name:     "Unix path",
			path:     "/tmp/test/file.go",
			wantDir:  "/tmp/test",
			wantBase: "file.go",
			wantExt:  ".go",
			wantName: "file",
		},
		{
			name:     "simple path",
			path:     "file.txt",
			wantDir:  ".",
			wantBase: "file.txt",
			wantExt:  ".txt",
			wantName: "file",
		},
		{
			name:     "no extension",
			path:     "/tmp/README",
			wantDir:  "/tmp",
			wantBase: "README",
			wantExt:  "",
			wantName: "README",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Dir(tt.path)
			base := filepath.Base(tt.path)
			ext := filepath.Ext(tt.path)
			name := strings.TrimSuffix(base, ext)

			if dir != tt.wantDir {
				t.Errorf("Dir = %v, want %v", dir, tt.wantDir)
			}
			if base != tt.wantBase {
				t.Errorf("Base = %v, want %v", base, tt.wantBase)
			}
			if ext != tt.wantExt {
				t.Errorf("Ext = %v, want %v", ext, tt.wantExt)
			}
			if name != tt.wantName {
				t.Errorf("Name = %v, want %v", name, tt.wantName)
			}
		})
	}
}

// Helper functions

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func isAbs(path string) bool {
	return len(path) > 0 && path[0] == '/'
}
