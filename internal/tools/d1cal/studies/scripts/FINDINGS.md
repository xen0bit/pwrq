# Findings: writing a whole query from a comment with d1-2B

- Question: `queries/task+query`.
- On held-out comments (test, 548 with a recipe, 9 without): the right recipe is a candidate 89% of the time, picked first 70% (top-3 85%); when it is, the slots are all right 92%; the whole query comes out exactly right for 65% of comments.
  - hand-written comments for the hand recipes (225): a query that does it 80% (the labelled recipe first 78%, its exact query 70%).
  - the catalog examples' intents (one comment an example, every example) (332): a query that does it 79% (the labelled recipe first 65%, its exact query 61%).
- Calibration: temperature 0.86 takes test NLL from 0.577 to 0.559 and ECE from 0.067 to 0.115.
- Ghost text appears unasked at confidence 0.49: on test it is the right recipe 83% of the time, shows for 90% of comments a recipe does and 56% of those none does.
- Latency (gpu): p50 0.082 s, p95 0.116 s, choice and slot questions in one pass.

Caveats. The eval set is small (1133 comments) and was written by the same hand as the recipes; the recipes' retrieval keywords (`# also:`) were last revised after reading a test run's misses, so the test numbers are somewhat optimistic. Comments for tasks no recipe does are the weak spot: a few still get a confident wrong recipe.

Re-run with `make d1.calibrate` after changing recipes.pwrq, the eval set or the question.
