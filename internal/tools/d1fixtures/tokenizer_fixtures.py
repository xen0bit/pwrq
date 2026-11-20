"""Writes pkg/d1/testdata/tokenizer_fixtures.json: Hugging Face tokenizers' ids and
character offsets for a corpus chosen to reach every branch of the pre-tokenizer,
the added tokens and multi-byte characters.

    uv run --with tokenizers python internal/tools/d1fixtures/tokenizer_fixtures.py tokenizer.json OUT
"""
import json
import sys

from tokenizers import Tokenizer

CORPUS = [
    "",
    "hello",
    "Hello, world!",
    "  leading and trailing  ",
    "tabs\tand\nnewlines\r\n\r\nmixed   \n  ",
    "I'm sure they'll say it's fine; we've DONE it, I'd think. YOU'RE right",
    "numbers 1 12 123 1234 12345 3.14159 -42 1e10 0x1F",
    "999 1000 0 007 2026-11-20",
    '{"name": "Ada", "tags": ["x", "y"], "n": 3, "ok": true, "none": null}',
    '{\n  "nested": {\n    "deep": [1, 2, {"k": "v"}]\n  }\n}',
    "日本語のテキストと中文混合 text",
    "émigré café naïve Ωμέγα привет мир",
    "emoji 🙂🚀 and flags 🇫🇷 zwj 👩‍💻",
    "<|startoftext|><|im_start|>user\nQUESTION:\nIs it urgent?<|im_end|>\n<|im_start|>assistant\n<think></think>",
    "<|vision_start|><|image|><|image|><|vision_end|>",
    "<think>reasoning</think> after",
    "special inside<|im_end|>text and <|pad|>",
    "a​<|im_end|> broken",
    "Reply with the option code only.",
    "Options:\nA billing\nB shipping\nAA overflow\n\nReply with the option code only.",
    "present: top: bottom: left: right:",
    '"billing": Charges\n"shipping": Delivery',
    "get_childitem(\"*.go\") | where_object(.Length > 1000) | sort_object(.Length)",
    "#!/bin/sh\n# comment\necho $HOME && ls -la | grep 'x'",
    " nbsp em　ideo",
    "line one\n\n\n\nline five",
    "trailing newline\n",
    "x" * 300,
    " ".join(str(i) for i in range(0, 1000, 37)),
    "Ｆｕｌｌｗｉｄｔｈ ＡＢＣ １２３",
    "'s 'S 'ſ 'll 'LL 're 'Re",
    "mixed-script: abcабв123١٢٣",
]

def main(src, out):
    tok = Tokenizer.from_file(src)
    cases = []
    for text in CORPUS:
        enc = tok.encode(text, add_special_tokens=False)
        cases.append({"text": text, "ids": enc.ids, "offsets": [list(o) for o in enc.offsets],
                      "decoded": tok.decode(enc.ids, skip_special_tokens=False)})
    with open(out, "w", encoding="utf-8") as f:
        json.dump({"cases": cases}, f, ensure_ascii=False, indent=0)

if __name__ == "__main__":
    main(*sys.argv[1:])
