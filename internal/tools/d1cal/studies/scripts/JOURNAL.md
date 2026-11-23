# Journal: the whole-query question on d1-2B

Run 2026-11-23 on gpu (q4f16), 1133 comments (dev 576, test 557).

Dev split, raw (T=1):

| variant | coverage | top-1 | top-3 | slots | exact | works | NLL | ECE | negatives shown | p50 s |
|---|---|---|---|---|---|---|---|---|---|---|
| queries/task | 0.930 | 0.729 | 0.886 | 0.889 | 0.648 | 0.766 | 0.611 | 0.075 | 100% | 0.053 |
| queries/task+query | 0.930 | 0.755 | 0.893 | 0.886 | 0.669 | 0.798 | 0.574 | 0.061 | 75% | 0.082 |
| task/task | 0.930 | 0.732 | 0.886 | 0.889 | 0.651 | 0.769 | 0.642 | 0.071 | 100% | 0.053 |
| task/task+none | 0.930 | 0.724 | 0.886 | 0.886 | 0.641 | 0.762 | 0.889 | 0.089 | 50% | 0.054 |
| wants/task | 0.930 | 0.729 | 0.891 | 0.879 | 0.641 | 0.766 | 0.640 | 0.078 | 75% | 0.053 |
| wants/task+none | 0.930 | 0.725 | 0.889 | 0.881 | 0.639 | 0.764 | 0.950 | 0.116 | 38% | 0.054 |

Chosen: **queries/task+query** (most working queries on dev). Temperature 0.863 by dev NLL; ghost bar 0.49, the most right for the fewest wrong at precision 0.80 or more on dev (precision 0.836, shown on 91% of positives and 75% of negatives).

Test split, scored once:

| | coverage | top-1 | top-3 | slots | exact | works | NLL | ECE |
|---|---|---|---|---|---|---|---|---|
| raw (T=1) | 0.889 | 0.701 | 0.854 | 0.924 | 0.648 | 0.794 | 0.577 | 0.067 |
| tempered | 0.889 | 0.701 | 0.854 | 0.924 | 0.648 | 0.794 | 0.559 | 0.115 |
| tempered, hand comments (225) | 0.958 | 0.778 | 0.931 | 0.905 | 0.704 | 0.796 | 0.562 | 0.047 |
| tempered, example comments (332) | 0.843 | 0.651 | 0.804 | 0.940 | 0.611 | 0.792 | 0.557 | 0.180 |

Ghost text on test at the dev bar: precision 0.831, shown on 90% of positives and 56% of negatives.

Misses on test (chosen variant):

| comment | wanted | got |
|---|---|---|
| which screenshots contain a terminal window, ask d1 | images-containing | images-classify |
| how many times does error appear in each file | count-matches | errors-per-log |
| first 20 lines of data.csv | head-file | head#1 |
| count lines of python code in src | lines-of-code | loc-by-language |
| what kind of file is mystery.bin | file-type | file_type#1 |
| is 10.1.2.3 a private address | ip-info | is_private_ip#1 |
| only the entries where status is failed | filter-by-field | failed-requests |
| stats for latency | stats-field | slowest-events |
| just the ids | pluck-field | is_uuid#1 |
| use d1 to find lines in app.log about disk space | classify-lines | error-rate |
| use ffmpeg to get frames from game.mkv and ask d1 if there is a crowd | video-frames-ffmpeg | slots: `Does the video show crowd?"}}; {Media: [{Type: "vi → Does the video show a dog?"}}; {Media: [{Type: "vi` |
| ffmpeg the avi into frames and see if anyone waves | video-frames-ffmpeg | slots: `oes the video show anyone waves?"}}; {Media: [{Typ → oes the video show a dog?"}}; {Media: [{Type: "vid` |
| at what times is the door open in cam.mp4 | video-when | videos-containing (not a candidate) |
| ask d1 if the screen recording demo.webm shows an error dialog | video-shows | ask-llm |
| when is a screen shared in meeting.webm | video-when | slots: `es the video show a screen shared?"}}; {Media: [{T → es the video show a dog?"}}; {Media: [{Type: "vide` |
| list files no deeper than 1 level | files-max-depth | list-files |
| tiniest files here | smallest-files | empty-files (not a candidate) |
| files over 100MB not modified in 180 days in ~/Downloads | cleanup-candidates | files-older-than (not a candidate) |
| check if backup.tar is byte for byte the same as orig.tar | files-identical | slots: `("backup.tar" \| sha256(true)) == → ("orig.tar" \| sha256(true)) == (` |
| tac history.txt | reverse-lines | files-by-extension |
| in README.md change localhost to example.com | replace-in-file | rename-extension (not a candidate) |
| duplicate config.yaml as config.old.yaml | copy-file | slots: `tem("config.yaml"; "config.old.yaml") → tem("config.yaml"; "notes-copy.txt")` |
| move report.pdf into archive/report.pdf | move-file | move_item#1 |
| rename scans so they start with the date | rename-with-date | slots: `get_childitem("scans")
\| select(.Mode \| start → get_childitem("photos")
\| select(.Mode \| star` |
| grep the .go files in pkg for context.TODO | search-file-type | search-with-context |
| grep every .log under /var/log for FATAL | errors-in-logs | errors-per-log |
| count todo comments per python file | todos-per-file | count-matches |
| cloc for ~/code/app | loc-by-language | code-rules |
| error rate of server.log | error-rate | errors-per-log |
| most recent errors in worker.log | last-errors | errors-in-logs |
| events in events.jsonl from 2026-01-01 to 2026-01-02 | log-between-times | read-jsonl (not a candidate) |
| top 20 pages by hits | top-paths | top_by#1 |
| which files changed since the manifest baseline.json | verify-checksums | recent-files |
| hmac-sha256 of notes.txt | hmac-file | file-hash |
| parse the tab separated file export.tsv | read-tsv | tsv_parse#1 |
| parse the ini file php.ini | read-ini | ini_parse#1 |
| describe the orders table in shop.db | sqlite-schema | format_table#1 |
| dump orders from shop.db as orders.csv | sqlite-to-csv | slots: `nvoke_sqlite_query("shop.db"; "select * from order → nvoke_sqlite_query("orders.csv"; "select * from or` |
| systemctl start docker | start-service | start_service#1 |
| current time in America/New_York | time-in-zone | current-date |
| fetch https://api.example.com/status and take its version | fetch-json-field | semver_parts#1 |
| use d1 to sort pictures showing receipts in scans into scans/receipts | move-images-containing | images-classify |
| use the decision model to prioritize the todos in src | urgent-todos | todos-per-file |
| ask claude to explain kubernetes | ask-llm | slots: `invoke_llm("explain kubernetes") → invoke_llm("What is jq")` |

## Continuing a recipe

Each labelled query cut after each stage, the cursor on the next line: the top suggestion is the exact rest on 421 of 549 dev cuts and 353 of 532 test cuts. Ghost bar 0.14 (dev: shown 491 of 549, right 407; test: shown 449 of 532, right 330).
