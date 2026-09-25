package tui

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// The model is tested the way Bubble Tea drives it: messages in, a frame
// out. Commands are run as the runtime would run them, concurrently, and a
// test waits until what it expects has happened - or the deadline has.

var update = flag.Bool("update", false, "rewrite the golden frames")

type harness struct {
	t    *testing.T
	m    *model
	msgs chan tea.Msg
	quit bool
}

func newHarness(t *testing.T, opts Options) *harness {
	t.Helper()
	if opts.StateDir == "" {
		opts.StateDir = t.TempDir()
	}
	m, err := newModel(opts, plainTheme())
	if err != nil {
		t.Fatal(err)
	}
	// What the header says about the machine varies; the frame should not.
	m.userName, m.cwd = "user", "/work"
	h := &harness{t: t, m: m, msgs: make(chan tea.Msg, 256)}
	h.apply(tea.WindowSizeMsg{Width: 100, Height: 30})
	h.exec(m.Init())
	return h
}

// exec runs a command in the background, as the runtime does, delivering
// what it returns.
func (h *harness) exec(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	go func() {
		msg := cmd()
		if msg != nil {
			h.msgs <- msg
		}
	}()
}

func (h *harness) apply(msg tea.Msg) {
	switch msg := msg.(type) {
	case tea.BatchMsg:
		for _, cmd := range msg {
			h.exec(cmd)
		}
		return
	case tea.QuitMsg:
		h.quit = true
		return
	}
	_, cmd := h.m.Update(msg)
	h.exec(cmd)
}

// until delivers messages until cond holds, failing if it does not within
// the deadline.
func (h *harness) until(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.After(10 * time.Second)
	for !cond() {
		select {
		case msg := <-h.msgs:
			h.apply(msg)
		case <-deadline:
			h.t.Fatalf("timed out waiting for %s\n%s", what, h.frame())
		}
	}
}

// settle delivers whatever arrives in the next moment: enough for a debounce
// and a validation to land.
func (h *harness) settle() {
	quiet := time.After(400 * time.Millisecond)
	for {
		select {
		case msg := <-h.msgs:
			h.apply(msg)
		case <-quiet:
			return
		}
	}
}

func (h *harness) press(keys ...string) {
	for _, k := range keys {
		h.apply(key(k))
	}
}

// typeText types text a rune at a time, as a user would.
func (h *harness) typeText(text string) {
	for _, r := range text {
		if r == ' ' {
			h.apply(key(" "))
		} else {
			h.apply(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
	}
}

func (h *harness) frame() string { return ansi.Strip(h.m.View()) }

func (h *harness) validated() bool {
	return h.m.validatedFor.query == h.m.query.Version && h.m.validateGen > 0 &&
		(h.m.validation.OK || h.m.validation.Error != "" || h.m.validation.Empty)
}

// TestEditingNeverRuns is the TUI's safety rule: typing, deleting, loading
// and rewriting a query that writes to disk must not write to disk. Only
// Ctrl-R may.
func TestEditingNeverRuns(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "marker")
	h := newHarness(t, Options{Query: " "})
	h.press("ctrl+a", "ctrl+k")

	query := `new_item("` + marker + `")`
	// Every prefix is typed; enough of them are validated along the way to
	// catch a run that validation, or anything else, might start.
	for i, r := range query {
		h.typeText(string(r))
		if i%8 == 7 {
			h.settle()
		}
	}
	h.press("alt+f", "alt+m", "alt+i", "ctrl+z", "ctrl+y")
	h.settle()
	if !h.m.validation.OK {
		t.Fatalf("the query should compile: %+v", h.m.validation)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("editing ran the query: the marker file exists")
	}
	if h.m.last != nil {
		t.Fatal("editing produced a run")
	}

	h.press("ctrl+r")
	h.until("the run", func() bool { return h.m.last != nil })
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("Ctrl-R should have run the query: %v", err)
	}
}

func TestRunShowsTheOutput(t *testing.T) {
	h := newHarness(t, Options{Query: ".a[]", Input: `{"a":[1,{"b":"two"}]}`})
	h.press("ctrl+r")
	h.until("the run", func() bool { return h.m.last != nil })
	frame := h.frame()
	for _, want := range []string{"2 results", "from 1 input", `"b": "two"`} {
		if !strings.Contains(frame, want) {
			t.Errorf("the output pane should show %q\n%s", want, frame)
		}
	}
}

// TestANarrowRunShowsTheOutput: where the input and output share a pane, a
// run from the input brings the output forward.
func TestANarrowRunShowsTheOutput(t *testing.T) {
	h := newHarness(t, Options{Query: ".", Input: `{"seen":true}`})
	h.apply(tea.WindowSizeMsg{Width: 64, Height: 22})
	h.press("alt+6", "ctrl+r")
	h.until("the run", func() bool { return h.m.last != nil })
	if h.m.focus != paneRight || !strings.Contains(h.frame(), "1 result") {
		t.Errorf("the output should be showing\n%s", h.frame())
	}
}

func TestOutputSaysWhenItIsStale(t *testing.T) {
	h := newHarness(t, Options{Query: ".", Input: `1`})
	h.press("ctrl+r")
	h.until("the run", func() bool { return h.m.last != nil })
	if h.m.stale() {
		t.Fatal("fresh output is not stale")
	}
	h.typeText("a")
	if !h.m.stale() || !strings.Contains(h.frame(), "stale") {
		t.Errorf("editing after a run should mark the output stale\n%s", h.frame())
	}
}

func TestValidationPointsAtTheError(t *testing.T) {
	h := newHarness(t, Options{Query: ".a | ]"})
	h.until("validation", h.validated)
	if h.m.validation.OK {
		t.Fatal("the query does not parse")
	}
	if span := h.m.errorSpan(); span == nil || span.Start != 5 {
		t.Errorf("the error should be at the ], got %+v", span)
	}
	if !strings.Contains(h.frame(), "✗ 1:6") {
		t.Errorf("the status line should locate the error\n%s", h.frame())
	}
}

// TestValidationCompiles is the native engine's advantage over the page's
// WASM one: an unknown name is caught while typing.
func TestValidationCompiles(t *testing.T) {
	h := newHarness(t, Options{Query: "no_such_thing(1)"})
	h.until("validation", h.validated)
	if h.m.validation.OK || !strings.Contains(h.m.validation.Error, "no_such_thing") {
		t.Errorf("validation = %+v", h.m.validation)
	}
}

func TestEscCancelsARun(t *testing.T) {
	h := newHarness(t, Options{Query: "repeat(1) | select(. == 0)", NullInput: true})
	h.press("ctrl+r")
	h.until("the run to start", func() bool { return h.m.running })
	h.press("esc")
	h.until("the run to stop", func() bool { return h.m.last != nil })
	if h.m.last.Kind != "cancelled" {
		t.Errorf("kind = %q", h.m.last.Kind)
	}
	if !strings.Contains(h.frame(), "cancelled") {
		t.Errorf("the output should say it was cancelled\n%s", h.frame())
	}
}

func TestAcceptPrintsTheQuery(t *testing.T) {
	h := newHarness(t, Options{Query: ".a | .b"})
	h.press("ctrl+x")
	h.until("quit", func() bool { return h.quit })
	if h.m.result.Emit != ".a | .b\n" {
		t.Errorf("emit = %q", h.m.result.Emit)
	}
	if h.m.last != nil {
		t.Error("accepting the query should not run it")
	}
}

// TestAcceptOutputRunsFirst: the output printed is always the query's, never
// a stale one.
func TestAcceptOutputRunsFirst(t *testing.T) {
	h := newHarness(t, Options{Query: ".[]", Input: `[1,2]`, Emit: "output", Compact: true})
	h.press("ctrl+x")
	h.until("quit", func() bool { return h.quit })
	if h.m.result.Emit != "1\n2\n" {
		t.Errorf("emit = %q", h.m.result.Emit)
	}
}

func TestQuitPrintsNothing(t *testing.T) {
	h := newHarness(t, Options{Query: "."})
	h.press("ctrl+c")
	h.until("quit", func() bool { return h.quit })
	if h.m.result.Emit != "" {
		t.Errorf("emit = %q", h.m.result.Emit)
	}
}

func TestCompletionOffersAndAccepts(t *testing.T) {
	h := newHarness(t, Options{Query: " "})
	h.press("ctrl+a", "ctrl+k")
	h.typeText("sha2")
	if h.m.complete == nil || h.m.complete.matches[0].name != "sha224" && h.m.complete.matches[0].name != "sha256" {
		t.Fatalf("typing sha2 should offer the sha2 family, got %+v", h.m.complete)
	}
	if !strings.Contains(h.frame(), "sha256") {
		t.Errorf("the completion list should be drawn\n%s", h.frame())
	}
	for h.m.complete.matches[h.m.complete.sel].name != "sha256" {
		h.press("down")
	}
	h.press("tab")
	if got := h.m.query.Value(); got != "sha256" {
		t.Errorf("query = %q", got)
	}
	if h.m.focus != paneQuery {
		t.Error("Tab accepted a completion; it should not also move the focus")
	}
}

func TestCompletionStaysOutOfStrings(t *testing.T) {
	h := newHarness(t, Options{Query: " "})
	h.press("ctrl+a", "ctrl+k")
	h.typeText(`"sha2`)
	if h.m.complete != nil {
		t.Error("a string is not a place for a function name")
	}
}

func TestFormatIsUndoable(t *testing.T) {
	h := newHarness(t, Options{Query: "{a:1,b:[.x,.y]}|.a"})
	before := h.m.query.Value()
	h.press("alt+f")
	if h.m.query.Value() == before {
		t.Fatal("format changed nothing")
	}
	h.press("ctrl+z")
	if h.m.query.Value() != before {
		t.Errorf("undo = %q", h.m.query.Value())
	}
}

func TestCatalogInsertsACmdlet(t *testing.T) {
	h := newHarness(t, Options{Query: " "})
	h.press("ctrl+a", "ctrl+k", "alt+3")
	if h.m.focus != paneRight || h.m.rightTab != tabCatalog {
		t.Fatal("Alt-3 should show and focus the catalog")
	}
	h.typeText("base64_encode")
	h.press("enter")
	if got := h.m.query.Value(); !strings.HasPrefix(got, "base64_encode") || h.m.focus != paneQuery {
		t.Errorf("query = %q, focus = %v", got, h.m.focus)
	}
}

func TestCatalogShowsHelp(t *testing.T) {
	h := newHarness(t, Options{})
	h.press("alt+3")
	h.typeText("new_item")
	h.press("?")
	h.until("help", func() bool { return len(h.m.helpText) > 1 })
	if !strings.Contains(h.frame(), "SYNOPSIS") {
		t.Errorf("get_help's text should be shown\n%s", h.frame())
	}
	h.press("esc")
	if h.m.helpFor != "" {
		t.Error("Esc should go back to the list")
	}
}

func TestExamplesLoad(t *testing.T) {
	h := newHarness(t, Options{Query: ".", Input: `{"mine":true}`})
	h.press("alt+4", "enter")
	ex := h.m.catalog.Examples[0]
	if h.m.query.Value() != ex.Query {
		t.Errorf("query = %q, want %q", h.m.query.Value(), ex.Query)
	}
	h.press("ctrl+p")
	h.typeText("restore the original input")
	h.press("enter")
	if h.m.input.Value() != `{"mine":true}` {
		t.Errorf("the palette should restore the input, got %q", h.m.input.Value())
	}
}

func TestSnippetsSaveAndLoadThroughHistory(t *testing.T) {
	h := newHarness(t, Options{Query: ".kept"})
	h.press("ctrl+s")
	if h.m.overlay != overlayPrompt {
		t.Fatal("Ctrl-S should ask for a name")
	}
	h.press("ctrl+u")
	h.typeText("my snippet")
	h.press("enter")
	if len(h.m.snippets) != 1 || h.m.snippets[0].Name != "my snippet" {
		t.Fatalf("snippets = %+v", h.m.snippets)
	}
	h.m.query.SetValue(".other")
	h.press("alt+5")
	h.typeText("my snip")
	h.press("enter")
	if h.m.query.Value() != ".kept" {
		t.Errorf("query = %q", h.m.query.Value())
	}
}

func TestShareLinkGoesToTheClipboard(t *testing.T) {
	h := newHarness(t, Options{Query: ".a", Input: `{"a":1}`})
	h.press("alt+l")
	if !strings.HasPrefix(h.m.osc, "\x1b]52;c;") {
		t.Fatalf("osc = %q", h.m.osc)
	}
	if !strings.HasPrefix(h.m.View(), "\x1b]52;c;") {
		t.Error("the clipboard sequence should go out with the next frame")
	}
}

func TestOpenSharedLinkOnStart(t *testing.T) {
	link, err := ShareLink(DefaultShareBase, Shared{Query: ".x", Input: `{"x":2}`, Options: map[string]any{"output": "compact"}})
	if err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, Options{Share: link})
	if h.m.query.Value() != ".x" || h.m.input.Value() != `{"x":2}` || !h.m.set.compact {
		t.Errorf("query=%q input=%q compact=%v", h.m.query.Value(), h.m.input.Value(), h.m.set.compact)
	}
}

func TestArgsBindVariables(t *testing.T) {
	h := newHarness(t, Options{Query: "$n + 1", NullInput: true})
	h.until("validation", h.validated)
	if h.m.validation.OK {
		t.Fatal("$n is unbound")
	}
	h.press("alt+7")
	h.typeText("n = 41")
	h.until("validation", func() bool { return h.validated() && h.m.validatedFor.args == h.m.args.Version })
	if !h.m.validation.OK {
		t.Fatalf("$n is bound now: %+v", h.m.validation)
	}
	h.press("ctrl+r")
	h.until("the run", func() bool { return h.m.last != nil })
	if len(h.m.last.Values) != 1 || h.m.last.Values[0] != "42" {
		t.Errorf("values = %v (%s)", h.m.last.Values, h.m.last.Error)
	}
}

func TestDebugIsCapturedNotPrinted(t *testing.T) {
	h := newHarness(t, Options{Query: `"x" | debug | stderr | empty`, NullInput: true})
	h.press("ctrl+r")
	h.until("the run", func() bool { return h.m.last != nil })
	frame := h.frame()
	if !strings.Contains(frame, `["DEBUG:","x"]`) || !strings.Contains(frame, "debug and stderr") {
		t.Errorf("debug output should appear in the output pane\n%s", frame)
	}
}

// TestEveryFrameFits keeps the layout inside the terminal whatever is open:
// a line wider than the terminal wraps, and the whole frame shifts.
func TestEveryFrameFits(t *testing.T) {
	h := newHarness(t, Options{Query: "[.[] | select(.Size > 1000)]", Input: `[{"Size":2000,"Name":"日本語🙂"}]`})
	h.press("ctrl+r")
	h.until("the run", func() bool { return h.m.last != nil })

	states := map[string]func(){
		"plain":      func() {},
		"diagram":    func() { h.press("alt+2") },
		"catalog":    func() { h.press("alt+3") },
		"examples":   func() { h.press("alt+4") },
		"history":    func() { h.press("alt+5") },
		"args":       func() { h.press("alt+7") },
		"palette":    func() { h.press("ctrl+p") },
		"keys":       func() { h.press("f1") },
		"prompt":     func() { h.press("ctrl+s") },
		"completion": func() { h.press("tab", "tab", "end"); h.typeText(" | sha") },
	}
	for _, size := range [][2]int{{100, 30}, {140, 45}, {60, 20}, {40, 12}} {
		for name, setup := range states {
			h.apply(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			h.m.overlay, h.m.complete, h.m.focus = overlayNone, nil, paneQuery
			setup()
			lines := strings.Split(h.m.View(), "\n")
			if len(lines) != size[1] {
				t.Errorf("%s at %dx%d: %d lines", name, size[0], size[1], len(lines))
			}
			for i, line := range lines {
				if w := ansi.StringWidth(line); w != size[0] {
					t.Errorf("%s at %dx%d: line %d is %d wide: %q", name, size[0], size[1], i, w, ansi.Strip(line))
				}
			}
		}
	}
}

// TestGoldenFrames pins what the screen looks like, colour aside. Run with
// -update to rewrite them after a deliberate change.
func TestGoldenFrames(t *testing.T) {
	for _, tc := range []struct {
		name          string
		width, height int
		setup         func(h *harness)
	}{
		{"wide", 100, 30, func(h *harness) {}},
		{"narrow", 64, 22, func(h *harness) {}},
		{"diagram", 100, 30, func(h *harness) { h.press("alt+2") }},
		{"palette", 100, 30, func(h *harness) { h.press("ctrl+p"); h.typeText("format") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, Options{RestoreSession: true, InputLabel: "stdin, 1.2 KB"})
			h.apply(tea.WindowSizeMsg{Width: tc.width, Height: tc.height})
			h.until("validation", h.validated)
			tc.setup(h)
			got := h.frame()
			path := filepath.Join("testdata", tc.name+".golden")
			if *update {
				if err := os.MkdirAll("testdata", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run with -update to create it)", err)
			}
			if got != string(want) {
				t.Errorf("frame differs from %s\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
			}
		})
	}
}
