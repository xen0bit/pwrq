package astsearch

import "github.com/xen0bit/pwrq/pkg/core/filewalk"

// SearchTree is select_ast without the stream: every match of every pattern
// under root, collected, as the same objects select_ast emits.
//
// It exists for the rule vocabulary, which combines whole result sets rather
// than reading them one at a time - "this call, in a file that also imports
// that" is a question about all the matches at once - and which asks about
// several languages in one search, so most patterns being code in none of the
// files met is ordinary here rather than the mistake it is when a person typed
// one pattern. That is the one difference: the diagnostic select_ast ends its
// stream with is left to select_ast. See exhausted.
func SearchTree(root string, patterns []string, include string) ([]any, error) {
	walk, err := filewalk.New(root, include)
	if err != nil {
		return nil, err
	}
	it := &matchIter{patterns: patterns, opts: selectAstOpts{include: include}, walk: walk}
	var out []any
	for {
		path, ok, err := walk.Next()
		if err != nil {
			return nil, err
		}
		if !ok {
			return out, nil
		}
		found, err := it.searchFile(path)
		if err != nil {
			return nil, err
		}
		out = append(out, found...)
	}
}
