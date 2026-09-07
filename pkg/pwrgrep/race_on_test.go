//go:build race

package pwrgrep_test

// raceEnabled is true in a test binary built with the race detector.
//
// There is no exported way to ask, and the answer decides whether the corpus
// test runs here or in a run of its own - so it is two files and a build tag
// rather than a runtime check.
const raceEnabled = true
