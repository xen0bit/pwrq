package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/graph"
)

type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayPalette
	overlayPrompt
	overlayKeys
)

// action is something the TUI can do. The palette lists them and the keys
// sheet documents them, from this one list, so neither can offer what the
// other does not.
type action struct {
	title string
	keys  string
	do    func() tea.Cmd
}

func (m *model) actions() []action {
	acts := []action{
		{"Run the query", "Ctrl-R  F5", m.startRun},
		{"Accept: exit and print the " + m.emitWhat(), "Ctrl-X", m.accept},
		{"Format the query", "Alt-F", func() tea.Cmd { return m.rewrite("format") }},
		{"Minify the query", "Alt-M", func() tea.Cmd { return m.rewrite("minify") }},
		{"Inline the query's definitions", "Alt-I", func() tea.Cmd { return m.rewrite("inline") }},
		{"Tidy the input JSON", "Alt-T", m.tidyInput},
		{"Toggle compact output (-c)", "Alt-C", func() tea.Cmd { return m.toggle("compact") }},
		{"Toggle raw output (-r)", "Alt-R", func() tea.Cmd { return m.toggle("raw") }},
		{"Toggle slurp (-s)", "Alt-S", func() tea.Cmd { return m.toggle("slurp") }},
		{"Toggle null input (-n)", "Alt-N", func() tea.Cmd { return m.toggle("null") }},
		{"Toggle raw input (-R)", "", func() tea.Cmd { return m.toggle("rawinput") }},
		{"Set the result limit…", "", m.promptLimit},
		{"Set the timeout…", "", m.promptTimeout},
		{"Save this query as a snippet…", "Ctrl-S", m.promptSaveSnippet},
		{"Copy a share link", "Alt-L", m.copyShareLink},
		{"Open a share link…", "", m.promptOpenShare},
		{"Copy the query", "", func() tea.Cmd { return m.copyToClipboard(m.query.Value(), "query") }},
		{"Copy the output", "", m.copyOutput},
		{"Save the output to a file…", "", m.promptSaveOutput},
		{"Copy the diagram's D2 source", "", m.copyD2},
		{"Save the diagram's D2 source…", "", m.promptSaveD2},
	}
	if m.opts.RenderSVG != nil {
		acts = append(acts, action{"Save the diagram as SVG…", "", m.promptSaveSVG})
	}
	acts = append(acts,
		action{"Import snippets from a file…", "", m.promptImport},
		action{"Export snippets to a file…", "", m.promptExport},
		action{"Clear the history", "", func() tea.Cmd {
			m.store.ClearHistory()
			m.history = nil
			m.refreshHistory()
			return m.flash("history cleared", false)
		}},
	)
	if m.originalInput != "" && m.input.Value() != m.originalInput {
		acts = append(acts, action{"Restore the original input", "", func() tea.Cmd {
			m.input.SetValue(m.originalInput)
			return m.flash("input restored", false)
		}})
	}
	acts = append(acts,
		action{"Clear the query", "", func() tea.Cmd {
			m.query.SetValue("")
			m.focus = paneQuery
			return m.scheduleValidate()
		}},
		action{"Keys", "F1  ?", func() tea.Cmd { m.overlay, m.keysTop = overlayKeys, 0; return nil }},
		action{"Quit without printing anything", "Ctrl-C  Ctrl-Q", func() tea.Cmd { m.result = Result{}; return tea.Quit }},
	)
	return acts
}

func (m *model) emitWhat() string {
	if m.opts.Emit == "output" {
		return "output"
	}
	return "query"
}

func (m *model) openPalette() {
	var items []item
	for _, a := range m.actions() {
		items = append(items, item{title: a.title, detail: a.keys, kind: "action", data: a.do})
	}
	for i, t := range tabOrder {
		t := t
		items = append(items, item{
			title: "Show " + tabNames[t], detail: fmt.Sprintf("Alt-%d  F%d", i+1, i+2), kind: "tab",
			data: func() tea.Cmd { m.selectTab(t); return nil },
		})
	}
	for _, s := range m.snippets {
		s := s
		items = append(items, item{title: s.Name, detail: oneLine(s.Query), kind: "snippet",
			data: func() tea.Cmd { return m.load(s.Query, s.Input, s.Args, "snippet "+s.Name) }})
	}
	for _, ex := range m.catalog.Examples {
		ex := ex
		items = append(items, item{title: ex.Title, detail: ex.Category, kind: "example",
			data: func() tea.Cmd { return m.load(ex.Query, ex.Input, ex.Args, "example") }})
	}
	for _, cmd := range m.catalog.Commands {
		name := cmd.Name
		items = append(items, item{title: name, detail: cmd.Description, kind: "cmdlet", search: strings.Join(cmd.Aliases, " "),
			data: func() tea.Cmd { return m.insertCommand(name) }})
	}
	m.palette = newList(items)
	m.overlay = overlayPalette
}

// promptState is a one-line question: a file name, a snippet's name, a
// number.
type promptState struct {
	title  string
	hint   string
	editor *Editor
	submit func(string) tea.Cmd
}

func (m *model) openPrompt(title, hint, initial string, submit func(string) tea.Cmd) tea.Cmd {
	e := NewEditor(initial)
	e.SetOffset(len([]rune(initial)))
	m.prompt = &promptState{title: title, hint: hint, editor: e, submit: submit}
	m.overlay = overlayPrompt
	return nil
}

func (m *model) handleOverlayKey(msg tea.KeyMsg) tea.Cmd {
	k := msg.String()
	switch m.overlay {
	case overlayPalette:
		switch k {
		case "esc", "ctrl+p":
			m.overlay = overlayNone
		case "enter":
			m.overlay = overlayNone
			if it, ok := m.palette.selected(); ok {
				return it.data.(func() tea.Cmd)()
			}
		default:
			m.palette.handleKey(msg, paletteRows)
		}

	case overlayPrompt:
		switch k {
		case "esc":
			m.overlay = overlayNone
		case "enter":
			m.overlay = overlayNone
			return m.prompt.submit(strings.TrimSpace(m.prompt.editor.Value()))
		default:
			// One line: a pasted newline is flattened rather than obeyed.
			if msg.Type == tea.KeyRunes && msg.Paste {
				msg.Runes = []rune(strings.ReplaceAll(string(msg.Runes), "\n", " "))
			}
			m.prompt.editor.HandleKey(msg, 1)
		}

	case overlayKeys:
		switch k {
		case "esc", "f1", "q", "?", "enter":
			m.overlay = overlayNone
		default:
			m.keysTop = scroll(k, m.keysTop, len(m.keySheet()), m.height-6)
		}
	}
	return nil
}

// paletteRows is how many rows of the palette show at once.
const paletteRows = 14

// keySheet is the keys overlay: the global keys, then the actions that have
// keys, then what each pane does with the rest.
func (m *model) keySheet() [][2]string {
	sheet := [][2]string{
		{"", "Anywhere"},
		{"Tab  Shift-Tab", "next / previous pane"},
		{"Alt-1…7  F2…F8", "Output, Diagram, Catalog, Examples, History, Input, Args"},
		{"Ctrl-P", "palette: every action, tab, example, snippet and cmdlet"},
		{"Esc", "close what is open, or cancel the run"},
	}
	for _, a := range m.actions() {
		if a.keys != "" {
			sheet = append(sheet, [2]string{a.keys, a.title})
		}
	}
	sheet = append(sheet,
		[2]string{"", ""},
		[2]string{"", "Editing (query, input, args)"},
		[2]string{"Ctrl-Space", "complete the name at the cursor; Tab accepts, ↑↓ choose"},
		[2]string{"Ctrl-A  Ctrl-E", "start / end of the line"},
		[2]string{"Ctrl-←  Ctrl-→", "previous / next word"},
		[2]string{"Ctrl-W  Ctrl-K  Ctrl-U", "delete the word before / to the end / to the start"},
		[2]string{"Ctrl-Z  Ctrl-Y", "undo / redo"},
		[2]string{"Ctrl-/", "comment the line out or back in"},
		[2]string{"", ""},
		[2]string{"", "Lists (catalog, examples, history)"},
		[2]string{"type", "filter"},
		[2]string{"Enter", "insert the cmdlet / load the example or entry"},
		[2]string{"?", "get_help for the cmdlet (catalog)"},
		[2]string{"Del", "delete the saved snippet (history)"},
		[2]string{"", ""},
		[2]string{"", "Nothing runs until you press Ctrl-R: edits are only validated."},
	)
	return sheet
}

// ---------------------------------------------------------------------------
// prompts

func (m *model) promptSaveSnippet() tea.Cmd {
	return m.openPrompt("Save this query as", "a name for the snippet", m.suggestName(), func(name string) tea.Cmd {
		snippets, err := m.store.SaveSnippet(Entry{Name: name, Query: m.query.Value(), Input: m.input.Value(), Args: parseArgs(m.args.Value())})
		if err != nil {
			return m.flash(err.Error(), true)
		}
		m.snippets = snippets
		m.refreshHistory()
		where := ""
		if m.opts.StateDir == "" {
			where = " (for this session only: there is nowhere to keep it)"
		}
		return m.flash("saved "+name+where, false)
	})
}

func (m *model) promptLimit() tea.Cmd {
	return m.openPrompt("Result limit", fmt.Sprintf("how many results a run may produce (at most %d)", tuiLimits.MaxResults),
		strconv.Itoa(m.set.limit), func(text string) tea.Cmd {
			n, err := strconv.Atoi(text)
			if err != nil || n <= 0 {
				return m.flash(fmt.Sprintf("%q is not a positive number", text), true)
			}
			m.set.limit = min(n, tuiLimits.MaxResults)
			m.settingsVersion++
			return m.flash(fmt.Sprintf("result limit %d", m.set.limit), false)
		})
}

func (m *model) promptTimeout() tea.Cmd {
	current := fmtDuration(m.set.timeoutMs)
	return m.openPrompt("Timeout", "how long a run may take: 30s, 5m, or seconds (at most 1h)", current, func(text string) tea.Cmd {
		d, err := time.ParseDuration(text)
		if err != nil {
			secs, serr := strconv.ParseFloat(text, 64)
			if serr != nil {
				return m.flash(fmt.Sprintf("%q is not a duration", text), true)
			}
			d = time.Duration(secs * float64(time.Second))
		}
		if d <= 0 {
			return m.flash("the timeout has to be positive", true)
		}
		m.set.timeoutMs = min(int(d/time.Millisecond), tuiLimits.MaxTimeoutMs)
		m.settingsVersion++
		return m.flash("timeout "+fmtDuration(m.set.timeoutMs), false)
	})
}

func (m *model) promptOpenShare() tea.Cmd {
	return m.openPrompt("Open a share link", "paste a link from the page or from here", "", m.openShare)
}

func (m *model) outputText() string {
	if m.last == nil {
		return ""
	}
	var b strings.Builder
	for _, v := range m.last.Values {
		b.WriteString(v)
		b.WriteByte('\n')
	}
	return b.String()
}

func (m *model) copyOutput() tea.Cmd {
	if m.last == nil {
		return m.flash("nothing has run yet", true)
	}
	return m.copyToClipboard(m.outputText(), fmt.Sprintf("output (%d results)", m.last.Count))
}

func (m *model) promptSaveOutput() tea.Cmd {
	if m.last == nil {
		return m.flash("nothing has run yet", true)
	}
	name := "pwrq-output.json"
	if m.set.raw {
		name = "pwrq-output.txt"
	}
	return m.openPrompt("Save the output to", "a file, relative to "+m.cwd, name, func(path string) tea.Cmd {
		written, err := writeOut(path, m.outputText())
		if err != nil {
			return m.flash(err.Error(), true)
		}
		return m.flash("output saved to "+written, false)
	})
}

// d2 renders the query's diagram as a D2 script.
func (m *model) d2() (string, *gojq.Query, error) {
	query, err := gojq.Parse(m.query.Value())
	if err != nil {
		return "", nil, err
	}
	return graph.RenderD2Opts(query, m.diagramOptions()), query, nil
}

func (m *model) diagramOptions() graph.RenderOptions {
	theme := "light"
	if m.th.r.HasDarkBackground() {
		theme = "dark"
	}
	return graph.RenderOptions{Cmdlets: m.vocab.Cmdlets, Theme: theme}
}

func (m *model) copyD2() tea.Cmd {
	script, _, err := m.d2()
	if err != nil {
		return m.flash("the query does not parse, so there is no diagram", true)
	}
	return m.copyToClipboard(script, "D2 source")
}

func (m *model) promptSaveD2() tea.Cmd {
	return m.openPrompt("Save the D2 source to", "a file, relative to "+m.cwd, "query.d2", func(path string) tea.Cmd {
		script, _, err := m.d2()
		if err != nil {
			return m.flash("the query does not parse, so there is no diagram", true)
		}
		written, err := writeOut(path, script)
		if err != nil {
			return m.flash(err.Error(), true)
		}
		return m.flash("D2 source saved to "+written, false)
	})
}

func (m *model) promptSaveSVG() tea.Cmd {
	return m.openPrompt("Save the diagram as SVG to", "a file, relative to "+m.cwd, "query.svg", func(path string) tea.Cmd {
		_, query, err := m.d2()
		if err != nil {
			return m.flash("the query does not parse, so there is no diagram", true)
		}
		svg, err := m.opts.RenderSVG(query, m.diagramOptions())
		if err != nil {
			return m.flash(err.Error(), true)
		}
		written, err := writeOut(path, svg)
		if err != nil {
			return m.flash(err.Error(), true)
		}
		return m.flash("diagram saved to "+written, false)
	})
}

func (m *model) promptImport() tea.Cmd {
	return m.openPrompt("Import snippets from", "a file exported by the page or by this", "pwrq-snippets.json", func(path string) tea.Cmd {
		n, err := m.store.ImportSnippets(path)
		if err != nil {
			return m.flash(err.Error(), true)
		}
		m.snippets = m.store.Snippets()
		m.refreshHistory()
		return m.flash(fmt.Sprintf("imported %d snippet%s", n, plural(n)), false)
	})
}

func (m *model) promptExport() tea.Cmd {
	return m.openPrompt("Export snippets to", "a file the page's Import can read", "pwrq-snippets.json", func(path string) tea.Cmd {
		n, err := m.store.ExportSnippets(path)
		if err != nil {
			return m.flash(err.Error(), true)
		}
		return m.flash(fmt.Sprintf("exported %d snippet%s to %s", n, plural(n), path), false)
	})
}
