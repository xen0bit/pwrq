/* Tests for the highlighter.
 *
 * Run them with `bun test` from pkg/web/src.
 *
 * Colour is cosmetic, but two things here are not: the tokeniser decides where
 * completion may be offered, and toHTML is the one place query text and query
 * *output* become markup. A hole in the escaping would turn a shared link into
 * a way of running script in the reader's page.
 */

import { describe, expect, test } from "bun:test";
import { inLiteral, toHTML, tokenizeJSON, tokenizeQuery, wordAt } from "./highlight.js";

const vocabulary = { cmdlets: new Set(["sha256", "get_childitem"]), builtins: new Set(["select", "map"]) };

function kinds(src) {
    return tokenizeQuery(src, vocabulary)
        .filter((token) => token.kind !== "space")
        .map((token) => [token.kind, token.text]);
}

describe("the query tokeniser", () => {
    test("tells cmdlets from jq builtins", () => {
        expect(kinds(".a | sha256 | select(.b)")).toEqual([
            ["field", ".a"],
            ["punct", "|"],
            ["cmdlet", "sha256"],
            ["punct", "|"],
            ["builtin", "select"],
            ["punct", "("],
            ["field", ".b"],
            ["punct", ")"],
        ]);
    });

    test("keeps strings whole, including their escapes", () => {
        expect(kinds('"a \\"quoted\\" thing"')).toEqual([["string", '"a \\"quoted\\" thing"']]);
    });

    test("treats string interpolation as code, not text", () => {
        const found = kinds('"total: \\(.n + 1)"');
        expect(found.some(([kind]) => kind === "interp")).toBe(true);
    });

    test("recognises variables, formats, comments and numbers", () => {
        expect(kinds('$x @base64 1.5e3 # note')).toEqual([
            ["variable", "$x"],
            ["format", "@base64"],
            ["number", "1.5e3"],
            ["comment", "# note"],
        ]);
    });

    test("covers every character of the source", () => {
        // A token stream with a gap in it would drop text from the highlight
        // layer, and the layer is what sits behind the caret.
        for (const src of ['.a|sha256', '{"k": [1, 2]} # x', '"s" as $v | $v', "..", ".a?.b[]"]) {
            const tokens = tokenizeQuery(src, vocabulary);
            expect(tokens.map((token) => token.text).join("")).toBe(src);
            expect(tokens[0]?.start).toBe(0);
            expect(tokens.at(-1)?.end).toBe(src.length);
        }
    });
});

describe("the JSON tokeniser", () => {
    test("distinguishes keys from string values", () => {
        const tokens = tokenizeJSON('{"a": "b"}').filter((token) => token.kind !== "space");
        expect(tokens.map((token) => token.kind)).toEqual(["punct", "key", "punct", "string", "punct"]);
    });

    test("covers every character", () => {
        const src = '{"a":[1,true,null],"b":"x"} {"c":-2.5e10}';
        expect(tokenizeJSON(src).map((token) => token.text).join("")).toBe(src);
    });
});

describe("toHTML", () => {
    test("escapes markup in the source", () => {
        const src = '"<script>alert(1)</script>"';
        const html = toHTML(src, tokenizeQuery(src, vocabulary));

        expect(html).not.toContain("<script>");
        expect(html).toContain("&lt;script&gt;");
    });

    test("escapes markup in query output as well", () => {
        // Output is the user's data, and a shared link chooses the input it is
        // rendered from.
        const value = '{"payload":"<img src=x onerror=alert(1)>"}';
        const html = toHTML(value, tokenizeJSON(value));

        expect(html).not.toContain("<img");
        expect(html).toContain("&lt;img");
    });

    test("marks exactly the span it is given", () => {
        const src = ".a | (";
        const html = toHTML(src, tokenizeQuery(src, vocabulary), { start: 5, end: 6 });

        expect(html).toContain("t-error");
        // The error span covers one character, so only one opening tag carries
        // the class.
        expect(html.match(/t-error/g).length).toBe(1);
    });

    test("preserves the text it renders", () => {
        const src = '.a | select(.b > 1) # "note"';
        const text = toHTML(src, tokenizeQuery(src, vocabulary))
            .replace(/<[^>]*>/g, "")
            .replace(/&lt;/g, "<")
            .replace(/&gt;/g, ">")
            .replace(/&amp;/g, "&");

        expect(text).toBe(src + "\n");
    });
});

describe("completion context", () => {
    test("wordAt reports the identifier being typed", () => {
        expect(wordAt(".a | sha2", 9)).toEqual({ text: "sha2", start: 5, end: 9, prefix: "" });
    });

    test("wordAt keeps a leading dollar with the word", () => {
        expect(wordAt("$va", 3).text).toBe("$va");
    });

    test("inLiteral stops completion inside strings and comments", () => {
        const src = '"sha2" # sha2';
        const tokens = tokenizeQuery(src, vocabulary);

        expect(inLiteral(tokens, 5)).toBe(true); // inside the string
        expect(inLiteral(tokens, 13)).toBe(true); // inside the comment
        expect(inLiteral(tokens, 7)).toBe(false); // between them
    });
});
