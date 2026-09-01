package pwrgrep

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// WritableDir is where a rule of your own goes: the first directory of
// $PWRQ_RULES if that is set, and otherwise the one under the user's config
// directory. It is returned whether or not it exists yet, because the answer
// to "where do I put a rule" should not depend on somebody having made the
// directory first.
//
// SystemDir is never it. That is the package's copy of the corpus, owned by
// the package manager and replaced by the next upgrade, so a rule written
// there is a rule that quietly disappears.
func WritableDir() string {
	for _, dir := range filepath.SplitList(os.Getenv(EnvDirs)) {
		if dir != "" {
			return dir
		}
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(config, filepath.FromSlash(userDir))
}

// Write saves a rule to WritableDir under a catalogue path, and reports the
// rule it wrote.
//
// It exists because writing the file is the easy half. The rest is knowing
// where the file goes on this machine, and knowing before it lands that what
// is in it is a rule: a file with no `# rules:` header, or one whose query
// does not compile, is not a rule that fails to fire - it is a file that
// fails the whole catalogue, so the next `invoke_pwrgrep` for anything at all
// comes back with an error about a file the caller may have written an hour
// ago. Checking here means the failure is the answer to the write.
//
// Writing over an existing rule is allowed and is what iterating on one looks
// like. What is not allowed is writing outside the directory: a name is a
// place in the catalogue, not a path on the filesystem.
func Write(name, source string) (*Rule, string, error) {
	dir := WritableDir()
	if dir == "" {
		return nil, "", fmt.Errorf("no directory to write rules to; set %s to one", EnvDirs)
	}
	rel, err := rulePath(name)
	if err != nil {
		return nil, "", err
	}
	// Parsed and compiled before anything is written, so a rule that is not
	// one never reaches the directory the catalogue reads.
	rule, err := parse(rel, dir, source)
	if err != nil {
		return nil, "", err
	}
	if _, err := rule.Compile(); err != nil {
		return nil, "", err
	}

	file := filepath.Join(dir, filepath.FromSlash(rel)+".pwrq")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return nil, "", err
	}
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		return nil, "", err
	}
	return rule, file, nil
}

// rulePath reads a rule's name as a place in the catalogue.
//
// The extension is optional because Path in the catalogue does not carry one
// and a caller copying a path back in should not have to notice. Everything
// else is refused: a name that climbs out of the directory, or names an
// absolute path, is a write somewhere on the filesystem wearing a rule's
// clothes.
func rulePath(name string) (string, error) {
	rel := strings.TrimSuffix(filepath.ToSlash(strings.TrimSpace(name)), ".pwrq")
	if rel == "" {
		return "", fmt.Errorf("a rule needs a name to be written under")
	}
	// The name comes first and the text second, the way it does for every
	// other cmdlet that writes something. Handing them over the other way
	// round otherwise fails as "this rule has no header", which is true of the
	// name and unhelpful about why.
	if strings.ContainsAny(rel, "\n\r") {
		return "", fmt.Errorf("that is the rule's text, not its name: write_pwrgrep_rule(\"mine/no-timeout\"; $source), or the source as the argument when the name is what is piped in")
	}
	if filepath.IsAbs(name) || strings.HasPrefix(rel, "/") {
		return "", fmt.Errorf("%q is an absolute path; a rule is named by its place in the catalogue, like \"mine/no-timeout\"", name)
	}
	if cleaned := path.Clean(rel); cleaned != rel || strings.HasPrefix(cleaned, "..") {
		return "", fmt.Errorf("%q does not name a place in the catalogue", name)
	}
	return rel, nil
}
