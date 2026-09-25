package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/xen0bit/pwrq/pkg/ideengine"
)

// completion is one name the query editor can offer, built as the page
// builds its list: from the catalog the engine reports, so it never offers a
// name it cannot run.
type completion struct {
	name    string
	kind    string
	detail  string
	insert  string
	minArgs int
	maxArgs int
}

func buildCompletions(cat ideengine.CatalogResponse) []completion {
	byName := map[string]ideengine.Command{}
	for _, cmd := range cat.Commands {
		byName[cmd.Name] = cmd
	}
	var items []completion
	for _, name := range cat.Cmdlets {
		cmd, known := byName[name]
		c := completion{name: name, kind: "cmdlet", detail: cmd.Description, insert: name}
		if known {
			c.minArgs, c.maxArgs = cmd.MinArgs, cmd.MaxArgs
			if cmd.MinArgs > 0 {
				c.insert = name + "()"
			}
		}
		items = append(items, c)
	}
	for _, alias := range cat.Aliases {
		items = append(items, completion{name: alias.Name, kind: "alias", detail: "→ " + alias.Target, insert: alias.Name})
	}
	for _, name := range cat.Builtins {
		if _, isCmdlet := byName[name]; isCmdlet {
			continue
		}
		items = append(items, completion{name: name, kind: "jq", insert: name})
	}
	for _, word := range []string{"def", "as", "if", "then", "elif", "else", "end", "reduce", "foreach", "try", "catch", "label"} {
		items = append(items, completion{name: word, kind: "keyword", insert: word})
	}
	return items
}

// completeState is the open completion list: the word it completes and what
// matches it.
type completeState struct {
	word    Word
	matches []completion
	sel     int
	top     int
}

// maxCompletions is as many as are worth reading.
const maxCompletions = 60

// openCompletion offers the names that complete the word at the cursor. It
// opens by itself once two letters are typed, and on Ctrl-Space with any;
// never inside a string or a comment, where a function name would be wrong.
func (m *model) openCompletion(explicit bool) {
	m.complete = nil
	src := []rune(m.query.Value())
	off := m.query.Offset()
	if InLiteral(TokenizeQuery(src, m.vocab), off) {
		return
	}
	word := WordAt(src, off)
	if word.Prefix != 0 {
		return
	}
	if !explicit && len([]rune(word.Text)) < 2 {
		return
	}
	// A name right after a dot is a field, not a call.
	if word.Start > 0 && src[word.Start-1] == '.' {
		return
	}

	var prefixed, containing []completion
	lower := strings.ToLower(word.Text)
	for _, c := range m.completions {
		name := strings.ToLower(c.name)
		switch {
		case strings.HasPrefix(name, lower):
			prefixed = append(prefixed, c)
		case explicit && lower != "" && strings.Contains(name, lower):
			containing = append(containing, c)
		}
	}
	matches := append(prefixed, containing...)
	if len(matches) > maxCompletions {
		matches = matches[:maxCompletions]
	}
	if len(matches) == 0 {
		return
	}
	if !explicit && len(matches) == 1 && matches[0].name == word.Text {
		return
	}
	m.complete = &completeState{word: word, matches: matches}
}

// handleCompleteKey takes the keys an open list uses. Anything else goes on
// to the editor, and typing refreshes the list; Enter is still a newline.
func (m *model) handleCompleteKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	c := m.complete
	switch msg.String() {
	case "up", "ctrl+p":
		c.sel = (c.sel - 1 + len(c.matches)) % len(c.matches)
	case "down", "ctrl+n":
		c.sel = (c.sel + 1) % len(c.matches)
	case "tab":
		return m.acceptCompletion(), true
	case "esc":
		m.complete = nil
	default:
		return nil, false
	}
	return nil, true
}

func (m *model) acceptCompletion() tea.Cmd {
	c := m.complete
	m.complete = nil
	chosen := c.matches[c.sel]
	m.query.Replace(c.word.Start, c.word.End, chosen.insert)
	if strings.HasSuffix(chosen.insert, "()") {
		m.query.SetOffset(m.query.Offset() - 1)
	}
	return m.scheduleValidate()
}
