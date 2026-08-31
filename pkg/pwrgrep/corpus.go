package pwrgrep

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// builtin is the corpus, built into the binary.
//
// Rules ship with pwrq for the same reason grammars do: a rule is worth
// nothing on a machine that does not have it, and a tool that has to be
// pointed at a rule directory before it can say anything is a tool nobody runs
// twice. The package installs the same files to SystemDir, where they can be
// read and edited; this copy is what a bare `go install` binary falls back to.
//
//go:embed rules
var builtin embed.FS

// The directories a rule may come from, most specific first.
const (
	// EnvDirs names extra rule directories, separated the way PATH is. It goes
	// first, so a checkout of rules under development wins over everything
	// installed.
	EnvDirs = "PWRQ_RULES"
	// SystemDir is where the package puts the corpus.
	SystemDir = "/usr/share/pwrq/rules"
	// userDir is under the user's config directory, and is where somebody's
	// own rules live.
	userDir = "pwrq/rules"
)

// Dirs are the directories searched for rules, in order, before the copy built
// into the binary.
//
// A rule found earlier hides one with the same path found later, which is what
// makes a shipped rule editable: copy it into your own directory, change it,
// and it is the one that runs. Directories that do not exist are left out
// rather than reported, because not having a rules directory is the ordinary
// case.
func Dirs() []string {
	var dirs []string
	add := func(dir string) {
		if dir == "" {
			return
		}
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			dirs = append(dirs, dir)
		}
	}
	for _, dir := range filepath.SplitList(os.Getenv(EnvDirs)) {
		add(dir)
	}
	if config, err := os.UserConfigDir(); err == nil {
		add(filepath.Join(config, filepath.FromSlash(userDir)))
	}
	add(SystemDir)
	return dirs
}

// Builtin is the name Origin carries for a rule that came from the binary
// rather than from a directory.
const Builtin = "<built in>"

var (
	corpusOnce  sync.Once
	corpusRules []*Rule
	corpusErr   error
)

// Rules is every rule pwrq can see, in a fixed order.
//
// It is read once, on first use rather than at startup: a query that never
// mentions a rule should not pay for the catalogue.
func Rules() ([]*Rule, error) {
	corpusOnce.Do(func() { corpusRules, corpusErr = load() })
	return corpusRules, corpusErr
}

func load() ([]*Rule, error) {
	found := map[string]*Rule{}
	var order []string

	keep := func(rel, origin, source string) error {
		rel = strings.TrimSuffix(filepath.ToSlash(rel), ".pwrq")
		if _, taken := found[rel]; taken {
			// An earlier directory already answered for this path, which is
			// what overriding a shipped rule means.
			return nil
		}
		rule, err := parse(rel, origin, source)
		if err != nil {
			return err
		}
		found[rel] = rule
		order = append(order, rel)
		return nil
	}

	for _, dir := range Dirs() {
		err := filepath.WalkDir(dir, func(name string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Ext(name) != ".pwrq" {
				// A directory that vanished under the walk is not worth
				// failing a query over.
				if err != nil && os.IsNotExist(err) {
					return nil
				}
				return err
			}
			rel, err := filepath.Rel(dir, name)
			if err != nil {
				return err
			}
			source, err := os.ReadFile(name)
			if err != nil {
				return err
			}
			return keep(rel, dir, string(source))
		})
		if err != nil {
			return nil, fmt.Errorf("reading rules from %s: %v", dir, err)
		}
	}

	err := fs.WalkDir(builtin, "rules", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path.Ext(name) != ".pwrq" {
			return err
		}
		source, err := fs.ReadFile(builtin, name)
		if err != nil {
			return err
		}
		return keep(strings.TrimPrefix(name, "rules/"), Builtin, string(source))
	})
	if err != nil {
		return nil, err
	}

	// Two rules may report under one id, and do: the set this corpus was
	// translated from reuses an id where the same check is written twice for
	// two frameworks. Naming that id runs both, which is what a caller asking
	// for it meant; Path is what tells the two apart afterwards.
	rules := make([]*Rule, 0, len(order))
	for _, rel := range order {
		rules = append(rules, found[rel])
	}
	sortRules(rules)
	return rules, nil
}

// Select narrows the corpus to the rules a caller asked for.
//
// A selector is a finding id, or a glob over ids, or a path into the corpus -
// either a whole rule file or a directory of them. Selecting nothing that
// exists is an error rather than an empty run: "no rule called that" is a
// typo, and reporting it as "nothing found" hands back a clean bill of health
// nobody earned.
func Select(selectors []string) ([]*Rule, error) {
	all, err := Rules()
	if err != nil {
		return nil, err
	}
	if len(selectors) == 0 {
		return all, nil
	}
	var out []*Rule
	taken := make(map[*Rule]bool, len(all))
	for _, selector := range selectors {
		matched := false
		for _, rule := range all {
			if !rule.selectedBy(selector) {
				continue
			}
			matched = true
			if !taken[rule] {
				taken[rule] = true
				out = append(out, rule)
			}
		}
		if !matched {
			return nil, fmt.Errorf("no rule matches %q; get_pwrgrep_rule lists the %d there are",
				selector, len(all))
		}
	}
	sortRules(out)
	return out, nil
}

// selectedBy reports whether one selector names this rule.
func (r *Rule) selectedBy(selector string) bool {
	if selector == "" || selector == "*" || selector == "**" {
		return true
	}
	if selector == r.Path {
		return true
	}
	// A directory names everything under it, which is how a caller asks for
	// "the Go rules" without knowing what is in there.
	if strings.HasPrefix(r.Path, strings.TrimSuffix(selector, "/")+"/") {
		return true
	}
	re := globPattern(selector)
	if re.MatchString(r.Path) {
		return true
	}
	for _, id := range r.Ids {
		if id == selector || re.MatchString(id) {
			return true
		}
	}
	return false
}

var globCache sync.Map // string -> *regexp.Regexp

// globPattern turns a glob into the regexp that decides it. `*` stands for a
// run of anything but a slash, `**` for a run of anything at all, and `?` for
// one character - the spelling everyone already knows from a shell.
func globPattern(glob string) *regexp.Regexp {
	if cached, ok := globCache.Load(glob); ok {
		return cached.(*regexp.Regexp)
	}
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		switch c := glob[i]; c {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				b.WriteString(".*")
				i++
				continue
			}
			b.WriteString("[^/]*")
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	// The pattern is built here, so it cannot fail to compile; MustCompile
	// says so rather than leaving a reader to wonder what the error case is.
	re := regexp.MustCompile(b.String())
	globCache.Store(glob, re)
	return re
}
