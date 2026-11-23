# Journal: calibrating the IDE's suggestion questions to d1-2B

Run 2026-11-23 on gpu (q4f16, ONNX Runtime 1.30.0), 500 samples.

| variant | covered | top-1 | top-3 | lexical top-1 | NLL | ECE | Brier | more acc | p50 s | p95 s |
|---|---|---|---|---|---|---|---|---|---|---|
| intent+help | 206/250 | 0.165 | 0.359 | 0.354 | 2.899 | 0.234 | 0.979 | 0.556 | 0.03 | 0.04 |
| intent+help+example | 206/250 | 0.194 | 0.364 | 0.354 | 2.918 | 0.249 | 0.976 | 0.532 | 0.03 | 0.05 |
| plain+help+example | 206/250 | 0.189 | 0.437 | 0.354 | 2.563 | 0.151 | 0.906 | 0.620 | 0.03 | 0.05 |

Chosen: **intent+help+example** (best top-1 among covered samples). Blended with the prefilter's order at weight 4.0: top-1 0.359, against 0.194 for the model alone and 0.354 for the order alone.

Temperatures fitted by NLL: choice 4.799, more 4.799. Cross-validated (5 folds by label):

| | top-1 | NLL | ECE | Brier | more acc | more NLL | more Brier |
|---|---|---|---|---|---|---|---|
| raw (T=1) | 0.359 | 5.140 | 0.522 | 1.133 | 0.532 | 0.720 | 0.261 |
| tempered (CV) | 0.359 | 2.348 | 0.120 | 0.845 | 0.532 | 0.693 | 0.250 |

Ghost-text bar: 0.53 (precision 1.000, shown on 0.4% of stage starts).

By category (chosen variant, raw):

| category | n | top-1 | top-3 |
|---|---|---|---|
| Compare | 16 | 0.31 | 0.50 |
| Dates | 2 | 0.00 | 0.50 |
| Discovery | 3 | 0.33 | 0.33 |
| Domain | 1 | 0.00 | 1.00 |
| Encoding | 2 | 0.00 | 0.00 |
| JSON | 10 | 0.60 | 0.80 |
| Logs | 46 | 0.30 | 0.48 |
| Network | 4 | 0.50 | 0.75 |
| Numbers | 11 | 0.36 | 0.36 |
| Statistics | 4 | 0.50 | 0.50 |
| Tables | 42 | 0.40 | 0.48 |
| Text | 14 | 0.36 | 0.50 |
| Validation | 4 | 0.00 | 0.25 |
| jq | 47 | 0.38 | 0.60 |
