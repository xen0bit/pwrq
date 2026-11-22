# Findings: the IDE's suggestion questions on d1-2B

- Harness: `intent+help+example`. The choice's options are the engine's prefiltered candidates, each described by its help text and first example.
- Coverage: the label is among the 16 candidates on 206 of 250 stage starts (82%); the rest are the prefilter's misses, which no ranking can recover.
- Ranking: the model alone is right first 19.4% of the time and the prefilter's order alone 35.4%; blended (prior weight 4.0) top-1 is 35.9% and top-3 51.5% among covered samples.
- Calibration: temperature 4.80 takes the choice's ECE from 0.522 to 0.120 and NLL from 5.140 to 2.348 (cross-validated); the gate's temperature is 4.80.
- Ghost text appears unasked at confidence 0.53 or more, where it is right 100% of the time, on 0% of stage starts.
- Latency (gpu): p50 0.03 s, p95 0.05 s a suggestion.

Re-run with `make d1.calibrate` whenever the prompt (pkg/d1/suggest) or the candidate prefilter (pkg/ideengine/suggest.go) changes; `TestHarnessMatchesPrompt` fails until it is.
