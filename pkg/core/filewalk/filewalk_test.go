package filewalk

import (
	"os"
	"path/filepath"
	"testing"
)

// tree writes a set of files and returns the directory holding them.
func tree(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// walked collects the paths a walk yields, relative to its root.
func walked(t *testing.T, root, include string) []string {
	t.Helper()
	w, err := New(root, include)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for {
		path, ok, err := w.Next()
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			return out
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, filepath.ToSlash(rel))
	}
}

// TestWalksDepthFirstInNameOrder pins the order, which is filepath.WalkDir's:
// entries are sorted by name and a directory is descended into where its own
// name falls, so "a/inner.txt" comes before "a.txt" because "a" sorts before
// "a.txt".
func TestWalksDepthFirstInNameOrder(t *testing.T) {
	dir := tree(t, "b.txt", "a/inner.txt", "a.txt", "c/d/deep.txt")
	got := walked(t, dir, "")
	want := []string{"a/inner.txt", "a.txt", "b.txt", "c/d/deep.txt"}
	if len(got) != len(want) {
		t.Fatalf("walked %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d was %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

func TestIncludeFiltersByBaseName(t *testing.T) {
	dir := tree(t, "a.go", "b.txt", "sub/c.go", "sub/d.md")
	got := walked(t, dir, "*.go")
	if len(got) != 2 {
		t.Fatalf("walked %v, want the two .go files", got)
	}
}

// TestASingleFileRootIsAlwaysYielded pins the one place the include glob does
// not apply. Naming a file is a stronger statement of intent than a glob, and
// a caller who passes both and gets nothing has been told nothing.
func TestASingleFileRootIsAlwaysYielded(t *testing.T) {
	dir := tree(t, "only.txt")
	got := walked(t, filepath.Join(dir, "only.txt"), "*.go")
	if len(got) != 1 {
		t.Errorf("naming a file directly yielded %v", got)
	}
}

// TestNoiseDirectoriesAreSkipped covers the three that turn a question about a
// project into a question about its dependencies and its history.
func TestNoiseDirectoriesAreSkipped(t *testing.T) {
	dir := tree(t, "kept.txt", ".git/objects/x.txt", "node_modules/p/index.js", "vendor/lib/a.go")
	got := walked(t, dir, "")
	if len(got) != 1 || got[0] != "kept.txt" {
		t.Errorf("walked %v, want only kept.txt", got)
	}
}

func TestAMissingRootIsAnError(t *testing.T) {
	if _, err := New(filepath.Join(t.TempDir(), "absent"), ""); err == nil {
		t.Error("walking a path that does not exist succeeded")
	}
}

// TestTheWalkIsLazy is the property the whole type exists for, and the reason
// it is a stack rather than filepath.WalkDir: nothing below the first file is
// read until the caller asks for more.
//
// It is checked by removing a subdirectory after the first result comes back.
// A walk that had already listed it would carry on yielding files that are no
// longer there; a lazy one tries to read it at that moment and says it is
// gone. The error is the evidence.
func TestTheWalkIsLazy(t *testing.T) {
	dir := tree(t, "a.txt", "later/b.txt", "later/c.txt")
	w, err := New(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	first, ok, err := w.Next()
	if err != nil || !ok {
		t.Fatalf("first file: %v %v", ok, err)
	}
	if filepath.Base(first) != "a.txt" {
		t.Fatalf("first file was %q, want a.txt", first)
	}
	if err := os.RemoveAll(filepath.Join(dir, "later")); err != nil {
		t.Fatal(err)
	}

	path, ok, err := w.Next()
	if err == nil {
		t.Fatalf("the walk yielded %q (ok=%v) from a directory removed before it was asked for, "+
			"so it had been read up front", path, ok)
	}
}
