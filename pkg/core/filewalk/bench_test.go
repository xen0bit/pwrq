package filewalk

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Every search starts with a walk, so what the walk costs is a floor under
// select_string, select_ast and every rule.

func benchTree(b *testing.B, dirs, perDir int) string {
	b.Helper()
	root := b.TempDir()
	for d := 0; d < dirs; d++ {
		dir := filepath.Join(root, fmt.Sprintf("pkg%d", d), fmt.Sprintf("sub%d", d%4))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			b.Fatal(err)
		}
		for f := 0; f < perDir; f++ {
			name := filepath.Join(dir, fmt.Sprintf("file%d.go", f))
			if err := os.WriteFile(name, []byte("package p\n"), 0o644); err != nil {
				b.Fatal(err)
			}
		}
	}
	return root
}

func benchWalk(b *testing.B, root, include string) {
	b.Helper()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w, err := New(root, include)
		if err != nil {
			b.Fatal(err)
		}
		n := 0
		for {
			_, ok, err := w.Next()
			if err != nil {
				b.Fatal(err)
			}
			if !ok {
				break
			}
			n++
		}
		if n == 0 {
			b.Fatal("walked nothing")
		}
	}
}

func BenchmarkWalk(b *testing.B)         { benchWalk(b, benchTree(b, 60, 20), "") }
func BenchmarkWalkFiltered(b *testing.B) { benchWalk(b, benchTree(b, 60, 20), "*.go") }
