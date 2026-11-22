# Journal: the whole-query question on d1-2B

Run 2026-11-22 on gpu (q4f16), 879 comments (dev 449, test 430).

Dev split, raw (T=1):

| variant | coverage | top-1 | top-3 | slots | exact | works | NLL | ECE | negatives shown | p50 s |
|---|---|---|---|---|---|---|---|---|---|---|
| queries/task | 0.916 | 0.695 | 0.883 | 0.893 | 0.621 | 0.725 | 0.653 | 0.092 | 67% | 0.053 |
| queries/task+query | 0.916 | 0.731 | 0.887 | 0.895 | 0.655 | 0.767 | 0.525 | 0.072 | 83% | 0.074 |
| task/task | 0.916 | 0.686 | 0.874 | 0.891 | 0.612 | 0.722 | 0.694 | 0.084 | 67% | 0.052 |
| task/task+none | 0.916 | 0.679 | 0.876 | 0.887 | 0.603 | 0.716 | 1.004 | 0.105 | 50% | 0.053 |
| wants/task | 0.916 | 0.682 | 0.880 | 0.894 | 0.609 | 0.718 | 0.697 | 0.100 | 67% | 0.052 |
| wants/task+none | 0.916 | 0.677 | 0.880 | 0.893 | 0.605 | 0.711 | 1.092 | 0.126 | 50% | 0.053 |

Chosen: **queries/task+query** (most working queries on dev). Temperature 0.863 by dev NLL; ghost bar 0.49, the most right for the fewest wrong at precision 0.80 or more on dev (precision 0.823, shown on 89% of positives and 100% of negatives).

Test split, scored once:

| | coverage | top-1 | top-3 | slots | exact | works | NLL | ECE |
|---|---|---|---|---|---|---|---|---|
| raw (T=1) | 0.894 | 0.719 | 0.866 | 0.934 | 0.672 | 0.795 | 0.503 | 0.089 |
| tempered | 0.894 | 0.719 | 0.866 | 0.934 | 0.672 | 0.795 | 0.487 | 0.113 |
| tempered, hand comments (98) | 0.978 | 0.848 | 0.978 | 0.936 | 0.793 | 0.859 | 0.432 | 0.075 |
| tempered, example comments (332) | 0.870 | 0.684 | 0.834 | 0.934 | 0.639 | 0.777 | 0.504 | 0.156 |

Ghost text on test at the dev bar: precision 0.834, shown on 92% of positives and 67% of negatives.

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
| ffmpeg the avi into frames and see if anyone waves | video-frames-ffmpeg | slots: `oes the video show anyone waves?"}}; {Media: [{Typ → oes the video show a dog?"}}; {Media: [{Type: "vid` |
| at what times is the door open in cam.mp4 | video-when | slots: `Does the video show the door open?"}}; {Media: [{T → Does the video show door open?"}}; {Media: [{Type:` |
| ask d1 if the screen recording demo.webm shows an error dialog | video-shows | invoke_systemone#9 |
| when is a screen shared in meeting.webm | video-when | slots: `es the video show a screen shared?"}}; {Media: [{T → es the video show a dog?"}}; {Media: [{Type: "vide` |

## Continuing a recipe

Each labelled query cut after each stage, the cursor on the next line: the top suggestion is the exact rest on 215 of 312 dev cuts and 180 of 295 test cuts. Ghost bar 0.18 (dev: shown 205 of 312, right 165; test: shown 195 of 295, right 142).
