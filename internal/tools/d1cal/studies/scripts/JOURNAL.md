# Journal: the whole-query question on d1-2B

Run 2026-11-22 on gpu (q4f16), 851 comments (dev 434, test 417).

Dev split, raw (T=1):

| variant | coverage | top-1 | top-3 | slots | exact | works | NLL | ECE | negatives shown | p50 s |
|---|---|---|---|---|---|---|---|---|---|---|
| queries/task | 0.916 | 0.696 | 0.883 | 0.909 | 0.633 | 0.731 | 0.660 | 0.091 | 67% | 0.055 |
| queries/task+query | 0.916 | 0.734 | 0.886 | 0.911 | 0.668 | 0.773 | 0.506 | 0.067 | 83% | 0.077 |
| task/task | 0.916 | 0.694 | 0.876 | 0.909 | 0.631 | 0.736 | 0.698 | 0.085 | 83% | 0.055 |
| task/task+none | 0.916 | 0.685 | 0.874 | 0.904 | 0.619 | 0.724 | 1.004 | 0.115 | 67% | 0.055 |
| wants/task | 0.916 | 0.682 | 0.881 | 0.911 | 0.621 | 0.724 | 0.700 | 0.096 | 67% | 0.055 |
| wants/task+none | 0.916 | 0.678 | 0.881 | 0.910 | 0.617 | 0.717 | 1.091 | 0.131 | 50% | 0.056 |

Chosen: **queries/task+query** (most working queries on dev). Temperature 0.863 by dev NLL; ghost bar 0.49, the most right for the fewest wrong at precision 0.80 or more on dev (precision 0.834, shown on 89% of positives and 100% of negatives).

Test split, scored once:

| | coverage | top-1 | top-3 | slots | exact | works | NLL | ECE |
|---|---|---|---|---|---|---|---|---|
| raw (T=1) | 0.891 | 0.720 | 0.861 | 0.946 | 0.681 | 0.808 | 0.503 | 0.085 |
| tempered | 0.891 | 0.720 | 0.861 | 0.946 | 0.681 | 0.808 | 0.489 | 0.114 |
| tempered, hand comments (86) | 0.975 | 0.838 | 0.975 | 0.985 | 0.825 | 0.912 | 0.453 | 0.082 |
| tempered, example comments (331) | 0.870 | 0.692 | 0.834 | 0.934 | 0.647 | 0.782 | 0.499 | 0.147 |

Ghost text on test at the dev bar: precision 0.851, shown on 91% of positives and 67% of negatives.

Misses on test (chosen variant):

| comment | wanted | got |
|---|---|---|
| which screenshots contain a terminal window, ask d1 | images-containing | images-classify |
| find all json files under config | files-by-extension | list-files-recursive (not a candidate) |
| first 20 lines of data.csv | head-file | head#1 |
| what kind of file is mystery.bin | file-type | file_type#1 |
| all links on https://go.dev | page-links | extract_urls#1 |
| is 10.1.2.3 a private address | ip-info | is_private_ip#1 |
| just the ids | pluck-field | is_uuid#1 |

## Continuing a recipe

Each labelled query cut after each stage, the cursor on the next line: the top suggestion is the exact rest on 197 of 282 dev cuts and 165 of 268 test cuts. Ghost bar 0.19 (dev: shown 168 of 282, right 136; test: shown 153 of 268, right 116).
