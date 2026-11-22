# Journal: the whole-query question on d1-2B

Run 2026-11-22 on gpu (q4f16), 874 comments (dev 446, test 428).

Dev split, raw (T=1):

| variant | coverage | top-1 | top-3 | slots | exact | works | NLL | ECE | negatives shown | p50 s |
|---|---|---|---|---|---|---|---|---|---|---|
| queries/task | 0.916 | 0.693 | 0.880 | 0.898 | 0.623 | 0.730 | 0.659 | 0.091 | 67% | 0.054 |
| queries/task+query | 0.916 | 0.736 | 0.882 | 0.901 | 0.664 | 0.775 | 0.521 | 0.075 | 83% | 0.076 |
| task/task | 0.916 | 0.684 | 0.870 | 0.900 | 0.616 | 0.725 | 0.698 | 0.079 | 83% | 0.053 |
| task/task+none | 0.916 | 0.680 | 0.875 | 0.893 | 0.607 | 0.718 | 1.007 | 0.108 | 50% | 0.054 |
| wants/task | 0.916 | 0.677 | 0.882 | 0.899 | 0.609 | 0.716 | 0.703 | 0.104 | 67% | 0.053 |
| wants/task+none | 0.916 | 0.675 | 0.880 | 0.896 | 0.605 | 0.714 | 1.095 | 0.126 | 50% | 0.056 |

Chosen: **queries/task+query** (most working queries on dev). Temperature 0.863 by dev NLL; ghost bar 0.49, the most right for the fewest wrong at precision 0.80 or more on dev (precision 0.825, shown on 90% of positives and 100% of negatives).

Test split, scored once:

| | coverage | top-1 | top-3 | slots | exact | works | NLL | ECE |
|---|---|---|---|---|---|---|---|---|
| raw (T=1) | 0.893 | 0.720 | 0.865 | 0.938 | 0.675 | 0.801 | 0.500 | 0.081 |
| tempered | 0.893 | 0.720 | 0.865 | 0.938 | 0.675 | 0.801 | 0.485 | 0.110 |
| tempered, hand comments (96) | 0.978 | 0.856 | 0.978 | 0.948 | 0.811 | 0.878 | 0.408 | 0.082 |
| tempered, example comments (332) | 0.870 | 0.684 | 0.834 | 0.934 | 0.639 | 0.780 | 0.508 | 0.156 |

Ghost text on test at the dev bar: precision 0.841, shown on 91% of positives and 67% of negatives.

Misses on test (chosen variant):

| comment | wanted | got |
|---|---|---|
| which screenshots contain a terminal window, ask d1 | images-containing | diff_lines#1 |
| files bigger than 500 KB | files-larger-than | largest-files |
| find all json files under config | files-by-extension | list-files-recursive (not a candidate) |
| first 20 lines of data.csv | head-file | head#1 |
| what kind of file is mystery.bin | file-type | file_type#1 |
| all links on https://go.dev | page-links | extract_urls#1 |
| is 10.1.2.3 a private address | ip-info | is_private_ip#1 |
| just the ids | pluck-field | is_uuid#1 |
| use ffmpeg to get frames from game.mkv and ask d1 if there is a crowd | video-frames-ffmpeg | slots: `Does the video show crowd?"}}; {Media: [{Type: "vi → Does the video show a dog?"}}; {Media: [{Type: "vi` |
| ffmpeg the webm into frames and see if anyone waves | video-frames-ffmpeg | slots: `oes the video show anyone waves?"}}; {Media: [{Typ → oes the video show a dog?"}}; {Media: [{Type: "vid` |
| at what times is the door open in cam.mp4 | video-when | slots: `Does the video show the door open?"}}; {Media: [{T → Does the video show door open?"}}; {Media: [{Type:` |

## Continuing a recipe

Each labelled query cut after each stage, the cursor on the next line: the top suggestion is the exact rest on 214 of 310 dev cuts and 178 of 293 test cuts. Ghost bar 0.18 (dev: shown 201 of 310, right 161; test: shown 193 of 293, right 140).
