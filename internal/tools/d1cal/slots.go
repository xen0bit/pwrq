package main

// The slots study chooses each kind's question: the wording that most often
// reads, from a dev comment, the value its label states - and reads nothing
// when the comment states none. It runs before the scripts study, which then
// scores whole queries with the questions chosen here.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xen0bit/pwrq/pkg/d1"
	"github.com/xen0bit/pwrq/pkg/d1/prompt"
	"github.com/xen0bit/pwrq/pkg/ideengine"
)

var kindPhrasings = map[string][]string{
	"number": {"How many?", "How many? (a count or limit)", "What count or limit?", "What number is given?", "How many items, lines or results?"},
	"days":   {"How many days?", "Over how many days?", "What time span, in days?", "How many days back?"},
	"size":   {"What size?", "What size limit?", "What file size, with its unit?", "How big?"},
	"path":   {"Which directory or folder?", "Which directory?", "In which folder or path?", "What directory path is named?", "Which folder or path, if any?"},
	"file":   {"Which file?", "Which file name?", "What file is named?", "Which file (name or path)?"},
	"host":   {"Which host, domain or URL?", "Which host name?", "What server or domain?", "Which domain or IP?"},
	"url":    {"Which URL or website?", "Which URL?", "What web address or site?", "Which website or link?"},
	"ip":     {"Which IP address?", "What IP address?", "Which address?"},
	"ext":    {"Which kind of file or extension?", "Which programming language or file extension?", "What file type?", "Which file extension or language?", "What kind of files?"},
	"input":  {"What text or value should it work on?", "What text does the comment give to work on?", "What value is quoted or named?", "Which string or value?"},
	"value":  {"What number should it work on?", "Which number?", "What number is given?"},
	"cidr":   {"Which CIDR range or subnet?", "Which network range?", "What subnet?"},
	"field":  {"Which field or property?", "Which field name?", "What is the name of the field it is about?", "Which property of the items is it about?", "Which attribute or column?", "Which key?"},
}

type slotCase struct {
	id, comment, kind, want string // want "" for a comment that states none
	def                     string
}

func runSlots(ctx context.Context, model *d1.Model, out string) error {
	samples, err := allSamples(out)
	if err != nil {
		return err
	}
	rs := ideengine.AllRecipes()
	byID := map[string]ideengine.Recipe{}
	for _, r := range rs {
		byID[r.ID] = r
	}
	var cases []slotCase
	for _, s := range samples {
		if s.Split != "dev" {
			continue
		}
		for _, sl := range byID[s.Recipe].Slots {
			if _, generic := kindPhrasings[sl.Kind]; !generic {
				continue
			}
			want := ""
			if v, ok := s.Slots[sl.Name]; ok {
				want, _ = ideengine.NormalizeSlot(sl.Kind, v)
			}
			def, _ := ideengine.NormalizeSlot(sl.Kind, sl.Default)
			cases = append(cases, slotCase{s.ID, s.Comment, sl.Kind, want, def})
		}
	}
	score := map[string]map[string]int{}
	total := map[string]int{}
	for _, c := range cases {
		raw := prompt.Object{}
		for i, q := range kindPhrasings[c.kind] {
			raw = append(raw, prompt.Member{Key: fmt.Sprintf("q%d", i), Value: prompt.Object{{Key: "type", Value: "text"}, {Key: "instructions", Value: q}}})
		}
		qs, err := prompt.ParseQuestions(raw)
		if err != nil {
			return err
		}
		resp, err := model.Ask(ctx, d1.Request{State: c.comment, Questions: qs})
		if err != nil {
			return err
		}
		total[c.kind]++
		if score[c.kind] == nil {
			score[c.kind] = map[string]int{}
		}
		for i, q := range kindPhrasings[c.kind] {
			ans, _ := resp.Answers[fmt.Sprintf("q%d", i)].(map[string]any)
			text, _ := ans["text"].(string)
			got, ok := ideengine.NormalizeSlot(c.kind, text)
			right := ok && got == c.want
			if c.want == "" {
				right = !ok || got == c.def
			}
			if right {
				score[c.kind][q]++
			}
		}
	}
	chosen := ideengine.KindQuestions()
	var j strings.Builder
	fmt.Fprintf(&j, "# Journal: the slot questions\n\nEach kind's phrasings, asked of every dev comment whose recipe has a slot of that kind: right when the value read is the labelled one, or nothing (or the default) is read when the comment states none.\n\n| kind | phrasing | right | of |\n|---|---|---|---|\n")
	var kinds []string
	for k := range kindPhrasings {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		best, bestN := chosen[k], score[k][chosen[k]]
		for _, q := range kindPhrasings[k] {
			fmt.Fprintf(&j, "| %s | %s | %d | %d |\n", k, q, score[k][q], total[k])
			if score[k][q] > bestN {
				best, bestN = q, score[k][q]
			}
		}
		chosen[k] = best
	}
	fmt.Fprintf(&j, "\nChosen:\n\n")
	for _, k := range kinds {
		fmt.Fprintf(&j, "- %s: %q (%d of %d)\n", k, chosen[k], score[k][chosen[k]], total[k])
	}
	if err := os.WriteFile(filepath.Join(out, "SLOTS.md"), []byte(j.String()), 0o644); err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(chosen, "", "  ")
	fmt.Fprint(os.Stderr, j.String())
	return os.WriteFile(filepath.Join("pkg", "ideengine", "slots.json"), append(raw, '\n'), 0o644)
}
