# A terminal UI for pwrq

## Context

pwrq had two editors, both the same browser page: `--ide` evaluates in a WASM
worker against the pure in-memory cmdlets, and `--ide-native` serves the page
from `pwrq-viz-native`, evaluating on the machine. Nearly everything the page
does is the engine behind it — seven methods: validate, run, diagram, format,
minify, inline, catalog. A terminal UI is a third client of those methods,
and adds what a tab cannot: data on stdin, files as arguments, and a query or
its output printed on exit, so it composes with the next command.

## Decisions

- **It ships in plain `pwrq`**, not `pwrq-viz`. It costs about 1.5MB.
- **It validates as you type and never runs as you type.** Validation
  compiles against the full vocabulary; running is always Ctrl-R. A test
  types a file-writing query a character at a time, formats, minifies,
  inlines, undoes and redoes it, and checks the file never appears.
- **It shares the engine rather than copying it.**

## Status: all four phases are built

### Phase 0: one engine

- `pkg/ideengine` is the engine, typed. `webapi` and `webnative` are
  configurations of it, and their wire types are aliases, so the page is
  unchanged.
- `pkg/graph` no longer links d2; SVG rendering is `pkg/graph/graphsvg`.
- `queryrun` reports a cancelled run as `cancelled`, not a runtime error.
- Validation's `formatted` no longer includes the alias definitions compile
  prepends (a bug the native page had).

### Phase 1: the minimum

- `pwrq --tui [query] [files…]` in `cli/tui.go`; the UI is `pkg/tui`, on
  Bubble Tea v1 and Lip Gloss.
- Input from the files named or from piped stdin, read with the CLI's own
  readers: `-R`, `-s`, `-n`, `--yaml-input`, `--stream`, `--arg`,
  `--argjson`, `--slurpfile`, `--rawfile`, `--args`, `--jsonargs`, `-L`.
- Keys come from the terminal (`/dev/tty`, `CONIN$`) and the screen is drawn
  on it, so stdin and stdout stay free for data.
- Runs have generations: a new run cancels the old, whose result is dropped.
  `debug` and `stderr` are captured into the output pane.
- Ctrl-X prints the query, or with `--emit=output` the output (running the
  query first if the output on screen is stale). Ctrl-C prints nothing.

### Phase 2: an editor

- The page's tokeniser, ported to Go (`pkg/tui/tokenize.go`), pinned to the
  cases in `highlight.test.js`.
- The parse error underlined at its span; compile errors in the status line.
- Completion from the catalog: automatic after two letters, Ctrl-Space on
  demand, Tab to accept, never inside strings or comments.
- Format, Minify, Inline (saying what it kept), all undoable.
- The Args pane: `name = JSON`, one per line.

### Phase 3: browsing

- Tabs: Output, Diagram, Catalog (Enter inserts, `?` shows `get_help`),
  Examples (the page's gallery), History (runs and snippets), Input, Args.
- A palette on Ctrl-P over every action, tab, example, snippet and cmdlet.
- History, snippets and the last session in `$XDG_STATE_HOME/pwrq`; snippets
  in the page's export format, importable and exportable either way.

### Phase 4: interop

- Share links in the page's `#z=` format, checked against `share.js` itself
  in both directions (the test runs bun when it is installed). Alt-L copies
  one by OSC 52; `pwrq --tui <link>` and the palette open one.
  `PWRQ_SHARE_URL` names the page.
- The diagram as a tree, recorded by the same builder that writes the D2
  script (`graph.Outline`), in the diagram's palette. The D2 source can be
  copied or saved; `pwrq-viz` can also save the SVG.

## Keys

Terminals do not deliver Ctrl-Enter or Ctrl-Shift-letter, so the page's keys
could not be copied. F1 lists these in the UI, from the same table the
palette is built from.

| Key | Does |
|---|---|
| Ctrl-R, F5 | run |
| Esc | close what is open, clear a filter, or cancel the run |
| Ctrl-X | accept: print the query (or output) and exit |
| Ctrl-C, Ctrl-Q | quit, printing nothing |
| Tab / Shift-Tab | next / previous pane |
| Alt-1…7, F2…F8 | Output, Diagram, Catalog, Examples, History, Input, Args |
| Ctrl-P | palette |
| F1, ? | keys |
| Alt-F / Alt-M / Alt-I | format / minify / inline |
| Alt-T | tidy the input JSON |
| Alt-C / Alt-R / Alt-S / Alt-N | toggle -c / -r / -s / -n |
| Alt-L | copy a share link |
| Ctrl-S | save the query as a snippet |
| Ctrl-Space | complete; Tab accepts |
| Ctrl-Z / Ctrl-Y | undo / redo |
| Ctrl-/ | comment the line |

Where this differs from the plan: the palette is Ctrl-P and help is F1, so
the tabs moved to F2…F8; completion accepts on Tab only, so Enter is always a
newline; the default timeout is 60s rather than the page's 5s, since Esc
works here.

## Testing

- `pkg/tui/model_test.go` drives the model with real messages and runs its
  commands concurrently, as the runtime does: running, cancelling, accepting,
  completion, the catalog and `get_help`, examples, snippets, share links,
  arguments, debug capture.
- Golden frames at 100×30 and 64×22 (`go test ./pkg/tui -update` rewrites
  them), and a check that every frame, overlays included, is exactly the
  terminal's size at four sizes.
- The binary was driven in a real pty: piped stdin with stdout piped and
  with stdout a terminal, a file argument, a share link, and `--emit`.

## Known limits

- A line over 4KB is drawn uncoloured, from the part on screen: colouring
  needs the whole line, and a minified document is one line of megabytes.
- The input is read into memory whole; the CLI streams, the TUI cannot.
- No mouse and no selection: the terminal's own selection keeps working.
- OSC 52 copying depends on the terminal allowing it (tmux needs
  `set-clipboard on`).

## Not in scope

- `pkg/mcpserver` still has an engine of its own; moving it onto
  `ideengine` is worth doing separately.
- The native web page still runs as you type. The TUI's rule may be right
  for it too.
- The Windows build of `pwrq` already fails on `main`, in
  `pkg/udf/http/reuseaddr.go`, before reaching any of this.
