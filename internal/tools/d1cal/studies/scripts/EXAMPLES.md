# Examples the scripts study misses

Every catalog example has a comment in intents.jsonl. These are the ones whose comment did not come back as a query that does it - the example's cmdlets with the values the comment states - with what came instead. Fix the example, the cmdlet's description, or the comment if it is not what someone would write - then `make d1.calibrate`.

| example | comment | got |
|---|---|---|
| find#2 | list directories here, at most 2 levels deep | list-directories |
| cat#3 | dump the contents of every file in this directory | list-files |
| cat#4 | show the last 20 lines of app.log | tail-file (not a candidate) |
| read_bytes#2 | how many bytes is a.bin | human_bytes#1 |
| binary_decode#1 | convert the bits 01101000 01101001 back to text | hamming_weight#1 (not a candidate) |
| url_encode#1 | percent-encode a b&c for a url | slots: `"a b\u0026c" \| url_encode → "b\u0026c" \| url_encode` |
| url_decode#1 | url decode a%20b%26c | url-decode |
| html_encode#1 | escape <b>hi</b> for html | slots: `u003e" \| html_encode → u003e" \| html_encode` |
| html_decode#1 | unescape html entities in &lt;b&gt;hi&lt;/b&gt; | slots: `"\u0026lt;b\u0026gt;hi\u0026lt; → "html entities" \| html_decode` |
| gzip_decompress#1 | gzip and gunzip hello | gzip_compress#1 |
| zlib_decompress#1 | zlib round trip of hello | zlib_compress#1 |
| deflate_compress#1 | raw deflate hello | slots: `"hello" \| deflate_compress → deflate_compress` |
| sha1#1 | sha1 hash of hello | sha1#2 |
| sha256#1 | sha256 of the string hello | sha256-text |
| sha256#2 | sha256sum of file.txt | sha256-text |
| hmac_sha256#1 | hmac-sha256 signature of message | sha256-text |
| timestamp_to_date#2 | what's the current time as a date | current-date |
| date_to_timestamp#2 | epoch seconds for 2021-01-01 | timestamps-to-dates (not a candidate) |
| csv_parse#3 | split a,b,c as csv | csv_stringify#1 |
| xml_parse#1 | parse the xml <root>test</root> | slots: `"\u003croot\u003etest\u003c/roo → "xml" \| xml_parse` |
| ssdeep#1 | fuzzy hash of firmware.bin | ssdeep#2 |
| ssdeep_compare#1 | how similar are firmware.bin and firmware2.bin by fuzzy hash | ssdeep#2 (not a candidate) |
| tee#2 | save a copy to /tmp/output.json while passing it through | json_stringify#2 (not a candidate) |
| tempdir#4 | new scratch directory in /tmp | mkdir#1 |
| http#1 | status code of a GET to https://example.com | http-status |
| http#2 | GET https://api.example.com/x and parse the json body | read-json |
| http#3 | fetch https://example.com | invoke_web_request#1 |
| http#4 | POST json to https://api.example.com | fetch-json |
| http#5 | download https://example.com/a.tar.gz and save it as a.tar.gz | download-file |
| aes_decrypt#2 | aes cbc round trip encrypt decrypt | aes_encrypt#2 |
| des_decrypt#1 | des encrypt then decrypt | des_encrypt#1 |
| triple_des_encrypt#1 | 3des encrypt data | des_encrypt#1 |
| blowfish_decrypt#2 | blowfish cbc encrypt and decrypt | blowfish_encrypt#2 |
| rc4#1 | rc4 encrypt secret with key | rc4#2 |
| rc4#2 | rc4 the bytes of a.bin | file_type#1 |
| chacha20#1 | chacha20 encrypt secret | chacha20#2 |
| chacha20#2 | chacha20 the file a.bin | file_type#1 |
| slugify#1 | slugify Hello World! | slots: `llo World" \| slugify → llo World" \| slugify` |
| camel_case#1 | convert hello world to camelCase | json_stringify#1 (not a candidate) |
| pascal_case#1 | PascalCase hello world | newest-files (not a candidate) |
| title_case#1 | title case the quick brown fox | slots: `"the quick brown fox" \| title_c → "quick brown fox" \| title_case` |
| pad_left#1 | zero pad 5 to 3 digits | slots: `"5" \| pad_left(3; "0") → pad_left(5; "0")` |
| pad_right#1 | pad 5 on the right with zeros to width 3 | slots: `"5" \| pad_right(3; "0") → pad_right(5; "0")` |
| count_occurrences#1 | how many times does an appear in banana | char_frequencies#1 (not a candidate) |
| to_base#1 | 42 in hexadecimal | hex_encode#1 (not a candidate) |
| to_base#2 | 255 in binary | binary_encode#1 (not a candidate) |
| human_bytes#1 | 1048576 bytes in human readable units | human_number#1 |
| percentage#1 | what percent is 40 of 200 | similarity_percent#1 (not a candidate) |
| basename#1 | file name of /tmp/data/file.txt | stem#1 (not a candidate) |
| dirname#1 | directory part of /tmp/data/file.txt | list-directories (not a candidate) |
| mean#1 | average of these numbers | average-field (not a candidate) |
| human_duration#1 | 3661 seconds as a readable duration | iso_duration#1 |
| parse_duration#1 | how many seconds is 2h30m | parse_duration#3 |
| parse_duration#2 | parse the iso duration P1DT2H3M4S | iso_duration#1 |
| is_ip#1 | is 192.168.1.1 a valid ip address | is_ipv4#1 |
| base64url_encode#1 | url-safe base64 encode hello? | slots: `" \| base64url_encode → " \| base64url_encode` |
| rot13#1 | rot13 hello | slots: `"hello" \| rot13 → rot13` |
| to_timezone#1 | convert 2026-08-11T12:00:00Z to london time | slots: `"2026-08-11T12:00:00Z" \| to_tim → "london" \| to_timezone("Europe/` |
| to_timezone#2 | show a timestamp in Asia/Tokyo time | current-date |
| format_date#2 | format a unix time as rfc1123 in UTC | timestamp_to_date#2 |
| parse_date#1 | parse 11/08/2026 as day/month/year | day_of_year#1 (not a candidate) |
| list_timezones#2 | which timezone is Tokyo | to_timezone#2 (not a candidate) |
| deep_diff#1 | what's different between two json objects | compare_object#1 (not a candidate) |
| rncd_compare#3 | rank the files in samples by similarity | similarity_percent#1 |
| shared_chunks#1 | what text do two strings share | where_same#1 (not a candidate) |
| shared_chunks#2 | how much of known.bin is inside suspect.bin | directory-size (not a candidate) |
| select_string#4 | find calls to join or walk in src | invoke_pwrgrep#1 (not a candidate) |
| select_ast#1 | names of go functions in src that return an error | invoke_pwrgrep#1 (not a candidate) |
| select_ast#2 | count the if statements in go files under src | lines-of-code (not a candidate) |
| select_ast#3 | find md5 and sha1 use in src grouped by pattern | sha1#1 |
| select_ast#4 | find tls configs that skip certificate verification | ini_parse#1 (not a candidate) |
| ast_pattern#2 | metavariables of an ast pattern | select_ast#4 |
| ast_pattern#3 | check a php ast pattern for problems | select_ast#4 |
| get_ast_language#2 | file extensions for the go grammar | file_extension#1 |
| wc_lines#1 | line count of app.log | line_count#1 (not a candidate) |
| invoke_pwrgrep#2 | run the go and python rules on src | scan_ast#1 |
| invoke_pwrgrep#3 | count go flow findings in src by rule | scan_ast#1 (not a candidate) |
| get_pwrgrep_rule#2 | how many pwrgrep rules are there | code-rules |
| get_pwrgrep_rule#3 | ids of the python rules | invoke_pwrgrep#2 |
| of#2 | scan src for weak hashes and keep only md5 | md5#1 (not a candidate) |
| within#1 | matches that are inside another match | where_capture_ast#1 |
| in_files_with#1 | matches in files that also contain something else | select_ast#4 |
| not_at#1 | drop matches at the same place as others | at_same_place#1 |
| where_capture_not#1 | drop matches whose algorithm is sha256 | hash-all-files |
| scan_regex#2 | count passwords in yaml files under src | yaml_parse#2 (not a candidate) |
| focus#1 | report the line of a capture rather than the match | search-text |
| finding#2 | report each java method as a finding | report#1 |
| is_ascii#1 | is the text plain ascii | slots: `"plain" \| is_ascii → is_ascii` |
| word_count#1 | count the words in the quick brown fox | slots: `"the quick brown fox" \| word_co → "quick brown fox" \| word_count` |
| match_glob#1 | does notes.txt match *.txt | slots: `.txt" \| match_glob("*.txt") → .txt" \| match_glob("notes.txt")` |
| unflatten_keys#1 | expand dotted keys into nested objects | flatten-json |
| query_string_parse#1 | parse the query string a=1&b=two | slots: `\| query_string_parse → \| query_string_parse` |
| duration_between#1 | how long from 2026-01-01 to 2026-01-03 | count-items (not a candidate) |
| ip_version#1 | is 192.168.1.1 ipv4 or ipv6 | is_ip#1 (not a candidate) |
| keccak_256#1 | keccak256 of hello | duplicate-files (not a candidate) |
| human_number#1 | 1500 as 1.5k | largest-files (not a candidate) |
| rescale#1 | rescale 5 from 0-10 to 0-100 | slots: `5 \| rescale(0; 10; 0; 100) → 0 \| rescale(5; 5; 5; 5)` |
| subnet_of#1 | is 10.0.0.0/24 a subnet of 10.0.0.0/8 | slots: `"10.0.0.0/24" \| subnet_of("10.0.0.0/8") → "10.0.0.0/8" \| subnet_of("10.0.0.0/8")` |
| n_grams#1 | bigrams of hello | files-named (not a candidate) |
| set_content#2 | overwrite file.txt with some text | search-text (not a candidate) |
| out_file#1 | save the output to report.txt | report#1 (not a candidate) |
| out_file#2 | append the output to run.log | add_content#2 (not a candidate) |
| test_path#2 | is /tmp a directory | is_dir_path#1 (not a candidate) |
| select_object#1 | keep only the Name property | pluck-field (not a candidate) |
| select_object#2 | pick the Name field of each object | pluck-field |
| where_object#1 | keep the numbers greater than 10 | filter-greater-than (not a candidate) |
| where_object#2 | people older than 26 | filter-greater-than (not a candidate) |
| sort_object#1 | sort by Name | sort-by-field |
| measure_object#1 | count the items | count-items (not a candidate) |
| measure_object#2 | sum of the values | sum-field |
| measure_object#3 | total of the v property | sum-field |
| format_list#2 | list view of the Name property | sort_object#1 |
| format_table#2 | table with just the Name column | pluck-field |
| get_variable#2 | list all session variables | environment-variables (not a candidate) |
| remove_variable#2 | clear all variables | environment-variables (not a candidate) |
| move_item#1 | rename draft.txt to final.txt | rename_keys#1 (not a candidate) |
| resolve_path#2 | full path of the current directory | current-directory |
| get_date#2 | what year is it | day_of_year#1 (not a candidate) |
| invoke_llm#2 | ask a language model for one word for blue | slots: `"one word for blue" \| invoke_ll → "blue" \| invoke_llm` |
| invoke_llm#3 | classify with an llm and get json back | invoke_llm_request#1 |
| invoke_llm_request#2 | llm call with caching | get_llm_usage#1 |
| invoke_llm_batch#2 | summarize each item with an llm | summarize-file |
| invoke_systemone#3 | read the invoice total and date with the decision model | sum-field |
| invoke_systemone#4 | count the cups in kitchen.jpg | images-count-objects (not a candidate) |
| invoke_systemone#9 | ask the remote jev model which team | get_llm_model#1 |
| invoke_systemone_batch#1 | flag spam in each item with the decision model | classify-lines (not a candidate) |
| get_llm_context#2 | context length of gpt-4o | count-files |
| get_llm_usage#2 | total tokens used | sum-field (not a candidate) |
| get_command#3 | names of the powershell cmdlets | select_object#1 (not a candidate) |
| get_help#2 | what does gci do | file-type (not a candidate) |
| avg_by#1 | average pay by department | average-field (not a candidate) |
| convert_unit#2 | 5 miles in km | count-files (not a candidate) |
| convert_unit#3 | 90 minutes in hours | count-files (not a candidate) |
| present_value#1 | present value of a future amount | net_present_value#1 |
| cumulative_max#1 | running maximum | largest-files (not a candidate) |
| cumulative_min#1 | running minimum | count-processes (not a candidate) |
| standardize#1 | z-scores of the values | unique-values (not a candidate) |
| normalize_path#1 | clean up the path /a//b/../c | slots: `"/a//b/../c" \| normalize_path → "/../c" \| normalize_path` |
| with_extension#1 | change the extension of c.txt to .md | slots: `" \| with_extension(".md") → " \| with_extension("c.txt")` |
| semver_compare#2 | is 1.2.3 older than 2.0.0 | oldest-files (not a candidate) |
| lookup#1 | find the row where name is ada | sqlite-query (not a candidate) |
| compare_object#1 | compare two lists, what was added and removed | deep_diff#1 |
| after_first#1 | domain of the email user@example.com | is_domain#1 (not a candidate) |
| surround#1 | wrap the text in brackets | wrap_text#1 (not a candidate) |
| days_between#1 | days between 2026-01-01 and 2026-01-10 | slots: `"2026-01-01" \| days_between("2 → days_between("2026-01-10")` |
| age_in_years#1 | how old is someone born 2000-06-15 | count-items (not a candidate) |
| has_path#1 | does it have the key a | rename_keys#1 (not a candidate) |
| del_path#1 | delete the field a.b | object-keys (not a candidate) |
| invoke_sqlite_command#2 | delete user 42 from app.db | sqlite-tables |
| get_sqlite_schema#2 | schema of every table in app.db | sqlite-tables |
| get_sqlite_schema#3 | primary key of users in app.db | object-keys (not a candidate) |
| out_sqlite#1 | save the file listing to files.db | invoke_sqlite_query#1 (not a candidate) |
| get_censys_context#2 | censys organization id | get_censys_organization#1 |
| get_censys_host#2 | what censys saw on 1.1.1.1 at the start of 2026 | get_censys_host_timeline#1 |
| search_censys#2 | censys hosts in chile | get_censys_aggregate#1 |

155 of 682 examples missed.
