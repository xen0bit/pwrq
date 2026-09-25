package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/graph"
	"github.com/xen0bit/pwrq/pkg/ideengine"
)

// ---------------------------------------------------------------------------
// catalog

func catalogItems(cat ideengine.CatalogResponse) []item {
	items := make([]item, 0, len(cat.Commands))
	for _, cmd := range cat.Commands {
		items = append(items, item{
			title:  cmd.Name,
			detail: cmd.Description,
			kind:   cmd.Category,
			search: strings.Join(cmd.Aliases, " "),
			data:   cmd,
		})
	}
	return items
}

// arity describes how many arguments a cmdlet takes, as the page's cards do.
func arity(cmd ideengine.Command) string {
	switch {
	case cmd.MaxArgs == 0:
		return "no args"
	case cmd.MinArgs == cmd.MaxArgs:
		return fmt.Sprintf("%d arg%s", cmd.MinArgs, plural(cmd.MinArgs))
	default:
		return fmt.Sprintf("%d–%d args", cmd.MinArgs, cmd.MaxArgs)
	}
}

func (m *model) catalogKey(msg tea.KeyMsg) tea.Cmd {
	k := msg.String()
	if m.helpFor != "" {
		switch k {
		case "q", "backspace", "left":
			m.helpFor, m.helpText = "", nil
		case "enter":
			return m.insertCommand(m.helpFor)
		default:
			m.helpTop = scroll(k, m.helpTop, len(m.helpText), m.paneHeight()-1)
		}
		return nil
	}
	switch k {
	case "enter":
		if it, ok := m.catalogList.selected(); ok {
			return m.insertCommand(it.title)
		}
		return nil
	case "?":
		if it, ok := m.catalogList.selected(); ok {
			return m.fetchHelp(it.title)
		}
		return nil
	}
	m.catalogList.handleKey(msg, m.paneHeight())
	return nil
}

// insertCommand types a cmdlet's name into the query at the cursor, with its
// parentheses when it needs arguments, and puts the cursor between them.
func (m *model) insertCommand(name string) tea.Cmd {
	insert := name
	cmd, known := m.commands[name]
	if known && cmd.MinArgs > 0 {
		insert += "()"
	}
	m.query.InsertText(insert)
	if strings.HasSuffix(insert, "()") {
		m.query.SetOffset(m.query.Offset() - 1)
	}
	m.focus = paneQuery
	m.helpFor, m.helpText = "", nil
	return m.scheduleValidate()
}

// fetchHelp asks get_help for a cmdlet, off the UI goroutine. get_help only
// reads the registry, so this is the one query the TUI runs on its own.
func (m *model) fetchHelp(name string) tea.Cmd {
	m.helpFor = name
	m.helpText = []string{"loading…"}
	m.helpTop = 0
	quoted, _ := json.Marshal(name)
	req := ideengine.RunRequest{
		Query: "get_help($name)", NullInput: true, Raw: true,
		Args: []ideengine.Arg{{Name: "name", Value: string(quoted)}},
	}
	r := m.run
	return func() tea.Msg {
		resp, _ := r.run(context.Background(), req)
		text := strings.Join(resp.Values, "\n")
		if resp.Error != "" {
			text = "get_help failed: " + resp.Error
		}
		return helpMsg{name: name, text: text}
	}
}

// ---------------------------------------------------------------------------
// examples

func exampleItems(examples []ideengine.Example) []item {
	var items []item
	category := ""
	for _, ex := range examples {
		if ex.Category != category {
			category = ex.Category
			items = append(items, item{title: category, header: true})
		}
		items = append(items, item{title: ex.Title, detail: ex.Description, kind: ex.Category, search: ex.Query, data: ex})
	}
	return items
}

func (m *model) examplesKey(msg tea.KeyMsg) tea.Cmd {
	if msg.String() == "enter" {
		if it, ok := m.examplesList.selected(); ok {
			ex := it.data.(ideengine.Example)
			return m.load(ex.Query, ex.Input, ex.Args, "example")
		}
		return nil
	}
	m.examplesList.handleKey(msg, m.paneHeight())
	return nil
}

// ---------------------------------------------------------------------------
// history

func (m *model) historyKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		if it, ok := m.historyList.selected(); ok {
			entry := it.data.(Entry)
			what := "run"
			if it.kind == "saved" {
				what = "snippet " + entry.Name
			}
			return m.load(entry.Query, entry.Input, entry.Args, what)
		}
		return nil
	case "delete", "ctrl+d":
		if it, ok := m.historyList.selected(); ok && it.kind == "saved" {
			entry := it.data.(Entry)
			m.snippets = m.store.DeleteSnippet(entry.Name)
			m.refreshHistory()
			return m.flash("deleted "+entry.Name, false)
		}
		return nil
	}
	m.historyList.handleKey(msg, m.paneHeight())
	return nil
}

// ---------------------------------------------------------------------------
// diagram

// outlineLine is one row of the diagram tab: the tree's branches, then a
// node coloured by its class.
type outlineLine struct {
	branch string
	label  string
	class  string
}

// refreshOutline redraws the diagram when the query has changed since it was
// last drawn. It is the same diagram the page draws, as a tree.
func (m *model) refreshOutline() {
	if m.outlineVersion == m.query.Version && (m.outlineLines != nil || m.outlineErr != "") {
		return
	}
	m.outlineVersion = m.query.Version
	m.outlineLines, m.outlineErr = nil, ""
	query, err := gojq.Parse(m.query.Value())
	if err != nil {
		m.outlineErr = err.Error()
		return
	}
	nodes := graph.Outline(query, graph.RenderOptions{Cmdlets: m.vocab.Cmdlets})
	m.outlineLines = flattenOutline(nodes, "")
	m.diagTop = min(m.diagTop, max(0, len(m.outlineLines)-1))
}

func flattenOutline(nodes []*graph.OutlineNode, prefix string) []outlineLine {
	var out []outlineLine
	for i, node := range nodes {
		last := i == len(nodes)-1
		branch, next := "├─ ", "│  "
		if last {
			branch, next = "└─ ", "   "
		}
		out = append(out, outlineLine{branch: prefix + branch, label: node.Label, class: node.Class})
		out = append(out, flattenOutline(node.Children, prefix+next)...)
	}
	return out
}
