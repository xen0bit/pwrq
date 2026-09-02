package astsearch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// What a structural search costs, measured where it is spent: converting byte
// offsets into places a person can open, and walking a tree of real size.

// benchSource is a file big enough that a per-match scan of it shows up, which
// is the shape a real search has - one match every few lines of a long file.
func benchSource(functions int) string {
	var b strings.Builder
	b.WriteString("package main\n\nimport (\n\t\"crypto/md5\"\n\t\"fmt\"\n)\n\n")
	for i := 0; i < functions; i++ {
		fmt.Fprintf(&b, `
func run%d(name string) error {
	sum := md5.Sum([]byte(name))
	if name == "" {
		return fmt.Errorf("empty %%x", sum)
	}
	h := md5.New()
	_ = h
	return nil
}
`, i)
	}
	return b.String()
}

func BenchmarkPosition(b *testing.B) {
	source := []byte(benchSource(200))
	// Offsets spread through the file, so the cost is the average one rather
	// than the first line's.
	offsets := make([]int, 512)
	for i := range offsets {
		offsets[i] = i * len(source) / len(offsets)
	}
	lines := Index(source)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lines.At(offsets[i%len(offsets)])
	}
}

// Indexing is what At costs amortised, so it is measured beside it: a file is
// indexed once and asked several thousand times.
func BenchmarkIndex(b *testing.B) {
	source := []byte(benchSource(200))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Index(source)
	}
}

// benchTree writes a tree of Go files and returns its root.
func benchTree(b *testing.B, files, functions int) string {
	b.Helper()
	dir := b.TempDir()
	source := benchSource(functions)
	for i := 0; i < files; i++ {
		name := filepath.Join(dir, fmt.Sprintf("pkg%d", i%8), fmt.Sprintf("file%d.go", i))
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(source), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	return dir
}

var benchPatterns = []string{"md5.New()", "md5.Sum($$$A)", "fmt.Errorf($$$A)", "if $C { $$$B }"}

func BenchmarkSearchTree(b *testing.B) {
	dir := benchTree(b, 24, 40)
	// The first search decodes the grammars, which is not what this measures.
	if _, err := SearchTree(context.Background(), dir, benchPatterns, "*.go", nil); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		found, err := SearchTree(context.Background(), dir, benchPatterns, "*.go", nil)
		if err != nil {
			b.Fatal(err)
		}
		if len(found) == 0 {
			b.Fatal("no matches, so this measures nothing")
		}
	}
}

// One file with many matches in it, which is where a per-match scan of the
// source shows up as the quadratic it is.
func BenchmarkSearchFile(b *testing.B) {
	dir := benchTree(b, 1, 400)
	if _, err := SearchTree(context.Background(), dir, benchPatterns, "*.go", nil); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := SearchTree(context.Background(), dir, benchPatterns, "*.go", nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMatchesText(b *testing.B) {
	if _, err := MatchesText("getenv($X)", "python", "os.getenv(\"HOME\")"); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := MatchesText("getenv($X)", "python", "os.getenv(\"HOME\")"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompilePattern(b *testing.B) {
	if _, err := compilePattern("md5.Sum($$$A)", "go"); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := compilePattern("md5.Sum($$$A)", "go"); err != nil {
			b.Fatal(err)
		}
	}
}
