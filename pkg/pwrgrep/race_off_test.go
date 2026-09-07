//go:build !race

package pwrgrep_test

// raceEnabled is false in a test binary built without the race detector.
const raceEnabled = false
