# Findings: writing a whole query from a comment with d1-2B

- Question: `queries/task+query`.
- On held-out comments (test, 424 with a recipe, 6 without): the right recipe is a candidate 89% of the time, picked first 72% (top-3 87%); when it is, the slots are all right 93%; the whole query comes out exactly right for 67% of comments.
  - hand-written comments for the hand recipes (98): a query that does it 86% (the labelled recipe first 85%, its exact query 79%).
  - the catalog examples' intents (one comment an example, every example) (332): a query that does it 78% (the labelled recipe first 68%, its exact query 64%).
- Calibration: temperature 0.86 takes test NLL from 0.503 to 0.487 and ECE from 0.089 to 0.113.
- Ghost text appears unasked at confidence 0.49: on test it is the right recipe 83% of the time, shows for 92% of comments a recipe does and 67% of those none does.
- Latency (gpu): p50 0.077 s, p95 0.121 s, choice and slot questions in one pass.

Caveats. The eval set is small (879 comments) and was written by the same hand as the recipes; the recipes' retrieval keywords (`# also:`) were last revised after reading a test run's misses, so the test numbers are somewhat optimistic. Comments for tasks no recipe does are the weak spot: a few still get a confident wrong recipe.

Re-run with `make d1.calibrate` after changing recipes.pwrq, the eval set or the question.
