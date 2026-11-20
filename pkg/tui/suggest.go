package tui

import (
	"context"
	"fmt"
	"math"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/xen0bit/pwrq/pkg/ideengine"
)

// Suggestions in the query editor: after a pause in typing, or on Ctrl-Space,
// the engine asks the decision model what comes next, and a confident answer
// is drawn after the cursor as ghost text that Tab accepts. Each request is
// for the text as it was; an edit drops it, and the engine cancels the
// model's pass for it.

// suggestDelay is the pause in typing that asks for a suggestion: long
// enough not to ask at every keystroke, short enough to feel like it was
// waiting.
const suggestDelay = 600 * time.Millisecond

// spinnerFrames draw the model's progress in the status line.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type suggestTickMsg struct{ gen int }

type suggestMsg struct {
	gen  int
	of   int // the query's version it was for
	resp ideengine.SuggestResponse
}

type spinMsg struct{}

// suggestState is the model's view of suggestions.
type suggestState struct {
	enabled  bool
	gen      int
	busy     bool
	started  time.Time
	cancel   context.CancelFunc
	frame    int
	spinning bool
	status   ideengine.SuggestStatus
	top      *ideengine.Suggestion
}

// initSuggest asks the engine's ranker for its state, which starts it
// loading in the background.
func (m *model) initSuggest() tea.Cmd {
	eng := m.run.eng
	st := eng.SuggestStatus()
	m.sugg.status = st
	m.sugg.enabled = st.State != "unavailable"
	if !m.sugg.enabled {
		return nil
	}
	return m.spin()
}

// scheduleSuggest asks for a suggestion after a pause; explicit asks now.
func (m *model) scheduleSuggest(explicit bool) tea.Cmd {
	if !m.sugg.enabled {
		return nil
	}
	m.sugg.gen++
	m.query.Ghost, m.sugg.top = "", nil
	if m.sugg.cancel != nil {
		m.sugg.cancel()
		m.sugg.cancel = nil
	}
	gen := m.sugg.gen
	if explicit {
		return m.suggest(gen, true)
	}
	return tea.Tick(suggestDelay, func(time.Time) tea.Msg { return suggestTickMsg{gen} })
}

// suggest runs a suggestion off the UI goroutine.
func (m *model) suggest(gen int, explicit bool) tea.Cmd {
	row, col := m.query.Cursor()
	cursor := 0
	for i := 0; i < row; i++ {
		cursor += runeLen(m.query.Line(i)) + 1
	}
	cursor += col
	req := ideengine.SuggestRequest{Query: m.query.Value(), Cursor: cursor, Input: m.input.Value(), Explicit: explicit}
	of := m.query.Version
	ctx, cancel := context.WithCancel(context.Background())
	m.sugg.cancel = cancel
	m.sugg.busy, m.sugg.started = true, time.Now()
	eng := m.run.eng
	return tea.Batch(
		func() tea.Msg { return suggestMsg{gen: gen, of: of, resp: eng.Suggest(ctx, req)} },
		m.spin(),
	)
}

func (m *model) finishSuggest(msg suggestMsg) tea.Cmd {
	if msg.gen != m.sugg.gen {
		return nil
	}
	m.sugg.busy, m.sugg.cancel = false, nil
	m.sugg.status = msg.resp.Status
	if msg.of != m.query.Version || m.focus != paneQuery {
		return nil
	}
	m.query.Ghost = msg.resp.Ghost
	m.sugg.top = nil
	if len(msg.resp.Items) > 0 && msg.resp.Ranked {
		top := msg.resp.Items[0]
		m.sugg.top = &top
	}
	return nil
}

// spin keeps the spinner turning while the model loads or thinks.
func (m *model) spin() tea.Cmd {
	if m.sugg.spinning {
		return nil
	}
	m.sugg.spinning = true
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return spinMsg{} })
}

func (m *model) onSpin() tea.Cmd {
	m.sugg.spinning = false
	m.sugg.frame = (m.sugg.frame + 1) % len(spinnerFrames)
	eng := m.run.eng
	st := eng.SuggestStatus()
	m.sugg.status = st
	if m.sugg.busy || st.State == "loading" || st.State == "fetching" || st.State == "idle" {
		return m.spin()
	}
	return nil
}

// acceptGhost inserts the suggestion, and asks for the next one.
func (m *model) acceptGhost() tea.Cmd {
	if !m.query.AcceptGhost() {
		return nil
	}
	m.sugg.top = nil
	return tea.Batch(m.scheduleValidate(), m.scheduleSuggest(false))
}

// suggestIndicator is the status line's word on suggestions.
func (m *model) suggestIndicator() string {
	if !m.sugg.enabled {
		return ""
	}
	th := m.th
	spinner := spinnerFrames[m.sugg.frame]
	st := m.sugg.status
	switch {
	case st.State == "fetching":
		if st.Fraction >= 0 {
			return th.accent.Render(fmt.Sprintf("%s d1 %s %d%%", spinner, st.Detail, int(math.Round(100*st.Fraction))))
		}
		return th.accent.Render(spinner + " d1 " + st.Detail)
	case st.State == "loading" || st.State == "idle":
		return th.accent.Render(spinner + " d1 loading")
	case m.sugg.busy:
		return th.accent.Render(spinner + " d1 thinking")
	case st.State == "error":
		return th.danger.Render("d1: " + st.Detail)
	case m.query.Ghost != "" && m.sugg.top != nil:
		return th.muted.Render(fmt.Sprintf("Tab ⇥ %s %d%%", m.sugg.top.Display, int(math.Round(100*m.sugg.top.Probability))))
	}
	return th.faint.Render("d1 " + st.Device)
}
