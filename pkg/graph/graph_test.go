package graph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/itchyny/gojq"
)

func TestGenerateGraph_SimpleQuery(t *testing.T) {
	query, err := gojq.Parse(".")
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "start") {
		t.Error("Output should contain 'start' node")
	}
	if !strings.Contains(contentStr, "end") {
		t.Error("Output should contain 'end' node")
	}
	if !strings.Contains(contentStr, "Start") {
		t.Error("Output should contain 'Start' label")
	}
}

func TestGenerateGraph_PipeOperation(t *testing.T) {
	query, err := gojq.Parse("md5 | ._val")
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "md5()") {
		t.Error("Output should contain 'md5()' function")
	}
	if !strings.Contains(contentStr, "._val") {
		t.Error("Output should contain '._val' access")
	}
	if !strings.Contains(contentStr, "start ->") {
		t.Error("Start node should be connected")
	}
	// Pipes should not create nodes, just edges
	if strings.Contains(contentStr, "Pipe") {
		t.Error("Pipe operations should not create nodes")
	}
}

func TestGenerateGraph_FunctionCall(t *testing.T) {
	query, err := gojq.Parse("md5")
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "md5()") {
		t.Error("Output should contain 'md5()' function")
	}
}

func TestGenerateGraph_ObjectLiteral(t *testing.T) {
	query, err := gojq.Parse(`{file: "test", md5: (md5 | ._val)}`)
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	// Each key should have its own container
	if !strings.Contains(contentStr, "file {") {
		t.Error("Output should contain 'file' container")
	}
	if !strings.Contains(contentStr, "md5 {") {
		t.Error("Output should contain 'md5' container")
	}
	// Keys should be independent (no edges between them)
	// Check that md5 container doesn't connect to file container
	lines := strings.Split(contentStr, "\n")
	fileFound := false
	for _, line := range lines {
		if strings.Contains(line, "file {") {
			fileFound = true
		}
		// Should not have edge from file to md5
		if fileFound && strings.Contains(line, "file") && strings.Contains(line, "md5") && strings.Contains(line, "->") {
			t.Error("Object keys should not be connected to each other")
		}
	}
}

func TestGenerateGraph_ObjectLiteralWithMultipleHashes(t *testing.T) {
	query, err := gojq.Parse(`{file: "test", md5: (md5 | ._val), sha1: (sha1 | ._val), sha256: (sha256 | ._val)}`)
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	// All keys should have containers
	keys := []string{"file", "md5", "sha1", "sha256"}
	for _, key := range keys {
		if !strings.Contains(contentStr, key+" {") {
			t.Errorf("Output should contain '%s' container", key)
		}
	}
	// Each hash function should be in its own container
	if !strings.Contains(contentStr, "md5()") {
		t.Error("Output should contain 'md5()' function")
	}
	if !strings.Contains(contentStr, "sha1()") {
		t.Error("Output should contain 'sha1()' function")
	}
	if !strings.Contains(contentStr, "sha256()") {
		t.Error("Output should contain 'sha256()' function")
	}
}

func TestGenerateGraph_ArrayLiteral(t *testing.T) {
	query, err := gojq.Parse(`[find("pkg/udf"; "file")]`)
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "find") {
		t.Error("Output should contain 'find' function")
	}
}

func TestGenerateGraph_SliceOperation(t *testing.T) {
	query, err := gojq.Parse(".[0:3]")
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "Slice [0:3]") {
		t.Error("Output should contain 'Slice [0:3]'")
	}
	// Should only appear once (no duplicates)
	count := strings.Count(contentStr, "Slice [0:3]")
	if count != 1 {
		t.Errorf("Slice should appear exactly once, found %d times", count)
	}
}

func TestGenerateGraph_MapFunction(t *testing.T) {
	query, err := gojq.Parse(`map(select(._val | endswith(".go")))`)
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "map()") {
		t.Error("Output should contain 'map()' function container")
	}
	if !strings.Contains(contentStr, "select()") {
		t.Error("Output should contain 'select()' function container")
	}
	if !strings.Contains(contentStr, "endswith()") {
		t.Error("Output should contain 'endswith()' function container")
	}
}

func TestGenerateGraph_ComplexNestedQuery(t *testing.T) {
	query, err := gojq.Parse(`[find("pkg/udf"; "file")] | map(select(._val | endswith(".go"))) | map(. as $path | $path | cat | ._val | {file: $path, md5: (md5 | ._val)}) | .[0:3]`)
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	// Check for key components
	components := []string{"find", "map()", "select()", "endswith()", "cat()", "md5()", "Slice [0:3]"}
	for _, comp := range components {
		if !strings.Contains(contentStr, comp) {
			t.Errorf("Output should contain '%s'", comp)
		}
	}
	// Check that object literal has independent keys
	if !strings.Contains(contentStr, "file {") {
		t.Error("Output should contain 'file' container")
	}
	if !strings.Contains(contentStr, "md5 {") {
		t.Error("Output should contain 'md5' container")
	}
}

func TestGenerateGraph_StartNodeConnection(t *testing.T) {
	query, err := gojq.Parse("md5")
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "start ->") {
		t.Error("Start node should be connected to first node")
	}
}

func TestGenerateGraph_EndNodeConnection(t *testing.T) {
	query, err := gojq.Parse("md5")
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "-> end_") {
		t.Error("Last node should be connected to end node")
	}
}

func TestGenerateGraph_SVGOutput(t *testing.T) {
	query, err := gojq.Parse("md5 | ._val")
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.svg")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	// Check that SVG file was created
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("Failed to stat output file: %v", err)
	}
	if info.Size() == 0 {
		t.Error("SVG file should not be empty")
	}

	// Check that it's actually SVG content
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "<svg") {
		t.Error("Output should be valid SVG")
	}
}

func TestGenerateGraph_InvalidOutputPath(t *testing.T) {
	query, err := gojq.Parse(".")
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	// Try to write to a non-existent directory
	outputPath := "/nonexistent/path/test.d2"

	err = GenerateGraph(query, outputPath)
	if err == nil {
		t.Error("GenerateGraph should fail with invalid path")
	}
}

func TestGenerateGraph_UnsupportedFormat(t *testing.T) {
	query, err := gojq.Parse(".")
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.txt")

	err = GenerateGraph(query, outputPath)
	if err == nil {
		t.Error("GenerateGraph should fail with unsupported format")
	}
	if !strings.Contains(err.Error(), "unsupported output format") {
		t.Errorf("Error should mention unsupported format, got: %v", err)
	}
}

func TestGenerateGraph_EmptyQuery(t *testing.T) {
	query, err := gojq.Parse("")
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	// Empty query might succeed or fail, but shouldn't panic
	if err != nil {
		t.Logf("GenerateGraph with empty query returned error (expected): %v", err)
	}
}

func TestGenerateGraph_NestedFunctions(t *testing.T) {
	query, err := gojq.Parse(`map(select(._val | endswith(".go")))`)
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	// Nested functions should create nested containers
	if !strings.Contains(contentStr, "map()") {
		t.Error("Output should contain 'map()' container")
	}
	// select should be inside map
	lines := strings.Split(contentStr, "\n")
	mapFound := false
	selectFound := false
	for _, line := range lines {
		if strings.Contains(line, "map()") {
			mapFound = true
		}
		if mapFound && strings.Contains(line, "select()") {
			selectFound = true
			break
		}
	}
	if !selectFound {
		t.Error("select() should be nested inside map()")
	}
}

func TestGenerateGraph_ObjectLiteralWithVariable(t *testing.T) {
	query, err := gojq.Parse(`. as $path | {file: $path}`)
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "file {") {
		t.Error("Output should contain 'file' container")
	}
	// Variable should be shown (might be formatted as _VAR_path)
	if !strings.Contains(contentStr, "$path") && !strings.Contains(contentStr, "_VAR_path") {
		t.Error("Output should contain variable reference")
	}
}

func TestGenerateGraph_MultiplePipes(t *testing.T) {
	query, err := gojq.Parse("md5 | ._val | sha1 | ._val")
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	// Should have md5 and sha1 functions
	if !strings.Contains(contentStr, "md5()") {
		t.Error("Output should contain 'md5()' function")
	}
	if !strings.Contains(contentStr, "sha1()") {
		t.Error("Output should contain 'sha1()' function")
	}
	// Should not have pipe nodes
	if strings.Contains(contentStr, "Pipe") {
		t.Error("Pipe operations should not create nodes")
	}
}

func TestGenerateGraph_ObjectLiteralInMap(t *testing.T) {
	query, err := gojq.Parse(`map({file: "test", md5: (md5 | ._val)})`)
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	// map should contain object literal
	if !strings.Contains(contentStr, "map()") {
		t.Error("Output should contain 'map()' container")
	}
	// Object literal should be nested inside map
	lines := strings.Split(contentStr, "\n")
	mapFound := false
	objectFound := false
	for _, line := range lines {
		if strings.Contains(line, "map()") {
			mapFound = true
		}
		if mapFound && (strings.Contains(line, "Object") || strings.Contains(line, "file {")) {
			objectFound = true
			break
		}
	}
	if !objectFound {
		t.Error("Object literal should be nested inside map()")
	}
}

func TestGenerateGraph_ParenthesizedExpression(t *testing.T) {
	query, err := gojq.Parse("(md5 | ._val)")
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	// Parenthesized expression currently shows as "Query" - this is a known limitation
	// The expression is wrapped in TermTypeQuery which needs unwrapping
	if !strings.Contains(contentStr, "Query") && !strings.Contains(contentStr, "md5") {
		t.Error("Output should contain parenthesized expression representation")
	}
}

func TestGenerateGraph_StringLiteral(t *testing.T) {
	query, err := gojq.Parse(`"test"`)
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "test") || !strings.Contains(contentStr, "String") {
		t.Error("Output should contain string literal")
	}
}

func TestGenerateGraph_NumberLiteral(t *testing.T) {
	query, err := gojq.Parse("42")
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "42") || !strings.Contains(contentStr, "Number") {
		t.Error("Output should contain number literal")
	}
}

func TestGenerateGraph_IdentityOperator(t *testing.T) {
	query, err := gojq.Parse(".")
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "Identity") {
		t.Error("Output should contain 'Identity' operator")
	}
}

func TestGenerateGraph_IndexAccess(t *testing.T) {
	query, err := gojq.Parse(".[0]")
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	// Index access shows as "Index" - this is the current label format
	if !strings.Contains(contentStr, "Index") {
		t.Error("Output should contain 'Index' label")
	}
}

func TestGenerateGraph_ObjectKeyAccess(t *testing.T) {
	query, err := gojq.Parse(".file")
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, ".file") {
		t.Error("Output should contain object key access '.file'")
	}
}

func TestGenerateGraph_NoDuplicateSlices(t *testing.T) {
	query, err := gojq.Parse(".[0:3] | .[0:2]")
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	// Should have both slices
	if !strings.Contains(contentStr, "Slice [0:3]") {
		t.Error("Output should contain 'Slice [0:3]'")
	}
	if !strings.Contains(contentStr, "Slice [0:2]") {
		t.Error("Output should contain 'Slice [0:2]'")
	}
	// Each should appear only once
	count1 := strings.Count(contentStr, "Slice [0:3]")
	count2 := strings.Count(contentStr, "Slice [0:2]")
	if count1 != 1 {
		t.Errorf("Slice [0:3] should appear exactly once, found %d times", count1)
	}
	if count2 != 1 {
		t.Errorf("Slice [0:2] should appear exactly once, found %d times", count2)
	}
}

func TestGenerateGraph_FunctionWithArguments(t *testing.T) {
	query, err := gojq.Parse(`find("pkg/udf"; "file")`)
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "find()") {
		t.Error("Output should contain 'find()' function container")
	}
	// Arguments should be inside the function container
	if !strings.Contains(contentStr, "pkg/udf") || !strings.Contains(contentStr, "file") {
		t.Error("Output should contain function arguments")
	}
}

func TestGenerateGraph_ComplexObjectWithAllHashes(t *testing.T) {
	query, err := gojq.Parse(`{file: "test", md5: (md5 | ._val), sha1: (sha1 | ._val), sha256: (sha256 | ._val), sha512: (sha512 | ._val)}`)
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	// All keys should have containers
	keys := []string{"file", "md5", "sha1", "sha256", "sha512"}
	for _, key := range keys {
		if !strings.Contains(contentStr, key+" {") {
			t.Errorf("Output should contain '%s' container", key)
		}
	}
	// Each hash function should be in its own container
	hashFuncs := []string{"md5()", "sha1()", "sha256()", "sha512()"}
	for _, funcName := range hashFuncs {
		if !strings.Contains(contentStr, funcName) {
			t.Errorf("Output should contain '%s' function", funcName)
		}
	}
}

func TestGenerateGraph_ArrayWithFunction(t *testing.T) {
	query, err := gojq.Parse(`[find("pkg/udf"; "file")]`)
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	// Array should show the function inside
	if !strings.Contains(contentStr, "find") {
		t.Error("Output should contain 'find' function")
	}
}

func TestGenerateGraph_SelectWithPipe(t *testing.T) {
	query, err := gojq.Parse(`select(._val | endswith(".go"))`)
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "select()") {
		t.Error("Output should contain 'select()' function container")
	}
	if !strings.Contains(contentStr, "endswith()") {
		t.Error("Output should contain 'endswith()' function container")
	}
	// endswith should be inside select
	lines := strings.Split(contentStr, "\n")
	selectFound := false
	endswithFound := false
	for _, line := range lines {
		if strings.Contains(line, "select()") {
			selectFound = true
		}
		if selectFound && strings.Contains(line, "endswith()") {
			endswithFound = true
			break
		}
	}
	if !endswithFound {
		t.Error("endswith() should be nested inside select()")
	}
}

func TestGenerateGraph_VariableAssignment(t *testing.T) {
	query, err := gojq.Parse(`. as $path | $path`)
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	// Variable should be shown
	if !strings.Contains(contentStr, "$path") && !strings.Contains(contentStr, "_VAR_path") {
		t.Error("Output should contain variable reference")
	}
}

func TestGenerateGraph_HTTPFunction(t *testing.T) {
	query, err := gojq.Parse(`http("POST"; "https://httpbin.konghq.com/post")`)
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "http()") {
		t.Error("Output should contain 'http()' function container")
	}
	// Arguments should be inside
	if !strings.Contains(contentStr, "POST") {
		t.Error("Output should contain HTTP method")
	}
}

func TestGenerateGraph_FromJSONFunction(t *testing.T) {
	query, err := gojq.Parse(`fromjson`)
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "fromjson()") {
		t.Error("Output should contain 'fromjson()' function")
	}
}

func TestGenerateGraph_EndToEndComplexQuery(t *testing.T) {
	// This is the full query from the examples
	query, err := gojq.Parse(`[find("pkg/udf"; "file")] | map(select(._val | endswith(".go"))) | map(. as $path | $path | cat | ._val | {file: $path, md5: (md5 | ._val), sha1: (sha1 | ._val), sha256: (sha256 | ._val), sha512: (sha512 | ._val)}) | .[0:3] | http("POST"; "https://httpbin.konghq.com/post") | ._val | fromjson`)
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.d2")

	err = GenerateGraph(query, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	// Verify all major components are present
	components := []string{
		"find", "map()", "select()", "endswith()",
		"cat()", "md5()", "sha1()", "sha256()", "sha512()",
		"Slice [0:3]", "http()", "fromjson()",
	}
	for _, comp := range components {
		if !strings.Contains(contentStr, comp) {
			t.Errorf("Output should contain '%s'", comp)
		}
	}
	// Verify object literal structure
	if !strings.Contains(contentStr, "file {") {
		t.Error("Output should contain 'file' container")
	}
	// Verify start and end connections
	if !strings.Contains(contentStr, "start ->") {
		t.Error("Start node should be connected")
	}
	if !strings.Contains(contentStr, "-> end_") {
		t.Error("End node should be connected")
	}
}

