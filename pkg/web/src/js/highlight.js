/* Tokenisers for the two languages the page edits: jq (as pwrq extends it) and
 * JSON.
 *
 * They produce tokens rather than HTML, so the same output drives colouring,
 * the "what am I inside" test that decides whether to offer a completion, and
 * the error underline. Everything is escaped at the point it becomes HTML,
 * once, in `toHTML`.
 */

const KEYWORDS = new Set([
    "def", "as", "if", "then", "elif", "else", "end", "reduce", "foreach",
    "try", "catch", "label", "import", "include", "and", "or", "not",
    "__loc__",
]);

const LITERALS = new Set(["true", "false", "null"]);

/* tokenizeQuery walks jq source once, left to right.
 *
 * It is a lexer, not a parser: it knows what a token *is*, not what it means.
 * That is enough for colour, and it means a half-written query - which is what
 * an editor mostly contains - still highlights sensibly. */
export function tokenizeQuery(src, vocabulary = {}) {
    const cmdlets = vocabulary.cmdlets || new Set();
    const builtins = vocabulary.builtins || new Set();
    const tokens = [];
    let i = 0;

    const push = (kind, start, end) => tokens.push({ kind, start, end, text: src.slice(start, end) });

    while (i < src.length) {
        const c = src[i];

        // comment
        if (c === "#") {
            const start = i;
            while (i < src.length && src[i] !== "\n") i++;
            push("comment", start, i);
            continue;
        }

        // string, including \( ... ) interpolation, which is coloured as code
        if (c === '"') {
            const start = i;
            i++;
            let segment = start;
            while (i < src.length) {
                if (src[i] === "\\" && src[i + 1] === "(") {
                    push("string", segment, i);
                    const open = i;
                    i += 2;
                    let depth = 1;
                    while (i < src.length && depth > 0) {
                        if (src[i] === "(") depth++;
                        else if (src[i] === ")") depth--;
                        else if (src[i] === '"') {
                            // a nested string inside the interpolation
                            i++;
                            while (i < src.length && src[i] !== '"') {
                                if (src[i] === "\\") i++;
                                i++;
                            }
                        }
                        i++;
                    }
                    push("interp", open, i);
                    segment = i;
                    continue;
                }
                if (src[i] === "\\") {
                    i += 2;
                    continue;
                }
                if (src[i] === '"') {
                    i++;
                    break;
                }
                i++;
            }
            push("string", segment, i);
            continue;
        }

        // @format
        if (c === "@") {
            const start = i;
            i++;
            while (i < src.length && /[A-Za-z0-9_]/.test(src[i])) i++;
            push("format", start, i);
            continue;
        }

        // $variable
        if (c === "$") {
            const start = i;
            i++;
            while (i < src.length && /[A-Za-z0-9_]/.test(src[i])) i++;
            push("variable", start, i);
            continue;
        }

        // number
        if (/[0-9]/.test(c) || (c === "." && /[0-9]/.test(src[i + 1] || ""))) {
            const start = i;
            while (i < src.length && /[0-9._eE+-]/.test(src[i])) {
                // stop at a '-' or '+' that is not an exponent sign
                if ((src[i] === "+" || src[i] === "-") && !/[eE]/.test(src[i - 1] || "")) break;
                if (src[i] === "." && !/[0-9]/.test(src[i + 1] || "")) break;
                i++;
            }
            push("number", start, i);
            continue;
        }

        // .field, .["key"], .. and a bare .
        if (c === ".") {
            const start = i;
            i++;
            if (src[i] === ".") {
                i++;
                push("field", start, i);
                continue;
            }
            while (i < src.length && /[A-Za-z0-9_]/.test(src[i])) i++;
            push("field", start, i);
            continue;
        }

        // identifier: keyword, cmdlet, builtin or unknown
        if (/[A-Za-z_]/.test(c)) {
            const start = i;
            while (i < src.length && /[A-Za-z0-9_:]/.test(src[i])) i++;
            const word = src.slice(start, i);
            let kind = "ident";
            if (KEYWORDS.has(word)) kind = "keyword";
            else if (LITERALS.has(word)) kind = "literal";
            else if (cmdlets.has(word)) kind = "cmdlet";
            else if (builtins.has(word)) kind = "builtin";
            push(kind, start, i);
            continue;
        }

        if (/\s/.test(c)) {
            const start = i;
            while (i < src.length && /\s/.test(src[i])) i++;
            push("space", start, i);
            continue;
        }

        const start = i;
        i++;
        push("punct", start, i);
    }

    return tokens;
}

/* tokenizeJSON colours JSON, including the multi-value streams the input pane
 * accepts. Malformed text still tokenises: it is what a half-typed document
 * looks like. */
export function tokenizeJSON(src) {
    const tokens = [];
    let i = 0;
    const push = (kind, start, end) => tokens.push({ kind, start, end, text: src.slice(start, end) });

    while (i < src.length) {
        const c = src[i];

        if (c === '"') {
            const start = i;
            i++;
            while (i < src.length) {
                if (src[i] === "\\") { i += 2; continue; }
                if (src[i] === '"') { i++; break; }
                i++;
            }
            // A string followed by a colon is a key, which is worth its own
            // colour: it is the shape of the data rather than the data.
            let j = i;
            while (j < src.length && /\s/.test(src[j])) j++;
            push(src[j] === ":" ? "key" : "string", start, i);
            continue;
        }

        if (/[-0-9]/.test(c)) {
            const start = i;
            i++;
            while (i < src.length && /[0-9.eE+-]/.test(src[i])) i++;
            push("number", start, i);
            continue;
        }

        if (/[A-Za-z]/.test(c)) {
            const start = i;
            while (i < src.length && /[A-Za-z]/.test(src[i])) i++;
            const word = src.slice(start, i);
            push(LITERALS.has(word) ? "literal" : "ident", start, i);
            continue;
        }

        if (/\s/.test(c)) {
            const start = i;
            while (i < src.length && /\s/.test(src[i])) i++;
            push("space", start, i);
            continue;
        }

        const start = i;
        i++;
        push("punct", start, i);
    }

    return tokens;
}

const CLASS_FOR = {
    comment: "t-comment",
    string: "t-string",
    interp: "t-punct",
    number: "t-number",
    keyword: "t-keyword",
    literal: "t-literal",
    cmdlet: "t-cmdlet",
    builtin: "t-builtin",
    variable: "t-variable",
    field: "t-field",
    format: "t-format",
    key: "t-key",
    punct: "t-punct",
    ident: "",
    space: "",
};

export function escapeHTML(text) {
    return text
        .replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;");
}

/* toHTML renders tokens, optionally marking a span as the error.
 *
 * The mark is applied by splitting tokens at its boundaries rather than by
 * wrapping them, so an error inside a string still underlines exactly the
 * offending characters. */
export function toHTML(src, tokens, error = null) {
    let html = "";
    const inError = (pos) => error && pos >= error.start && pos < error.end;

    for (const token of tokens) {
        const cls = CLASS_FOR[token.kind] ?? "";
        if (!error || token.end <= error.start || token.start >= error.end) {
            html += span(escapeHTML(token.text), cls);
            continue;
        }
        // The token overlaps the error: emit it in up to three pieces.
        for (let pos = token.start; pos < token.end; ) {
            const marked = inError(pos);
            let end = pos;
            while (end < token.end && inError(end) === marked) end++;
            const piece = escapeHTML(src.slice(pos, end));
            html += span(piece, marked ? `${cls} t-error`.trim() : cls);
            pos = end;
        }
    }

    // A highlight layer that ends exactly at the text's end scrolls short of
    // the caret on the final line; a trailing newline keeps the two aligned.
    return html + "\n";
}

function span(text, cls) {
    if (!text) return "";
    return cls ? `<span class="${cls}">${text}</span>` : text;
}

/* wordAt reports the identifier being typed at a caret position, which is what
 * completion needs: its text, and where it starts, so accepting a completion
 * can replace it. */
export function wordAt(src, caret) {
    let start = caret;
    while (start > 0 && /[A-Za-z0-9_]/.test(src[start - 1])) start--;

    // A leading dot or dollar is part of what is being typed: `.Na` should
    // complete field-ish things and `$x` should not be mistaken for a call.
    let prefixChar = "";
    if (start > 0 && (src[start - 1] === "$" || src[start - 1] === "@")) {
        prefixChar = src[start - 1];
        start--;
    }

    return { text: src.slice(start, caret), start, end: caret, prefix: prefixChar };
}

/* inLiteral reports whether a caret sits inside a string or a comment, where
 * offering a function name would be wrong. */
export function inLiteral(tokens, caret) {
    for (const token of tokens) {
        if (caret > token.start && caret <= token.end) {
            return token.kind === "string" || token.kind === "comment";
        }
    }
    return false;
}
