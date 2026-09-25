package tui

import (
	"errors"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/itchyny/gojq"
	"github.com/muesli/termenv"
	"github.com/xen0bit/pwrq/pkg/graph"
	"github.com/xen0bit/pwrq/pkg/ideengine"
)

// Options are what the command line hands the TUI: the query and input it
// starts with, jq's flags, and where it keeps and sends things.
type Options struct {
	// Query is the query to start with. Empty means the last session's, or
	// an example when there is none and RestoreSession is set, or ".".
	Query string
	// Share is a share link to open instead of Query and Input.
	Share string
	// RestoreSession starts from the last session when no query was given.
	// The command line sets it only when nothing was given at all: a query,
	// a file or piped data is what the user asked to look at.
	RestoreSession bool

	// Input is the text queries run against, and InputLabel says where it
	// came from, for the header: "stdin", "data.json", "3 files".
	Input      string
	InputLabel string
	// InputName is what input_filename reports.
	InputName string

	// jq's flags, as the command line parsed them.
	RawInput, Slurp, NullInput bool
	Raw, Compact               bool
	Indent                     int
	Tab                        bool
	Args                       []ideengine.Arg
	Positional                 []any

	// RunOptions are compiler options every run adds: the module loader for
	// -L, the environment. The TUI adds its own debug and stderr.
	RunOptions []gojq.CompilerOption

	// Emit is what Ctrl-X prints to stdout on the way out: "query" (the
	// default) or "output".
	Emit string

	// StateDir holds history, snippets and the last session; empty keeps
	// nothing.
	StateDir string
	// ShareBase is the page a share link opens.
	ShareBase string

	// RenderSVG draws a diagram as an image. Only a build with d2 has one;
	// without it the diagram can be saved as its D2 script.
	RenderSVG func(query *gojq.Query, opts graph.RenderOptions) (string, error)

	Version string

	// In and Out are the terminal. They are not stdin and stdout when those
	// carry data: the input is piped in and the result may be piped out.
	In  io.Reader
	Out io.Writer
}

// Result is what the TUI hands back when it closes.
type Result struct {
	// Emit is what to print to stdout, or empty for nothing.
	Emit string
	// Warning is said on stderr: output that was cut short, say.
	Warning string
}

// Run opens the TUI and blocks until it is closed.
func Run(opts Options) (Result, error) {
	if opts.In == nil || opts.Out == nil {
		return Result{}, errors.New("the TUI needs a terminal")
	}

	r := lipgloss.NewRenderer(opts.Out)
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		r.SetColorProfile(termenv.Ascii)
	}
	// Asked now, before the program owns the terminal's input: the answer
	// arrives on the same stream as the keys.
	r.SetHasDarkBackground(r.HasDarkBackground())

	m, err := newModel(opts, newTheme(r))
	if err != nil {
		return Result{}, err
	}

	program := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithInput(opts.In),
		tea.WithOutput(opts.Out),
	)
	final, err := program.Run()
	if err != nil {
		return Result{}, err
	}
	fm := final.(*model)
	fm.saveSession()
	return fm.result, nil
}
