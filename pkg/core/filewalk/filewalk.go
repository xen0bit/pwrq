// Package filewalk hands out the files under a root, one at a time.
package filewalk

import (
	"io/fs"
	"os"
	"path/filepath"
)

// Skipped are the directory names a search never means to descend into.
//
// They are the three that turn a question about a project into a question
// about its dependencies and its history: a `select_string(".", "TODO")` that
// walks .git reports the TODOs of every version ever committed.
var Skipped = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
}

// Walker yields the files under a root as the caller asks for them.
//
// The walk is an explicit stack of directories rather than filepath.WalkDir,
// because WalkDir pushes: it calls back for every file before returning, which
// is the whole tree's work done up front. A stack can stop between two files
// and resume, which is what a lazy stream needs - `first(select_string(".";
// "needle"))` should stop at the first hit rather than grep the tree and throw
// the rest away.
//
// Entry order matches WalkDir's: os.ReadDir sorts by name, and a directory is
// descended into where its own name falls.
type Walker struct {
	stack []*frame // directories still being read, deepest last
	file  string   // the root, when it is a single file rather than a tree
	// include is a filepath.Match pattern on the base name. Empty accepts
	// every file.
	include string
}

// frame is one directory part-way through being read.
type frame struct {
	path    string
	entries []fs.DirEntry
	next    int
}

// New starts a walk at root, which may be a single file or a directory.
//
// include is a filepath.Match glob tested against each file's base name; an
// empty string accepts everything. A root that is itself a file is always
// yielded: naming a file is a stronger statement of intent than a glob, and a
// caller who passes both and gets nothing has been told nothing.
func New(root, include string) (*Walker, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	w := &Walker{include: include}
	if !info.IsDir() {
		w.file = root
		return w, nil
	}
	first, err := readFrame(root)
	if err != nil {
		return nil, err
	}
	w.stack = []*frame{first}
	return w, nil
}

func readFrame(path string) (*frame, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	return &frame{path: path, entries: entries}, nil
}

// Next advances to the next file worth reading, reporting false when the walk
// is finished.
func (w *Walker) Next() (string, bool, error) {
	if w.file != "" {
		path := w.file
		w.file = ""
		return path, true, nil
	}
	for len(w.stack) > 0 {
		top := w.stack[len(w.stack)-1]
		if top.next >= len(top.entries) {
			w.stack = w.stack[:len(w.stack)-1]
			continue
		}
		entry := top.entries[top.next]
		top.next++
		path := filepath.Join(top.path, entry.Name())
		if entry.IsDir() {
			if Skipped[entry.Name()] {
				continue
			}
			child, err := readFrame(path)
			if err != nil {
				return "", false, err
			}
			w.stack = append(w.stack, child)
			continue
		}
		if w.include != "" {
			ok, err := filepath.Match(w.include, entry.Name())
			if err != nil || !ok {
				continue
			}
		}
		return path, true, nil
	}
	return "", false, nil
}
