package tui

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/xen0bit/pwrq/pkg/ideengine"
)

// The panes, and the tabs they hold. On a wide terminal the input and the
// arguments sit beside the rest; on a narrow one every tab shares one pane.
type pane int

const (
	paneQuery pane = iota
	paneLeft
	paneRight
)

type tab int

const (
	tabInput tab = iota
	tabArgs
	tabOutput
	tabDiagram
	tabCatalog
	tabExamples
	tabHistory
)

var tabNames = map[tab]string{
	tabInput: "Input", tabArgs: "Args", tabOutput: "Output", tabDiagram: "Diagram",
	tabCatalog: "Catalog", tabExamples: "Examples", tabHistory: "History",
}

// tabOrder is the order Alt-1…7 and F2…F8 select tabs in: the ones for
// looking first, then the ones for editing.
var tabOrder = []tab{tabOutput, tabDiagram, tabCatalog, tabExamples, tabHistory, tabInput, tabArgs}

var (
	leftTabs  = []tab{tabInput, tabArgs}
	rightTabs = []tab{tabOutput, tabDiagram, tabCatalog, tabExamples, tabHistory}
)

func (t tab) onLeft() bool { return t == tabInput || t == tabArgs }

// validateDelay debounces validation: long enough not to compile on every
// keystroke of a word, short enough that an error appears as typing stops.
const validateDelay = 120 * time.Millisecond

// demoQuery and demoInput are what the page shows on a first visit, and what
// the TUI shows when it has nothing else: something that works, rather than
// an empty box.
const (
	demoQuery = "[.[] | select(.Size > 1000) | {Name, Hash: (.Name | sha256)}]\n| sort_by(.Name)"
	demoInput = "[{\"Name\":\"notes.txt\",\"Size\":812},\n {\"Name\":\"report.pdf\",\"Size\":48211},\n {\"Name\":\"image.png\",\"Size\":10240}]"
)

type settings struct {
	raw, compact, slurp, nullInput, rawInput bool
	indent                                   int
	tab                                      bool
	limit, timeoutMs                         int
}

// versions identifies what a run was made from, so the output can say when
// it no longer matches what is on screen.
type versions struct{ query, input, args, settings int }

type model struct {
	opts   Options
	run    *runner
	store  *Store
	th     *theme
	width  int
	height int

	query, input, args *Editor
	focus              pane
	leftTab, rightTab  tab
	set                settings
	settingsVersion    int

	// The vocabulary, from the engine's catalog.
	catalog     ideengine.CatalogResponse
	vocab       Vocabulary
	commands    map[string]ideengine.Command
	completions []completion

	// Validation of the query and arguments, and the versions it answered
	// for.
	validateGen  int
	validation   ideengine.ValidateResponse
	validatedFor versions

	// The query's colours, cached by its version.
	kindsVersion int
	queryKinds   [][]string

	// The run in flight, and the last one that finished.
	runGen     int
	running    bool
	cancel     context.CancelFunc
	runStarted time.Time
	runFor     versions
	last       *ideengine.RunResponse
	lastFor    versions
	// runRaw and lastRaw say whether a run printed raw text, which is not
	// JSON and is not coloured as if it were.
	runRaw    bool
	lastRaw   bool
	outLines  []outLine
	outTop    int
	outLeft   int
	exitOnRun bool

	// The diagram, cached by the query's version.
	outlineVersion int
	outlineLines   []outlineLine
	outlineErr     string
	diagTop        int

	catalogList  *list
	examplesList *list
	historyList  *list
	// helpFor is the cmdlet whose get_help the catalog is showing.
	helpFor  string
	helpText []string
	helpTop  int

	overlay  overlayKind
	palette  *list
	prompt   *promptState
	keysTop  int
	complete *completeState

	flashText string
	flashErr  bool
	flashID   int
	// osc is a terminal control string drawn with the next frame: the
	// clipboard. See copyToClipboard.
	osc   string
	oscID int

	history  []Entry
	snippets []Entry

	originalInput string
	userName      string
	cwd           string

	result Result
}

// outLine is one line of the output pane: a line of a value, or of what the
// query wrote to debug and stderr.
type outLine struct {
	text  string
	debug bool
	// head marks a line that labels the lines after it.
	head bool
}

func newModel(opts Options, th *theme) (*model, error) {
	m := &model{
		opts:     opts,
		th:       th,
		store:    OpenStore(opts.StateDir),
		width:    100,
		height:   30,
		focus:    paneQuery,
		leftTab:  tabInput,
		rightTab: tabOutput,
		set: settings{
			raw: opts.Raw, compact: opts.Compact, slurp: opts.Slurp,
			nullInput: opts.NullInput, rawInput: opts.RawInput,
			indent: opts.Indent, tab: opts.Tab,
			limit: tuiLimits.DefaultResults, timeoutMs: tuiLimits.DefaultTimeoutMs,
		},
	}
	m.run = newRunner(opts.RunOptions, opts.Version)

	query, input, args := opts.Query, opts.Input, opts.Args
	switch {
	case opts.Share != "":
		shared, err := DecodeShare(opts.Share)
		if err != nil {
			return nil, fmt.Errorf("cannot open the share link: %w", err)
		}
		query = shared.Query
		// Data given on the command line is what the user asked to look at;
		// the link's sample stands in only when there is none.
		if input == "" {
			input = shared.Input
		}
		if len(args) == 0 {
			args = shared.Args
		}
		m.applySharedOptions(shared.Options)
	case query == "" && opts.RestoreSession:
		if session, ok := m.store.LoadSession(); ok {
			query, input, args = session.Query, session.Input, session.Args
		} else {
			query, input = demoQuery, demoInput
		}
	}
	if strings.TrimSpace(query) == "" {
		query = "."
	}

	m.query = NewEditor(query)
	m.query.SetOffset(len([]rune(query)))
	m.input = NewEditor(input)
	m.args = NewEditor(formatArgs(args))
	m.originalInput = input

	m.catalog = m.run.eng.Catalog()
	m.vocab = Vocabulary{Cmdlets: map[string]bool{}, Builtins: map[string]bool{}}
	for _, name := range m.catalog.Cmdlets {
		m.vocab.Cmdlets[name] = true
	}
	for _, alias := range m.catalog.Aliases {
		m.vocab.Cmdlets[alias.Name] = true
	}
	for _, name := range m.catalog.Builtins {
		m.vocab.Builtins[name] = true
	}
	m.commands = map[string]ideengine.Command{}
	for _, cmd := range m.catalog.Commands {
		m.commands[cmd.Name] = cmd
	}
	m.completions = buildCompletions(m.catalog)
	m.catalogList = newList(catalogItems(m.catalog))
	m.examplesList = newList(exampleItems(m.catalog.Examples))
	m.history, m.snippets = m.store.History(), m.store.Snippets()
	m.historyList = newList(nil)
	m.refreshHistory()

	if u, err := user.Current(); err == nil {
		m.userName = u.Username
	}
	m.cwd, _ = os.Getwd()
	return m, nil
}

// applySharedOptions takes the page's settings from a link: the output mode
// and the input flags. The page's diagram settings mean nothing here.
func (m *model) applySharedOptions(options map[string]any) {
	if output, ok := options["output"].(string); ok {
		m.set.raw, m.set.compact = output == "raw", output == "compact"
	}
	if slurp, ok := options["slurp"].(bool); ok {
		m.set.slurp = slurp
	}
	if null, ok := options["nullInput"].(bool); ok {
		m.set.nullInput = null
	}
}

func (m *model) sharedOptions() map[string]any {
	output := "pretty"
	switch {
	case m.set.raw:
		output = "raw"
	case m.set.compact:
		output = "compact"
	}
	return map[string]any{"output": output, "slurp": m.set.slurp, "nullInput": m.set.nullInput}
}

// formatArgs and parseArgs are the Args pane's text: one binding per line,
// `name = JSON value`.
func formatArgs(args []ideengine.Arg) string {
	lines := make([]string, 0, len(args))
	for _, arg := range args {
		lines = append(lines, strings.TrimPrefix(arg.Name, "$")+" = "+arg.Value)
	}
	return strings.Join(lines, "\n")
}

func parseArgs(text string) []ideengine.Arg {
	var args []ideengine.Arg
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, found := strings.Cut(line, "=")
		if !found {
			// A bare name binds null, so a half-written line still names
			// the variable and the query compiles.
			value = "null"
		}
		args = append(args, ideengine.Arg{
			Name:  strings.TrimPrefix(strings.TrimSpace(name), "$"),
			Value: strings.TrimSpace(value),
		})
	}
	return args
}

func (m *model) current() versions {
	return versions{m.query.Version, m.input.Version, m.args.Version, m.settingsVersion}
}

// stale reports whether the output was made from something other than what
// is on screen.
func (m *model) stale() bool { return m.last != nil && m.lastFor != m.current() }

func (m *model) Init() tea.Cmd {
	return m.scheduleValidate()
}

// ---------------------------------------------------------------------------
// messages

type validateTickMsg struct{ gen int }

type validateMsg struct {
	gen  int
	of   versions
	resp ideengine.ValidateResponse
}

type runMsg struct {
	gen   int
	resp  ideengine.RunResponse
	debug []string
}

type runTickMsg struct{ gen int }

type helpMsg struct {
	name string
	text string
}

type flashClearMsg struct{ id int }

type oscClearMsg struct{ id int }

func (m *model) scheduleValidate() tea.Cmd {
	m.validateGen++
	gen := m.validateGen
	return tea.Tick(validateDelay, func(time.Time) tea.Msg { return validateTickMsg{gen} })
}

// validate compiles the query off the UI goroutine. It never runs it:
// nothing the TUI does between keystrokes may touch the machine.
func (m *model) validate(gen int) tea.Cmd {
	req := ideengine.ValidateRequest{Query: m.query.Value(), Args: parseArgs(m.args.Value())}
	of := m.current()
	eng := m.run.eng
	return func() tea.Msg { return validateMsg{gen: gen, of: of, resp: eng.Validate(req)} }
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		return m, m.handleKey(msg)

	case validateTickMsg:
		if msg.gen != m.validateGen {
			return m, nil
		}
		return m, m.validate(msg.gen)

	case validateMsg:
		// validatedFor moves with the answer, not the question, so the
		// status line never presents an old answer as the current one.
		if msg.gen == m.validateGen {
			m.validation = msg.resp
			m.validatedFor = msg.of
		}
		return m, nil

	case runMsg:
		return m, m.finishRun(msg)

	case runTickMsg:
		if m.running && msg.gen == m.runGen {
			return m, runTick(msg.gen)
		}
		return m, nil

	case helpMsg:
		if msg.name == m.helpFor {
			m.helpText = strings.Split(strings.TrimRight(msg.text, "\n"), "\n")
			m.helpTop = 0
		}
		return m, nil

	case flashClearMsg:
		if msg.id == m.flashID {
			m.flashText = ""
		}
		return m, nil

	case oscClearMsg:
		if msg.id == m.oscID {
			m.osc = ""
		}
		return m, nil
	}
	return m, nil
}

func runTick(gen int) tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg { return runTickMsg{gen} })
}

// flash says something in the status line for a few seconds.
func (m *model) flash(text string, isErr bool) tea.Cmd {
	m.flashID++
	id := m.flashID
	m.flashText, m.flashErr = text, isErr
	return tea.Tick(4*time.Second, func(time.Time) tea.Msg { return flashClearMsg{id} })
}

// ---------------------------------------------------------------------------
// running

func (m *model) runRequest() ideengine.RunRequest {
	return ideengine.RunRequest{
		Query:      m.query.Value(),
		Input:      m.input.Value(),
		RawInput:   m.set.rawInput,
		Slurp:      m.set.slurp,
		NullInput:  m.set.nullInput,
		Raw:        m.set.raw,
		Compact:    m.set.compact,
		Indent:     m.set.indent,
		Tab:        m.set.tab,
		Limit:      m.set.limit,
		TimeoutMs:  m.set.timeoutMs,
		Args:       parseArgs(m.args.Value()),
		InputName:  m.opts.InputName,
		Positional: m.opts.Positional,
	}
}

// startRun runs the query. It is the only way anything runs: a key the user
// pressed, never an edit. A run already in flight is cancelled first, and
// what it reports when it stops is dropped.
func (m *model) startRun() tea.Cmd {
	if strings.TrimSpace(m.query.Value()) == "" {
		return m.flash("the query is empty", true)
	}
	if m.cancel != nil {
		m.cancel()
	}
	m.runGen++
	gen := m.runGen
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.running = true
	m.runStarted = time.Now()
	m.runFor = m.current()
	m.runRaw = m.set.raw

	req := m.runRequest()
	m.history = m.store.Record(Entry{Query: req.Query, Input: req.Input, Args: req.Args})
	m.refreshHistory()

	r := m.run
	return tea.Batch(
		func() tea.Msg {
			resp, debug := r.run(ctx, req)
			return runMsg{gen: gen, resp: resp, debug: debug}
		},
		runTick(gen),
	)
}

func (m *model) cancelRun() tea.Cmd {
	if !m.running || m.cancel == nil {
		return nil
	}
	m.cancel()
	return m.flash("cancelling…", false)
}

func (m *model) finishRun(msg runMsg) tea.Cmd {
	if msg.gen != m.runGen {
		return nil
	}
	m.running = false
	m.cancel = nil
	resp := msg.resp
	m.last = &resp
	m.lastFor = m.runFor
	m.lastRaw = m.runRaw
	m.outTop, m.outLeft = 0, 0

	m.outLines = m.outLines[:0]
	for _, value := range resp.Values {
		for _, line := range strings.Split(value, "\n") {
			m.outLines = append(m.outLines, outLine{text: line})
		}
	}
	if len(msg.debug) > 0 {
		m.outLines = append(m.outLines, outLine{text: "debug and stderr", head: true, debug: true})
		for _, line := range msg.debug {
			m.outLines = append(m.outLines, outLine{text: line, debug: true})
		}
	}

	if m.exitOnRun {
		m.exitOnRun = false
		if resp.Error != "" && resp.Kind != "limit" {
			return m.flash("not exiting: the run failed - "+firstLine(resp.Error), true)
		}
		return m.acceptOutput()
	}
	// A run started from another tab shows its output. On a narrow
	// terminal the input and the output share one pane, so the output has
	// to take it.
	m.rightTab = tabOutput
	if m.focus == paneLeft && !m.layout().wide {
		m.focus = paneRight
	}
	return nil
}

// ---------------------------------------------------------------------------
// exit

// accept closes the TUI, printing the query or its output.
func (m *model) accept() tea.Cmd {
	if m.opts.Emit != "output" {
		m.result.Emit = m.query.Value() + "\n"
		return tea.Quit
	}
	if m.last != nil && !m.stale() && !m.running {
		return m.acceptOutput()
	}
	// The output on screen is not the query's: run it, and leave when it
	// is.
	m.exitOnRun = true
	return tea.Batch(m.startRun(), m.flash("running the query before exiting…", false))
}

func (m *model) acceptOutput() tea.Cmd {
	var b strings.Builder
	for _, value := range m.last.Values {
		b.WriteString(value)
		b.WriteByte('\n')
	}
	m.result.Emit = b.String()
	if m.last.Truncated {
		m.result.Warning = fmt.Sprintf("output stopped after %d results (the result limit); raise it with the palette's \"Set the result limit\"", m.last.Count)
	}
	return tea.Quit
}

func (m *model) saveSession() {
	m.store.SaveSession(Session{Query: m.query.Value(), Input: m.input.Value(), Args: parseArgs(m.args.Value())})
}

// ---------------------------------------------------------------------------
// keys

func (m *model) visibleTab(p pane) tab {
	if p == paneLeft {
		return m.leftTab
	}
	return m.rightTab
}

// focusedTab is the tab under the focus, when the focus is not the query.
func (m *model) focusedTab() (tab, bool) {
	if m.focus == paneQuery {
		return 0, false
	}
	return m.visibleTab(m.focus), true
}

func (m *model) selectTab(t tab) {
	if t.onLeft() {
		m.leftTab = t
		m.focus = paneLeft
	} else {
		m.rightTab = t
		m.focus = paneRight
	}
	if t == tabHistory {
		m.refreshHistory()
	}
}

func (m *model) cycleFocus(dir int) {
	m.complete = nil
	m.focus = pane((int(m.focus) + dir + 3) % 3)
}

func (m *model) handleKey(msg tea.KeyMsg) tea.Cmd {
	k := msg.String()

	// Leaving always works, whatever is open.
	if k == "ctrl+c" || k == "ctrl+q" {
		if m.cancel != nil {
			m.cancel()
		}
		m.result = Result{}
		return tea.Quit
	}

	if m.overlay != overlayNone {
		return m.handleOverlayKey(msg)
	}

	if m.complete != nil && m.focus == paneQuery {
		if cmd, used := m.handleCompleteKey(msg); used {
			return cmd
		}
	}

	switch k {
	case "ctrl+r", "f5":
		return m.startRun()
	case "ctrl+x":
		return m.accept()
	case "ctrl+p":
		m.openPalette()
		return nil
	case "f1":
		m.overlay, m.keysTop = overlayKeys, 0
		return nil
	case "tab":
		m.cycleFocus(1)
		return nil
	case "shift+tab":
		m.cycleFocus(-1)
		return nil
	case "esc":
		return m.escape()
	case "ctrl+s":
		return m.promptSaveSnippet()
	case "ctrl+@", "ctrl+ ":
		if m.focus == paneQuery {
			m.openCompletion(true)
		}
		return nil
	case "alt+f":
		return m.rewrite("format")
	case "alt+m":
		return m.rewrite("minify")
	case "alt+i":
		return m.rewrite("inline")
	case "alt+t":
		return m.tidyInput()
	case "alt+c":
		return m.toggle("compact")
	case "alt+r":
		return m.toggle("raw")
	case "alt+s":
		return m.toggle("slurp")
	case "alt+n":
		return m.toggle("null")
	case "alt+l":
		return m.copyShareLink()
	}
	for i, t := range tabOrder {
		if k == "alt+"+strconv.Itoa(i+1) || k == "f"+strconv.Itoa(i+2) {
			m.selectTab(t)
			return nil
		}
	}

	if m.focus == paneQuery {
		return m.editQuery(msg)
	}
	switch m.visibleTab(m.focus) {
	case tabInput:
		if handled, changed := m.input.HandleKey(msg, m.paneHeight()); handled && changed {
			return nil
		}
	case tabArgs:
		if handled, changed := m.args.HandleKey(msg, m.paneHeight()); handled && changed {
			return m.scheduleValidate()
		}
	case tabOutput:
		m.scrollOutput(k)
	case tabDiagram:
		m.diagTop = scroll(k, m.diagTop, len(m.outlineLines), m.paneHeight()-1)
	case tabCatalog:
		return m.catalogKey(msg)
	case tabExamples:
		return m.examplesKey(msg)
	case tabHistory:
		return m.historyKey(msg)
	}
	if k == "?" {
		m.overlay, m.keysTop = overlayKeys, 0
	}
	return nil
}

// escape closes the innermost thing that is open.
func (m *model) escape() tea.Cmd {
	switch {
	case m.complete != nil:
		m.complete = nil
	case m.helpFor != "" && m.focus != paneQuery && m.visibleTab(m.focus) == tabCatalog:
		m.helpFor, m.helpText = "", nil
	case m.focus != paneQuery && m.focusedList() != nil && m.focusedList().filter != "":
		l := m.focusedList()
		l.filter = ""
		l.refilter()
	case m.running:
		return m.cancelRun()
	case m.focus != paneQuery:
		m.focus = paneQuery
	}
	return nil
}

func (m *model) focusedList() *list {
	t, ok := m.focusedTab()
	if !ok {
		return nil
	}
	switch t {
	case tabCatalog:
		return m.catalogList
	case tabExamples:
		return m.examplesList
	case tabHistory:
		return m.historyList
	}
	return nil
}

func (m *model) editQuery(msg tea.KeyMsg) tea.Cmd {
	handled, changed := m.query.HandleKey(msg, m.queryHeight())
	if !handled || !changed {
		if handled {
			m.complete = nil
		}
		return nil
	}
	if msg.Type == tea.KeyRunes || msg.String() == "backspace" {
		m.openCompletion(false)
	} else {
		m.complete = nil
	}
	return m.scheduleValidate()
}

// scroll moves a view's top line by a key, within the lines it has.
func scroll(k string, top, lines, height int) int {
	switch k {
	case "up", "k":
		top--
	case "down", "j":
		top++
	case "pgup":
		top -= max(1, height-1)
	case "pgdown", " ":
		top += max(1, height-1)
	case "home", "g":
		top = 0
	case "end", "G":
		top = lines - height
	}
	return max(0, min(top, lines-height))
}

func (m *model) scrollOutput(k string) {
	height := m.paneHeight() - 1
	m.outTop = scroll(k, m.outTop, len(m.outLines), height)
	switch k {
	case "left", "h":
		m.outLeft = max(0, m.outLeft-8)
	case "right", "l":
		m.outLeft += 8
	case "home", "g":
		m.outLeft = 0
	}
}

// ---------------------------------------------------------------------------
// actions

func (m *model) rewrite(how string) tea.Cmd {
	src := m.query.Value()
	var resp ideengine.FormatResponse
	note := ""
	switch how {
	case "format":
		resp = ideengine.Format(src)
	case "minify":
		resp = ideengine.Minify(src)
	case "inline":
		inlined := ideengine.Inline(src)
		resp = ideengine.FormatResponse{Query: inlined.Query, Error: inlined.Error}
		switch {
		case inlined.Expanded == 0 && len(inlined.Kept) == 0:
			note = "nothing to inline: the query calls none of its own definitions"
		default:
			note = fmt.Sprintf("inlined %d call%s", inlined.Expanded, plural(inlined.Expanded))
			if len(inlined.Kept) > 0 {
				note += "; kept " + strings.Join(inlined.Kept, "; ")
			}
		}
	}
	if resp.Error != "" {
		return m.flash("cannot "+how+": the query does not parse", true)
	}
	if resp.Query == src {
		if note != "" {
			return m.flash(note, false)
		}
		return m.flash("already "+map[string]string{"format": "formatted", "minify": "minified"}[how], false)
	}
	m.query.SetValue(resp.Query)
	m.complete = nil
	if note == "" {
		note = how + "ted - Ctrl-Z undoes it"
		if how == "minify" {
			note = "minified - Ctrl-Z undoes it"
		}
	}
	return tea.Batch(m.scheduleValidate(), m.flash(note, false))
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// tidyInput pretty-prints the input when it is one JSON value, as the page's
// Tidy does.
func (m *model) tidyInput() tea.Cmd {
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		return nil
	}
	tidy, err := prettyJSON(text)
	if err != nil {
		return m.flash("the input is not a single JSON value, so it was left alone", true)
	}
	m.input.SetValue(tidy)
	return m.flash("input tidied - Ctrl-Z in the input undoes it", false)
}

func (m *model) toggle(which string) tea.Cmd {
	var on bool
	var name string
	switch which {
	case "compact":
		m.set.compact = !m.set.compact
		on, name = m.set.compact, "compact output (-c)"
	case "raw":
		m.set.raw = !m.set.raw
		on, name = m.set.raw, "raw output (-r)"
	case "slurp":
		m.set.slurp = !m.set.slurp
		on, name = m.set.slurp, "slurp (-s)"
	case "null":
		m.set.nullInput = !m.set.nullInput
		on, name = m.set.nullInput, "null input (-n)"
	case "rawinput":
		m.set.rawInput = !m.set.rawInput
		on, name = m.set.rawInput, "raw input (-R)"
	}
	m.settingsVersion++
	state := "off"
	if on {
		state = "on"
	}
	return m.flash(name+" "+state+" - Ctrl-R to run", false)
}

// copyToClipboard puts text on the terminal's clipboard with OSC 52, which
// works over SSH and needs nothing installed. The sequence is drawn as part
// of the next frame rather than written beside it, so it can never land in
// the middle of one; it stays in the frame briefly and is then dropped.
func (m *model) copyToClipboard(text, what string) tea.Cmd {
	m.oscID++
	id := m.oscID
	m.osc = "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\x07"
	return tea.Batch(
		tea.Tick(time.Second, func(time.Time) tea.Msg { return oscClearMsg{id} }),
		m.flash(what+" copied to the clipboard (terminals that allow OSC 52)", false),
	)
}

func (m *model) shareLink() (string, error) {
	base := m.opts.ShareBase
	if base == "" {
		base = DefaultShareBase
	}
	return ShareLink(base, Shared{
		Query:   m.query.Value(),
		Input:   m.input.Value(),
		Args:    parseArgs(m.args.Value()),
		Options: m.sharedOptions(),
	})
}

func (m *model) copyShareLink() tea.Cmd {
	link, err := m.shareLink()
	if err != nil {
		return m.flash("cannot make a link: "+err.Error(), true)
	}
	return m.copyToClipboard(link, fmt.Sprintf("share link (%d characters)", len(link)))
}

func (m *model) openShare(link string) tea.Cmd {
	shared, err := DecodeShare(link)
	if err != nil {
		return m.flash(err.Error(), true)
	}
	m.query.SetValue(shared.Query)
	m.input.SetValue(shared.Input)
	m.args.SetValue(formatArgs(shared.Args))
	m.applySharedOptions(shared.Options)
	m.settingsVersion++
	m.focus = paneQuery
	return tea.Batch(m.scheduleValidate(), m.flash("opened the link - Ctrl-R to run it", false))
}

// load puts a query, and optionally an input and arguments, on screen.
func (m *model) load(query, input string, args []ideengine.Arg, what string) tea.Cmd {
	m.query.SetValue(query)
	m.query.SetOffset(len([]rune(query)))
	if input != "" {
		m.input.SetValue(input)
	}
	m.args.SetValue(formatArgs(args))
	m.complete = nil
	m.focus = paneQuery
	note := what + " loaded - Ctrl-R to run it"
	if input != "" && m.originalInput != "" && input != m.originalInput {
		note += "; the palette can restore your input"
	}
	return tea.Batch(m.scheduleValidate(), m.flash(note, false))
}

func (m *model) refreshHistory() {
	var items []item
	if len(m.snippets) > 0 {
		items = append(items, item{title: "Saved", header: true})
		for _, s := range m.snippets {
			items = append(items, item{title: s.Name, detail: oneLine(s.Query), kind: "saved", data: s})
		}
	}
	if len(m.history) > 0 {
		items = append(items, item{title: "Recent", header: true})
		for _, h := range m.history {
			items = append(items, item{title: oneLine(h.Query), detail: ago(h.At), kind: "run", data: h})
		}
	}
	filter := m.historyList.filter
	m.historyList.setItems(items)
	if filter != "" {
		m.historyList.filter = filter
		m.historyList.refilter()
	}
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

func ago(ms int64) string {
	if ms == 0 {
		return ""
	}
	d := time.Since(time.UnixMilli(ms))
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return time.UnixMilli(ms).Format("Jan 2")
}

// suggestName is the default name for a snippet: the query's first line.
func (m *model) suggestName() string {
	first := strings.TrimSpace(firstLine(strings.TrimSpace(m.query.Value())))
	return truncateRunes(first, 40)
}

// writeOut saves text to a path the user typed, relative to the working
// directory.
func writeOut(path, text string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("no file named")
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// prettyJSON indents one JSON value. It works on the text rather than a
// decoded value, so a number keeps exactly the digits it was written with.
func prettyJSON(text string) (string, error) {
	var b bytes.Buffer
	if err := json.Indent(&b, []byte(text), "", "  "); err != nil {
		return "", err
	}
	return b.String(), nil
}
