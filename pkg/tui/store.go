package tui

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xen0bit/pwrq/pkg/ideengine"
)

// What the TUI remembers between runs: the queries that were run, the ones
// that were saved, and the last session.
//
// It lives in $XDG_STATE_HOME/pwrq, the place for state that is worth keeping
// but not worth backing up. The snippet file is in the page's export format
// ({"tool": "pwrq", "snippets": [...]}), so the page's Import reads it as it
// is, and a file the page exported can be imported here.
//
// Nothing here is allowed to break the UI: a store that cannot be read is
// empty, and one that cannot be written forgets.

const (
	historyLimit = 60
	snippetLimit = 200
	// storedInputLimit keeps a piped megabyte out of every history entry.
	// Past it an entry keeps its query and loses its input.
	storedInputLimit = 64 << 10
)

// Entry is a query worth coming back to: a run from history, or a snippet.
type Entry struct {
	Name  string          `json:"name,omitempty"`
	Query string          `json:"query"`
	Input string          `json:"input"`
	Args  []ideengine.Arg `json:"args"`
	At    int64           `json:"at,omitempty"`
}

// Store reads and writes the state directory. A Store with no directory
// remembers nothing, which is what tests and a home-less environment get.
type Store struct {
	dir string
}

// DefaultStateDir is $XDG_STATE_HOME/pwrq, or ~/.local/state/pwrq.
func DefaultStateDir() string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" && filepath.IsAbs(dir) {
		return filepath.Join(dir, "pwrq")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "state", "pwrq")
}

// OpenStore returns a store over dir; an empty dir remembers nothing.
func OpenStore(dir string) *Store { return &Store{dir: dir} }

func (s *Store) path(name string) string { return filepath.Join(s.dir, name) }

func (s *Store) read(name string, into any) {
	if s.dir == "" {
		return
	}
	data, err := os.ReadFile(s.path(name))
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, into)
}

// write replaces a file whole, through a temporary one, so a crash never
// leaves half a history. The files hold queries and sample data, so they are
// private to the user.
func (s *Store) write(name string, v any) error {
	if s.dir == "" {
		return nil
	}
	data, err := marshalReadable(v)
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path(name), data)
}

func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func clean(entries []Entry) []Entry {
	out := entries[:0]
	for _, entry := range entries {
		if strings.TrimSpace(entry.Query) != "" {
			out = append(out, entry)
		}
	}
	return out
}

// History is what was run, newest first.
func (s *Store) History() []Entry {
	var history []Entry
	s.read("history.json", &history)
	return clean(history)
}

// Record adds a run to the history. It records what was run, not what was
// typed, and a query run again moves to the top rather than appearing twice.
func (s *Store) Record(entry Entry) []Entry {
	if strings.TrimSpace(entry.Query) == "" {
		return s.History()
	}
	if len(entry.Input) > storedInputLimit {
		entry.Input = ""
	}
	entry.Name = ""
	entry.At = time.Now().UnixMilli()
	history := []Entry{entry}
	for _, item := range s.History() {
		if item.Query != entry.Query {
			history = append(history, item)
		}
	}
	if len(history) > historyLimit {
		history = history[:historyLimit]
	}
	_ = s.write("history.json", history)
	return history
}

// ClearHistory forgets every run.
func (s *Store) ClearHistory() {
	_ = s.write("history.json", []Entry{})
}

type snippetFile struct {
	Tool     string  `json:"tool"`
	Snippets []Entry `json:"snippets"`
}

// Snippets are the saved queries, newest first.
func (s *Store) Snippets() []Entry {
	var file snippetFile
	s.read("snippets.json", &file)
	return clean(file.Snippets)
}

func (s *Store) writeSnippets(snippets []Entry) error {
	return s.write("snippets.json", snippetFile{Tool: "pwrq", Snippets: snippets})
}

// SaveSnippet saves a query under a name, replacing one of the same name.
func (s *Store) SaveSnippet(entry Entry) ([]Entry, error) {
	entry.Name = truncateRunes(strings.TrimSpace(entry.Name), 120)
	if entry.Name == "" {
		return s.Snippets(), errors.New("a snippet needs a name")
	}
	if len(entry.Input) > storedInputLimit {
		entry.Input = ""
	}
	entry.At = time.Now().UnixMilli()
	snippets := []Entry{entry}
	for _, item := range s.Snippets() {
		if item.Name != entry.Name {
			snippets = append(snippets, item)
		}
	}
	if len(snippets) > snippetLimit {
		snippets = snippets[:snippetLimit]
	}
	return snippets, s.writeSnippets(snippets)
}

// DeleteSnippet forgets a saved query.
func (s *Store) DeleteSnippet(name string) []Entry {
	var kept []Entry
	for _, item := range s.Snippets() {
		if item.Name != name {
			kept = append(kept, item)
		}
	}
	_ = s.writeSnippets(kept)
	return kept
}

// ImportSnippets merges a file the page (or this) exported: either the
// export object or a bare array of snippets. The shapes are checked as the
// page checks them, and a malformed entry is skipped.
func (s *Store) ImportSnippets(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return 0, errors.New("that file is not JSON")
	}
	var incoming []any
	switch v := raw.(type) {
	case []any:
		incoming = v
	case map[string]any:
		incoming, _ = v["snippets"].([]any)
	}
	if incoming == nil {
		return 0, errors.New("that file has no snippets in it")
	}

	added := 0
	// Oldest first, so the file's own order survives being saved newest-first.
	for i := len(incoming) - 1; i >= 0; i-- {
		item, ok := incoming[i].(map[string]any)
		if !ok {
			continue
		}
		name, okName := item["name"].(string)
		query, okQuery := item["query"].(string)
		if !okName || !okQuery {
			continue
		}
		input, _ := item["input"].(string)
		entry := Entry{Name: name, Query: query, Input: input}
		if args, ok := item["args"].([]any); ok {
			for _, a := range args {
				if arg, ok := a.(map[string]any); ok {
					argName, _ := arg["name"].(string)
					argValue, _ := arg["value"].(string)
					if argName != "" {
						entry.Args = append(entry.Args, ideengine.Arg{Name: argName, Value: argValue})
					}
				}
			}
		}
		if _, err := s.SaveSnippet(entry); err == nil {
			added++
		}
	}
	if added == 0 {
		return 0, errors.New("that file had no usable snippets")
	}
	return added, nil
}

// ExportSnippets writes the snippets in the page's export format.
func (s *Store) ExportSnippets(path string) (int, error) {
	snippets := s.Snippets()
	data, err := marshalReadable(snippetFile{Tool: "pwrq", Snippets: snippets})
	if err != nil {
		return 0, err
	}
	return len(snippets), os.WriteFile(path, data, 0o600)
}

// Session is what was on screen when the TUI last closed.
type Session struct {
	Query string          `json:"query"`
	Input string          `json:"input"`
	Args  []ideengine.Arg `json:"args"`
}

// LoadSession returns the last session, or nothing.
func (s *Store) LoadSession() (Session, bool) {
	var session Session
	s.read("session.json", &session)
	return session, strings.TrimSpace(session.Query) != ""
}

// SaveSession remembers what is on screen. An input too large to be worth
// keeping is dropped rather than cut, since half a document is not a
// document.
func (s *Store) SaveSession(session Session) {
	if len(session.Input) > storedInputLimit {
		session.Input = ""
	}
	_ = s.write("session.json", session)
}

// marshalReadable indents, and leaves <, > and & as they are: these files
// hold queries, which are full of them, and a person may well read one.
func marshalReadable(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
