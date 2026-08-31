// Package pwrgrep is pwrq's structural rules: the corpus it ships with, where
// to find one, and how to run it.
//
// A rule is a pwrq query and nothing else. select_ast answers one question -
// where does this piece of syntax occur - and that is rarely a finding on its
// own: "MD5, but only in a file that imports crypto/md5", "assigning to
// innerHTML, but not a string literal". The operators that combine those
// answers are cmdlets, so a rule is a file and a header:
//
//	# rules: go-weak-hash
//	# from: go/lang/security/audit/crypto/use_of_weak_crypto.yaml
//
//	["md5.New()", "md5.Sum($$$A)"] as $calls
//	| ["\"crypto/md5\""] as $imports
//	| scan_ast("*.go"; $calls + $imports) as $all
//	| ($all | of($calls) | in_files_with($all | of($imports)))
//	| finding("go-weak-hash"; "this hash is not collision resistant")
//	| report
//
// Which is the point of keeping them as queries. A rule is readable, a rule is
// editable, and writing a new one is copying a file - not learning a schema
// and not rebuilding pwrq. See pkg/udf/pwrgrep for the operators, and Dirs for
// where a rule of your own goes.
package pwrgrep

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// A Rule is one file of the corpus: the query, where it was found, and what
// its header says about it.
//
// A file usually holds several rules in the sense that matters to a caller -
// it reports findings under several ids - because rules that search the same
// files share one walk of the tree. Ids is those ids, and is what a caller
// names.
type Rule struct {
	// Path is the rule's place in the corpus, without the extension:
	// "go/lang/security/audit/crypto/use_of_weak_crypto".
	Path string
	// Ids are the finding ids this file reports under, from its `# rules:`
	// header. A caller names one of these.
	Ids []string
	// From is what the query was translated from, if it was, from its
	// `# from:` header.
	From string
	// Fixture is a file the rule is checked against, annotated line by line
	// with where it must fire and where it must not.
	Fixture string
	// Description is the prose in the header, which is where a rule says what
	// it does not cover and why a pattern is written the way it is.
	Description string
	// Query is the rule itself.
	Query string
	// Origin is where the file was read from: a directory on disk, or the copy
	// built into the binary. It is what tells a caller whether the rule they
	// are looking at is one they can edit.
	Origin string
}

// Id is the name a rule is usually called by, which is the first id its header
// names.
func (r *Rule) Id() string {
	if len(r.Ids) == 0 {
		return ""
	}
	return r.Ids[0]
}

var (
	// idsHeader and the two beside it are the whole of a rule's metadata. They
	// are comments in the query rather than a file alongside it, so that
	// moving a rule moves everything about it.
	idsHeader     = regexp.MustCompile(`(?m)^#\s*rules:\s*(.+?)\s*$`)
	fromHeader    = regexp.MustCompile(`(?m)^#\s*from:\s*(.+?)\s*$`)
	fixtureHeader = regexp.MustCompile(`(?m)^#\s*fixture:\s*(\S+)`)
)

// parse reads a rule out of a query file.
//
// A file with no `# rules:` header is not a rule: it may be a library the
// rules include, or a query somebody left in the directory. Saying so is
// better than inventing an id from the file name, because an id that nobody
// wrote is an id no finding will ever carry.
func parse(path, origin, source string) (*Rule, error) {
	m := idsHeader.FindStringSubmatch(source)
	if m == nil {
		return nil, fmt.Errorf("%s has no `# rules:` header naming the ids it reports under", path)
	}
	var ids []string
	for _, id := range strings.Split(m[1], ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("%s names no rule in its `# rules:` header", path)
	}
	rule := &Rule{Path: path, Ids: ids, Query: source, Origin: origin,
		Description: description(source)}
	if m := fromHeader.FindStringSubmatch(source); m != nil {
		rule.From = m[1]
	}
	if m := fixtureHeader.FindStringSubmatch(source); m != nil {
		rule.Fixture = m[1]
	}
	return rule, nil
}

// description is the prose in a rule's header: the comment block, minus the
// lines that are metadata and minus the `#` that makes it a comment.
func description(source string) string {
	var lines []string
	for _, line := range strings.Split(source, "\n") {
		if !strings.HasPrefix(line, "#") {
			// The header is what comes before the query, so the first line
			// that is not a comment ends it.
			if strings.TrimSpace(line) == "" && len(lines) == 0 {
				continue
			}
			if strings.TrimSpace(line) != "" {
				break
			}
			continue
		}
		if idsHeader.MatchString(line) || fromHeader.MatchString(line) || fixtureHeader.MatchString(line) {
			continue
		}
		lines = append(lines, strings.TrimPrefix(strings.TrimPrefix(line, "#"), " "))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// sortRules puts rules in the order a person reads them, which is also the
// order that makes a listing diffable.
func sortRules(rules []*Rule) {
	sort.SliceStable(rules, func(a, b int) bool { return rules[a].Path < rules[b].Path })
}
