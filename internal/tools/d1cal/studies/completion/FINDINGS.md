# Findings: the IDE's suggestion questions on d1-2B

- Harness: `intent+help+example`. The choice's options are the engine's prefiltered candidates, each described by its help text and first example.
- Coverage: the label is among the 16 candidates on 206 of 250 stage starts (82%); the rest are the prefilter's misses, which no ranking can recover.
- Ranking: top-1 14.1% and top-3 32.5% among covered samples, against 37.9% for the lexical order alone.
- Calibration: temperature 3.60 takes the choice's ECE from 0.397 to 0.163 and NLL from 3.612 to 2.849 (cross-validated); the gate's temperature is 3.97.
- Ghost text appears unasked at confidence 0.99 or more, where it is right 0% of the time, on 0% of stage starts.
- Latency (cpu): p50 1.13 s, p95 2.04 s a suggestion.

Re-run with `make d1.calibrate` whenever the prompt (pkg/d1/suggest) or the candidate prefilter (pkg/ideengine/suggest.go) changes; `TestHarnessMatchesPrompt` fails until it is.

Caveats. The staple candidates (always offered, six slots) were chosen after
looking at the gallery's label distribution, so coverage and top-1 are slightly
optimistic. Top-1 is low: the ghost bar sits at 0.99 because no lower bar reached
0.8 precision, so suggestions show on their own almost never, and Ctrl+Space is
the way to ask. The next lever is the candidate set, not the temperature.
