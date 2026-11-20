# Journal: calibrating the IDE's suggestion questions to d1-2B

Run 2026-11-20 on cpu (q4, ONNX Runtime 1.30.0), 500 samples.

| variant | covered | top-1 | top-3 | lexical top-1 | NLL | ECE | Brier | more acc | p50 s | p95 s |
|---|---|---|---|---|---|---|---|---|---|---|
| intent+help | 206/250 | 0.121 | 0.311 | 0.379 | 3.525 | 0.367 | 1.112 | 0.556 | 0.99 | 1.95 |
| intent+help+example | 206/250 | 0.141 | 0.325 | 0.379 | 3.612 | 0.397 | 1.137 | 0.536 | 1.13 | 2.04 |
| plain+help+example | 206/250 | 0.141 | 0.388 | 0.379 | 2.845 | 0.270 | 0.989 | 0.612 | 1.09 | 1.95 |

Chosen: **intent+help+example** (best top-1 among covered samples).

Temperatures fitted by NLL: choice 3.605, more 3.966. Cross-validated (5 folds by label):

| | top-1 | NLL | ECE | Brier | more acc | more NLL | more Brier |
|---|---|---|---|---|---|---|---|
| raw (T=1) | 0.141 | 3.612 | 0.397 | 1.137 | 0.536 | 0.744 | 0.271 |
| tempered (CV) | 0.141 | 2.849 | 0.163 | 0.983 | 0.536 | 0.691 | 0.249 |

Ghost-text bar: 0.99 (precision 0.000, shown on 0.0% of stage starts).

By category (chosen variant, raw):

| category | n | top-1 | top-3 |
|---|---|---|---|
| Compare | 16 | 0.12 | 0.31 |
| Dates | 2 | 0.00 | 0.00 |
| Discovery | 3 | 0.00 | 0.00 |
| Domain | 1 | 1.00 | 1.00 |
| Encoding | 2 | 0.50 | 1.00 |
| JSON | 10 | 0.50 | 0.60 |
| Logs | 46 | 0.02 | 0.22 |
| Network | 4 | 0.25 | 0.50 |
| Numbers | 11 | 0.00 | 0.36 |
| Statistics | 4 | 0.25 | 0.25 |
| Tables | 42 | 0.12 | 0.14 |
| Text | 14 | 0.14 | 0.50 |
| Validation | 4 | 0.00 | 0.25 |
| jq | 47 | 0.21 | 0.47 |
