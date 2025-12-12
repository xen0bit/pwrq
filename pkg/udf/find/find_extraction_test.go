package find

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xen0bit/pwrq/pkg/udf/common"
)

func TestFindWithUDFResultInput(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "pwrq-find-extraction-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		name    string
		pathArg any
		wantErr bool
	}{
		{
			name:    "regular string path",
			pathArg: tmpDir,
			wantErr: false,
		},
		{
			name: "UDF result object with path in _val",
			pathArg: map[string]any{
				"_val": tmpDir,
				"_meta": map[string]any{
					"source": "previous_udf",
				},
			},
			wantErr: false,
		},
		{
			name: "UDF result with different path",
			pathArg: map[string]any{
				"_val": tmpDir,
				"_meta": map[string]any{
					"type": "path",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Extract _val if it's a UDF result
			extracted := common.ExtractUDFValue(tt.pathArg)
			
			path, ok := extracted.(string)
			if !ok {
				if !tt.wantErr {
					t.Errorf("extracted value is not a string: %T", extracted)
				}
				return
			}

			// Test that find works with the extracted path
			opts := FindOptions{
				Path:     path,
				Type:     "",
				MaxDepth: -1,
				MinDepth: 0,
			}

			results, err := findFiles(opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("findFiles() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(results) == 0 {
					t.Error("findFiles() returned no results")
				}
				
				// Verify results have correct structure
				for _, result := range results {
					obj, ok := result.(map[string]any)
					if !ok {
						t.Errorf("result is not a map: %T", result)
						continue
					}
					if _, ok := obj["_val"]; !ok {
						t.Error("result missing _val key")
					}
					if _, ok := obj["_meta"]; !ok {
						t.Error("result missing _meta key")
					}
				}
			}
		})
	}
}

func TestParseFindArgsWithUDFResult(t *testing.T) {
	tests := []struct {
		name    string
		args    []any
		want    FindOptions
		wantErr bool
	}{
		{
			name: "regular string path",
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
			name: "UDF result as path argument",
			args: []any{
				map[string]any{
					"_val": "/tmp",
					"_meta": map[string]any{
						"source": "previous_udf",
					},
				},
			},
			want: FindOptions{
				Path:     "/tmp",
				Type:     "",
				MaxDepth: -1,
				MinDepth: 0,
			},
			wantErr: false,
		},
		{
			name: "UDF result as path with type filter",
			args: []any{
				map[string]any{
					"_val": "/tmp",
					"_meta": map[string]any{},
				},
				"file",
			},
			want: FindOptions{
				Path:     "/tmp",
				Type:     "file",
				MaxDepth: -1,
				MinDepth: 0,
			},
			wantErr: false,
		},
		{
			name: "UDF result as type argument",
			args: []any{
				"/tmp",
				map[string]any{
					"_val": "file",
					"_meta": map[string]any{},
				},
			},
			want: FindOptions{
				Path:     "/tmp",
				Type:     "file",
				MaxDepth: -1,
				MinDepth: 0,
			},
			wantErr: false,
		},
		{
			name: "UDF result in options object",
			args: []any{
				"/tmp",
				map[string]any{
					"type": map[string]any{
						"_val": "file",
						"_meta": map[string]any{},
					},
				},
			},
			want: FindOptions{
				Path:     "/tmp",
				Type:     "",
				MaxDepth: -1,
				MinDepth: 0,
			},
			wantErr: false,
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
			}
		})
	}
}

func TestCommonExtractUDFValue(t *testing.T) {
	// Test that the common utility works correctly
	tests := []struct {
		name string
		input any
		want any
	}{
		{
			name:  "UDF result",
			input: map[string]any{"_val": "test", "_meta": map[string]any{}},
			want:  "test",
		},
		{
			name:  "regular string",
			input: "test",
			want:  "test",
		},
		{
			name:  "regular map",
			input: map[string]any{"key": "value"},
			want:  map[string]any{"key": "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := common.ExtractUDFValue(tt.input)
			// For maps, use proper comparison
			if gotMap, ok := got.(map[string]any); ok {
				if wantMap, ok := tt.want.(map[string]any); ok {
					if !equalMaps(gotMap, wantMap) {
						t.Errorf("common.ExtractUDFValue() = %v, want %v", got, tt.want)
					}
					return
				}
			}
			// For other types, direct comparison
			if got != tt.want {
				t.Errorf("common.ExtractUDFValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

// equalMaps compares two maps for equality
func equalMaps(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok {
			return false
		} else if v != bv {
			return false
		}
	}
	return true
}

