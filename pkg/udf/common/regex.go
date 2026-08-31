package common

import (
	"regexp"
	"sync"
)

// A regex compiled once rather than once per value.
//
// A cmdlet that filters a pipeline is handed one value at a time, so a pattern
// compiled where the comparison happens is a pattern compiled per value: a
// where_object over ten thousand rows compiled the same regex ten thousand
// times, which cost more than the matching did. The pattern comes from the
// query rather than from the data, so there are as many of them as the query
// wrote - a handful - however long the pipeline is.

var compiledRegexes sync.Map // pattern -> regexResult

type regexResult struct {
	re  *regexp.Regexp
	err error
}

// Regex compiles a pattern, or hands back the compilation from last time.
//
// A pattern that will not compile is remembered too, because a query that
// wrote a bad regex writes it once and is handed the same complaint for every
// value rather than deriving it again.
func Regex(pattern string) (*regexp.Regexp, error) {
	if cached, ok := compiledRegexes.Load(pattern); ok {
		r := cached.(regexResult)
		return r.re, r.err
	}
	re, err := regexp.Compile(pattern)
	compiledRegexes.Store(pattern, regexResult{re: re, err: err})
	return re, err
}
