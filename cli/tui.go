package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/mattn/go-isatty"

	"github.com/xen0bit/pwrq/pkg/ideengine"
	"github.com/xen0bit/pwrq/pkg/tui"
)

// ShareURLEnv names the page a share link made in the TUI opens. It defaults
// to the IDE as `pwrq-viz --ide` serves it on this machine.
const ShareURLEnv = "PWRQ_SHARE_URL"

// launchTUI opens the terminal editor over the query, files and flags the
// command line was given, the way pwrq would have run them.
//
// The data comes from the files named, or from stdin when it is piped; the
// keys come from the terminal either way. On Ctrl-X the query, or its output
// under --emit=output, is printed to stdout, so the TUI composes:
//
//	q=$(pwrq --tui)                     # build a query, keep it
//	kubectl get pods -o json | pwrq --tui --emit=output | less
func (cli *cli) launchTUI(opts *flagopts, args []string) error {
	switch opts.Emit {
	case "", "query", "output":
	default:
		return fmt.Errorf("--emit is query or output, not %q", opts.Emit)
	}

	t := tui.Options{
		Emit:       opts.Emit,
		RawInput:   opts.InputRaw,
		Slurp:      opts.InputSlurp,
		NullInput:  opts.InputNull,
		Raw:        opts.OutputRaw || opts.OutputRaw0 || opts.OutputJoin,
		Compact:    opts.OutputCompact,
		Tab:        opts.OutputTab,
		Version:    fmt.Sprintf("%s (rev: %s/%s)", version, revision, runtime.Version()),
		StateDir:   tui.DefaultStateDir(),
		ShareBase:  os.Getenv(ShareURLEnv),
		RenderSVG:  renderSVG,
		Positional: positionalArgs(opts),
	}
	if opts.OutputIndent != nil {
		t.Indent = *opts.OutputIndent
	}

	// The query: from -f's file, the first argument, or a share link in its
	// place.
	fname := ""
	switch {
	case opts.FromFile:
		if len(args) == 0 {
			return errors.New("expected a query file for flag `-f'")
		}
		src, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		t.Query, fname, args = string(src), args[0], args[1:]
	case len(args) > 0 && tui.LooksLikeShare(args[0]):
		t.Share, args = args[0], args[1:]
	case len(args) > 0:
		t.Query, args = args[0], args[1:]
	}

	// The data.
	stdinIsTerminal := isTerminal(os.Stdin)
	switch {
	case len(args) > 0:
		var parts []string
		for _, name := range args {
			data, err := os.ReadFile(name)
			if err != nil {
				return err
			}
			parts = append(parts, string(data))
		}
		t.Input = strings.Join(parts, "\n")
		t.InputLabel = args[0]
		if len(args) == 1 {
			t.InputName = args[0]
		} else {
			t.InputLabel = fmt.Sprintf("%d files", len(args))
		}
	case !stdinIsTerminal:
		data, err := io.ReadAll(cli.inStream)
		if err != nil {
			return err
		}
		t.Input = string(data)
		t.InputLabel = "stdin"
	}
	if t.Input != "" {
		t.InputLabel += ", " + humanBytes(len(t.Input))
	}
	// A bare `pwrq --tui` picks up where the last one left off.
	t.RestoreSession = t.Query == "" && t.Share == "" && len(args) == 0 && stdinIsTerminal

	// The TUI's engine reads JSON. YAML and --stream are what this command
	// line reads them as, re-rendered as the JSON values they decode to.
	if t.Input != "" && (opts.InputYAML || opts.InputStream) && !opts.InputRaw {
		newIter := newYAMLInputIter
		if opts.InputStream {
			newIter = newStreamInputIter
		}
		converted, err := toJSONLines(newIter(strings.NewReader(t.Input), t.InputLabel))
		if err != nil {
			return err
		}
		t.Input = converted
	}

	named, err := namedArgs(opts)
	if err != nil {
		return err
	}
	t.Args = named

	modulePaths := opts.ModulePaths
	if len(modulePaths) == 0 && addDefaultModulePaths {
		modulePaths = []string{"~/.jq", "$ORIGIN/../lib/pwrq", "$ORIGIN/../lib"}
	}
	if fname != "" {
		modulePaths = append([]string{filepath.Dir(fname)}, modulePaths...)
	}
	t.RunOptions = []gojq.CompilerOption{
		gojq.WithModuleLoader(gojq.NewModuleLoader(modulePaths)),
		gojq.WithEnvironLoader(os.Environ),
	}

	in, out, closeTerminal, err := openTerminal(stdinIsTerminal)
	if err != nil {
		return err
	}
	defer closeTerminal()
	t.In, t.Out = in, out

	res, err := tui.Run(t)
	if err != nil {
		return err
	}
	if res.Warning != "" {
		_, _ = fmt.Fprintf(cli.errStream, "%s: %s\n", name, res.Warning)
	}
	if res.Emit != "" {
		_, err = io.WriteString(cli.outStream, res.Emit)
	}
	return err
}

func isTerminal(f *os.File) bool {
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// openTerminal finds the terminal to draw on and read keys from. Neither is
// stdin or stdout when those carry data, which is the case the TUI is most
// useful in.
func openTerminal(stdinIsTerminal bool) (io.Reader, io.Writer, func(), error) {
	inName, outName := "/dev/tty", "/dev/tty"
	if runtime.GOOS == "windows" {
		inName, outName = "CONIN$", "CONOUT$"
	}
	var closers []io.Closer
	closeAll := func() {
		for _, c := range closers {
			_ = c.Close()
		}
	}

	var in io.Reader = os.Stdin
	if !stdinIsTerminal {
		f, err := os.Open(inName)
		if err != nil {
			return nil, nil, nil, errors.New("--tui needs a terminal to read keys from, and stdin is not one")
		}
		closers = append(closers, f)
		in = f
	}

	var out io.Writer
	switch {
	case isTerminal(os.Stdout):
		out = os.Stdout
	default:
		// Read-write, not write-only: asking the terminal its background
		// colour reads the answer back from here. Were this write-only, the
		// answer would arrive with the keys instead, typed into the query.
		if f, err := os.OpenFile(outName, os.O_RDWR, 0); err == nil {
			closers = append(closers, f)
			out = f
		} else if isTerminal(os.Stderr) {
			out = os.Stderr
		} else {
			closeAll()
			return nil, nil, nil, errors.New("--tui needs a terminal to draw on")
		}
	}
	return in, out, closeAll, nil
}

// namedArgs turns --arg, --argjson, --slurpfile and --rawfile into the JSON
// the TUI's Args pane holds.
func namedArgs(opts *flagopts) ([]ideengine.Arg, error) {
	var args []ideengine.Arg
	add := func(name string, v any) error {
		b, err := gojq.Marshal(v)
		if err != nil {
			return err
		}
		args = append(args, ideengine.Arg{Name: name, Value: string(b)})
		return nil
	}
	for k, v := range opts.Arg {
		if err := add(k, v); err != nil {
			return nil, err
		}
	}
	for k, v := range opts.ArgJSON {
		val, _ := newJSONInputIter(strings.NewReader(v), "$"+k).Next()
		if err, ok := val.(error); ok {
			return nil, err
		}
		if err := add(k, val); err != nil {
			return nil, err
		}
	}
	for k, v := range opts.SlurpFile {
		val, err := slurpFile(v)
		if err != nil {
			return nil, err
		}
		if err := add(k, val); err != nil {
			return nil, err
		}
	}
	for k, v := range opts.RawFile {
		val, err := os.ReadFile(v)
		if err != nil {
			return nil, err
		}
		if err := add(k, string(val)); err != nil {
			return nil, err
		}
	}
	// Flags arrive as maps; a stable order keeps the pane from reshuffling.
	sort.Slice(args, func(i, j int) bool { return args[i].Name < args[j].Name })
	return args, nil
}

// positionalArgs is $ARGS.positional, from --args and --jsonargs.
func positionalArgs(opts *flagopts) []any {
	positional := append([]any{}, opts.Args...)
	for i, v := range opts.JSONArgs {
		if v == nil {
			continue
		}
		val, _ := newJSONInputIter(strings.NewReader(v.(string)), "--jsonargs").Next()
		if _, isErr := val.(error); isErr {
			continue
		}
		if i < len(positional) {
			positional[i] = val
		} else {
			positional = append(positional, val)
		}
	}
	return positional
}

// toJSONLines drains an input iterator into one JSON value per line.
func toJSONLines(iter inputIter) (string, error) {
	defer func() { _ = iter.Close() }()
	var b strings.Builder
	for {
		v, ok := iter.Next()
		if !ok {
			return b.String(), nil
		}
		if err, isErr := v.(error); isErr {
			return "", err
		}
		text, err := gojq.Marshal(v)
		if err != nil {
			return "", err
		}
		b.Write(text)
		b.WriteByte('\n')
	}
}

func humanBytes(n int) string {
	switch {
	case n < 1<<10:
		return fmt.Sprintf("%d B", n)
	case n < 1<<20:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}
