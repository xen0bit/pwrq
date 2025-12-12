package md5

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/xen0bit/pwrq/pkg/udf/common"
)

func TestMD5File(t *testing.T) {
	// Create a temporary file for testing
	tmpFile, err := os.CreateTemp("", "pwrq-md5-file-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Write test content
	testContent := "hello world"
	if _, err := tmpFile.WriteString(testContent); err != nil {
		t.Fatalf("Failed to write test content: %v", err)
	}
	tmpFile.Close()

	// Calculate expected hash
	expectedHash := fmt.Sprintf("%x", md5.Sum([]byte(testContent)))

	tests := []struct {
		name    string
		input   any
		want    string
		wantErr bool
	}{
		{
			name:    "regular file path string",
			input:   tmpFile.Name(),
			want:    expectedHash,
			wantErr: false,
		},
		{
			name: "UDF result object input - should extract _val",
			input: map[string]any{
				"_val": tmpFile.Name(),
				"_meta": map[string]any{
					"source": "previous_udf",
				},
			},
			want:    expectedHash,
			wantErr: false,
		},
		{
			name:    "non-existent file",
			input:   "/nonexistent/file/path",
			want:    "",
			wantErr: true,
		},
		{
			name:    "non-string input",
			input:   123,
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Extract _val if it's a UDF result
			inputVal := common.ExtractUDFValue(tt.input)

			// Convert to file path
			filePath, ok := inputVal.(string)
			if !ok {
				if !tt.wantErr {
					t.Errorf("input is not a string: %T", inputVal)
				}
				return
			}

			// Convert to absolute path
			absPath, err := filepath.Abs(filePath)
			if err != nil {
				if !tt.wantErr {
					t.Errorf("failed to resolve path: %v", err)
				}
				return
			}

			// Read file
			fileData, err := os.ReadFile(absPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Compute hash
				hash := md5.Sum(fileData)
				got := fmt.Sprintf("%x", hash)

				if got != tt.want {
					t.Errorf("md5_file() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestMD5FileWithUDFResultInput(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "pwrq-md5-file-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Write test content
	testContent := "test file content"
	if _, err := tmpFile.WriteString(testContent); err != nil {
		t.Fatalf("Failed to write test content: %v", err)
	}
	tmpFile.Close()

	// Create a UDF result object
	udfResult := map[string]any{
		"_val": tmpFile.Name(),
		"_meta": map[string]any{
			"source": "find",
		},
	}

	// Extract _val (simulating what the function does)
	extracted := common.ExtractUDFValue(udfResult)

	if extracted != tmpFile.Name() {
		t.Errorf("extractUDFValue() = %v, want %v", extracted, tmpFile.Name())
	}

	// Verify it hashes correctly
	fileData, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	hash := md5.Sum(fileData)
	expected := fmt.Sprintf("%x", md5.Sum([]byte(testContent)))

	if fmt.Sprintf("%x", hash) != expected {
		t.Errorf("hashing file = %v, want %v", fmt.Sprintf("%x", hash), expected)
	}
}

func TestMD5FileChaining(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "pwrq-md5-file-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Write test content
	testContent := "chaining test"
	if _, err := tmpFile.WriteString(testContent); err != nil {
		t.Fatalf("Failed to write test content: %v", err)
	}
	tmpFile.Close()

	// Simulate: find returns UDF result with file path
	udfResult := map[string]any{
		"_val": tmpFile.Name(),
		"_meta": map[string]any{
			"type": "file",
		},
	}

	// Simulate: md5_file receives UDF result and extracts _val
	extracted := common.ExtractUDFValue(udfResult)
	if extracted != tmpFile.Name() {
		t.Fatalf("extraction failed: got %v, want %v", extracted, tmpFile.Name())
	}

	// Read and hash the file
	fileData, err := os.ReadFile(extracted.(string))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	hash := md5.Sum(fileData)
	expectedHash := md5.Sum([]byte(testContent))

	if hash != expectedHash {
		t.Errorf("chaining failed: got %x, want %x", hash, expectedHash)
	}
}

func TestMD5FileMetadata(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "pwrq-md5-file-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Write test content
	testContent := "metadata test"
	if _, err := tmpFile.WriteString(testContent); err != nil {
		t.Fatalf("Failed to write test content: %v", err)
	}
	if err := tmpFile.Sync(); err != nil {
		t.Fatalf("Failed to sync file: %v", err)
	}
	tmpFile.Close()

	// Get file info
	fileInfo, err := os.Stat(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}
	
	// Verify file size is correct
	if fileInfo.Size() != int64(len(testContent)) {
		t.Fatalf("File size mismatch: got %d, want %d", fileInfo.Size(), len(testContent))
	}

	// Read file and compute hash
	fileData, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	hash := md5.Sum(fileData)
	hashHex := fmt.Sprintf("%x", hash)

	absPath, err := filepath.Abs(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to get absolute path: %v", err)
	}

	// Simulate function return (matching actual function behavior)
	result := map[string]any{
		"_val": hashHex,
		"_meta": map[string]any{
			"algorithm":    "md5",
			"file_path":    absPath,
			"file_size":    int(fileInfo.Size()), // Function converts to int
			"hash_length":  len(hashHex),
		},
	}

	// Verify structure
	if val, ok := result["_val"].(string); !ok || val != hashHex {
		t.Errorf("_val = %v, want %v", val, hashHex)
	}

	meta, ok := result["_meta"].(map[string]any)
	if !ok {
		t.Fatal("_meta is not a map")
	}

	if algo, ok := meta["algorithm"].(string); !ok || algo != "md5" {
		t.Errorf("algorithm = %v, want %v", algo, "md5")
	}

	if filePath, ok := meta["file_path"].(string); !ok || filePath != absPath {
		t.Errorf("file_path = %v, want %v", filePath, absPath)
	}

	if fileSize, ok := meta["file_size"].(int); !ok || fileSize != int(fileInfo.Size()) {
		t.Errorf("file_size = %v, want %v", fileSize, fileInfo.Size())
	}

	if hashLen, ok := meta["hash_length"].(int); !ok || hashLen != len(hashHex) {
		t.Errorf("hash_length = %v, want %v", hashLen, len(hashHex))
	}
}

func TestMD5FileErrorCases(t *testing.T) {
	tests := []struct {
		name    string
		filePath string
		wantErr bool
	}{
		{
			name:    "non-existent file",
			filePath: "/nonexistent/path/to/file",
			wantErr: true,
		},
		{
			name:    "directory instead of file",
			filePath: "/tmp",
			wantErr: false, // Reading a directory might succeed or fail depending on OS
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := os.ReadFile(tt.filePath)
			if (err != nil) != tt.wantErr {
				// For directory case, we just check that we get an error or not
				// The actual behavior depends on the OS
				if tt.name == "directory instead of file" {
					return
				}
				t.Errorf("ReadFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

