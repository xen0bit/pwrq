/* A small code editor: a textarea with a highlighted layer behind it.
 *
 * The alternative was a contenteditable surface or a bundled editor library.
 * A textarea keeps every behaviour the browser already gets right - the caret,
 * selection, undo, IME, spellcheck, accessibility, mobile keyboards - and the
 * only thing it lacks is colour, which a <pre> underneath it supplies. The two
 * layers share the same font metrics and padding, so they stay aligned as
 * long as neither is allowed to wrap differently.
 */

import { escapeHTML, inLiteral, toHTML, tokenizeJSON, tokenizeQuery, wordAt } from "./highlight.js";

const CLOSERS = { "(": ")", "[": "]", "{": "}", '"': '"' };

// Everything that decides where a character lands. The hidden mirror copies
// these from the textarea so that measuring text in the mirror answers
// questions about the textarea.
const MIRRORED_STYLES = [
    "fontFamily", "fontSize", "fontWeight", "lineHeight", "letterSpacing",
    "paddingTop", "paddingRight", "paddingBottom", "paddingLeft",
    "whiteSpace", "overflowWrap", "wordBreak", "tabSize",
];

export class Editor extends EventTarget {
    /*
     * host        - the element to fill
     * language    - "query" or "json"
     * placeholder - shown when empty
     * completions - () => [{name, kind, detail, insert}], consulted as you type
     */
    constructor(host, { language = "query", placeholder = "", completions = null } = {}) {
        super();
        this.host = host;
        this.language = language;
        this.vocabulary = { cmdlets: new Set(), builtins: new Set() };
        this.completionSource = completions;
        this.error = null;
        this.tokens = [];

        host.innerHTML = "";
        host.classList.add("editor-host");

        this.root = el("div", "editor");
        this.gutter = el("div", "editor-gutter");
        this.scroll = el("div", "editor-scroll");
        this.highlight = el("pre", "editor-highlight");
        this.highlightCode = el("code");
        this.highlight.appendChild(this.highlightCode);

        this.input = document.createElement("textarea");
        this.input.className = "editor-input";
        this.input.spellcheck = false;
        this.input.autocapitalize = "off";
        this.input.autocomplete = "off";
        this.input.setAttribute("aria-label", language === "json" ? "Input JSON" : "Query");
        if (placeholder) this.input.placeholder = placeholder;

        this.scroll.append(this.highlight, this.input);
        this.root.append(this.gutter, this.scroll);
        host.append(this.root);

        this.completionBox = el("div", "completions hidden");
        this.completionBox.setAttribute("role", "listbox");
        host.append(this.completionBox);
        this.completionState = null;

        this.mirror = el("div", "editor-mirror");
        host.append(this.mirror);

        this.bind();
        this.render();
    }

    bind() {
        this.input.addEventListener("input", () => {
            this.render();
            this.emit("change");
            this.maybeComplete();
        });

        this.input.addEventListener("scroll", () => this.syncScroll());
        this.scroll.addEventListener("scroll", () => this.syncScroll());

        this.input.addEventListener("keydown", (event) => this.onKeydown(event));
        this.input.addEventListener("blur", () => this.closeCompletions());
        this.input.addEventListener("click", () => {
            this.closeCompletions();
            this.emit("caret");
        });
        this.input.addEventListener("keyup", (event) => {
            if (event.key.startsWith("Arrow") || event.key === "Home" || event.key === "End") {
                this.emit("caret");
            }
        });

        // Dropping a file on an editor is the obvious way to load one, and
        // the browser's default - navigating away - is never what was meant.
        this.input.addEventListener("dragover", (event) => {
            if (event.dataTransfer?.types?.includes("Files")) event.preventDefault();
        });
        this.input.addEventListener("drop", (event) => {
            const file = event.dataTransfer?.files?.[0];
            if (!file) return;
            event.preventDefault();
            file.text().then((text) => {
                this.value = text;
                this.emit("change");
            });
        });
    }

    emit(type, detail = {}) {
        this.dispatchEvent(new CustomEvent(type, { detail }));
    }

    get value() {
        return this.input.value;
    }

    set value(text) {
        if (this.input.value === text) return;
        this.input.value = text;
        this.render();
    }

    setVocabulary(vocabulary) {
        this.vocabulary = vocabulary;
        this.render();
    }

    /* setError marks a span of the source. Passing null clears it. */
    setError(error) {
        const same =
            (!error && !this.error) ||
            (error && this.error && error.start === this.error.start && error.end === this.error.end);
        this.error = error;
        if (!same) this.render();
    }

    focus() {
        this.input.focus();
    }

    get caret() {
        return this.input.selectionStart;
    }

    caretPosition() {
        const upTo = this.input.value.slice(0, this.caret);
        const line = upTo.split("\n").length;
        const column = upTo.length - upTo.lastIndexOf("\n");
        return { line, column };
    }

    /* insert replaces the selection, leaving the caret after what was put in.
     * execCommand is deprecated but is the only way to write to a textarea
     * without destroying the browser's undo stack, so it is tried first. */
    insert(text) {
        this.input.focus();
        if (!document.execCommand || !document.execCommand("insertText", false, text)) {
            const { selectionStart: start, selectionEnd: end, value } = this.input;
            this.input.value = value.slice(0, start) + text + value.slice(end);
            this.input.selectionStart = this.input.selectionEnd = start + text.length;
        }
        this.render();
        this.emit("change");
    }

    replaceRange(start, end, text) {
        this.input.focus();
        this.input.setSelectionRange(start, end);
        this.insert(text);
    }

    render() {
        const src = this.input.value;
        this.tokens = this.language === "json" ? tokenizeJSON(src) : tokenizeQuery(src, this.vocabulary);
        this.highlightCode.innerHTML = src ? toHTML(src, this.tokens, this.error) : "";
        this.renderGutter(src);
        this.syncHeight();
    }

    renderGutter(src) {
        const lines = src.split("\n");

        // A wrapped line occupies more than one row, so a naive 1..n gutter
        // drifts out of step with the text as soon as one line is long - which
        // in a narrow pane is most of them. Each logical line is measured, and
        // its number is followed by blank rows for the rest of what it covers.
        //
        // Measuring costs a layout pass per line, so a very long document
        // falls back to plain numbering rather than making every keystroke
        // expensive; at that length the pane is scrolling anyway.
        if (lines.length > 400) {
            this.gutter.textContent = lines.map((_, index) => index + 1).join("\n");
            return;
        }

        const rowHeight = this.rowHeight();
        const rows = [];
        for (let i = 0; i < lines.length; i++) {
            rows.push(String(i + 1));
            for (let extra = this.wrappedRows(lines[i], rowHeight); extra > 1; extra--) {
                rows.push("");
            }
        }
        this.gutter.textContent = rows.join("\n");
    }

    rowHeight() {
        const height = parseFloat(getComputedStyle(this.input).lineHeight);
        return Number.isFinite(height) && height > 0 ? height : 18;
    }

    /* wrappedRows reports how many rows a logical line occupies, by laying it
     * out in the hidden mirror at the textarea's own width. */
    wrappedRows(line, rowHeight) {
        if (line === "") return 1;

        const style = getComputedStyle(this.input);
        const padding = parseFloat(style.paddingLeft) + parseFloat(style.paddingRight);
        const width = this.input.clientWidth - padding;
        if (!(width > 0)) return 1;

        // Most lines are plainly short enough to fit, and for those the
        // measurement can be skipped: the font is monospaced, so the width of
        // a line is its length times the width of a character.
        if (line.length * this.averageCharWidth(style) < width) return 1;

        this.syncMirrorStyle(style);
        this.mirror.style.padding = "0";
        this.mirror.style.width = `${width}px`;
        this.mirror.textContent = line;

        return Math.max(1, Math.round(this.mirror.offsetHeight / rowHeight));
    }

    averageCharWidth(style) {
        // The font is monospaced, so one measurement holds for the whole
        // document until the font changes.
        if (this.charWidth && this.charWidthFont === style.font) return this.charWidth;

        this.syncMirrorStyle(style);
        this.mirror.style.padding = "0";
        this.mirror.style.width = "auto";
        this.mirror.textContent = "0".repeat(100);
        this.charWidth = Math.max(this.mirror.offsetWidth / 100, 1);
        this.charWidthFont = style.font;
        return this.charWidth;
    }

    syncMirrorStyle(style) {
        for (const property of MIRRORED_STYLES) {
            this.mirror.style[property] = style[property];
        }
    }

    syncHeight() {
        // The textarea drives the scroll container's height so that the
        // highlight layer, which is absolutely positioned, covers all of it.
        this.input.style.height = "auto";
        const needed = Math.max(this.input.scrollHeight, this.scroll.clientHeight);
        this.input.style.height = `${needed}px`;
        this.highlight.style.height = `${needed}px`;
        this.syncScroll();
    }

    syncScroll() {
        this.gutter.scrollTop = this.scroll.scrollTop;
        this.gutter.style.transform = `translateY(${-this.scroll.scrollTop}px)`;
    }

    onKeydown(event) {
        if (this.completionState && this.handleCompletionKey(event)) return;

        const { selectionStart: start, selectionEnd: end, value } = this.input;

        if (event.key === "Tab" && !event.ctrlKey && !event.metaKey) {
            // Tab indents rather than leaving the editor. Shift+Tab outdents,
            // and Escape-then-Tab is the way out for keyboard users.
            event.preventDefault();
            if (event.shiftKey) {
                this.outdent();
            } else if (start !== end) {
                this.indentSelection();
            } else {
                this.insert("  ");
            }
            return;
        }

        if (event.key === "Enter" && !event.shiftKey && !event.ctrlKey && !event.metaKey) {
            // Keep the indentation of the line being left, and open a block
            // when the caret sits between a bracket pair.
            const lineStart = value.lastIndexOf("\n", start - 1) + 1;
            const indent = (value.slice(lineStart, start).match(/^[ \t]*/) || [""])[0];
            const before = value[start - 1];
            const after = value[start];
            if (before && after && CLOSERS[before] === after) {
                event.preventDefault();
                this.insert(`\n${indent}  \n${indent}`);
                this.input.selectionStart = this.input.selectionEnd = start + indent.length + 3;
                this.render();
                return;
            }
            if (indent) {
                event.preventDefault();
                this.insert(`\n${indent}`);
                return;
            }
            return;
        }

        if (CLOSERS[event.key] && !event.ctrlKey && !event.metaKey) {
            const closer = CLOSERS[event.key];
            if (start !== end) {
                // Wrap the selection rather than replacing it.
                event.preventDefault();
                const selected = value.slice(start, end);
                this.insert(event.key + selected + closer);
                this.input.selectionStart = start + 1;
                this.input.selectionEnd = start + 1 + selected.length;
                return;
            }
            // Do not auto-close a quote in the middle of a word, where it is
            // far more likely to be closing one already open.
            if (event.key === '"' && /[A-Za-z0-9_"]/.test(value[start] || "")) return;
            event.preventDefault();
            this.insert(event.key + closer);
            this.input.selectionStart = this.input.selectionEnd = start + 1;
            return;
        }

        if ((event.key === ")" || event.key === "]" || event.key === "}") && value[start] === event.key) {
            // Typing over an auto-inserted closer.
            event.preventDefault();
            this.input.selectionStart = this.input.selectionEnd = start + 1;
            return;
        }

        if (event.key === "Backspace" && start === end && start > 0) {
            const before = value[start - 1];
            if (CLOSERS[before] && value[start] === CLOSERS[before]) {
                event.preventDefault();
                this.replaceRange(start - 1, start + 1, "");
                return;
            }
        }

        if (event.key === "/" && (event.ctrlKey || event.metaKey) && this.language === "query") {
            event.preventDefault();
            this.toggleComment();
            return;
        }

        if (event.key === " " && event.ctrlKey) {
            event.preventDefault();
            this.maybeComplete(true);
        }
    }

    lineRange(start, end) {
        const value = this.input.value;
        const from = value.lastIndexOf("\n", start - 1) + 1;
        let to = value.indexOf("\n", end);
        if (to === -1) to = value.length;
        return { from, to };
    }

    indentSelection() {
        const { selectionStart: start, selectionEnd: end } = this.input;
        const { from, to } = this.lineRange(start, end);
        const block = this.input.value.slice(from, to);
        const indented = block.split("\n").map((line) => `  ${line}`).join("\n");
        this.replaceRange(from, to, indented);
        this.input.setSelectionRange(from, from + indented.length);
    }

    outdent() {
        const { selectionStart: start, selectionEnd: end } = this.input;
        const { from, to } = this.lineRange(start, end);
        const block = this.input.value.slice(from, to);
        const outdented = block.split("\n").map((line) => line.replace(/^ {1,2}|^\t/, "")).join("\n");
        if (outdented === block) return;
        this.replaceRange(from, to, outdented);
        this.input.setSelectionRange(from, from + outdented.length);
    }

    toggleComment() {
        const { selectionStart: start, selectionEnd: end } = this.input;
        const { from, to } = this.lineRange(start, end);
        const block = this.input.value.slice(from, to);
        const lines = block.split("\n");
        const commented = lines.every((line) => line.trim() === "" || line.trimStart().startsWith("#"));
        const next = lines
            .map((line) => {
                if (line.trim() === "") return line;
                if (commented) return line.replace(/^(\s*)#\s?/, "$1");
                const indent = (line.match(/^\s*/) || [""])[0];
                return `${indent}# ${line.slice(indent.length)}`;
            })
            .join("\n");
        this.replaceRange(from, to, next);
        this.input.setSelectionRange(from, from + next.length);
    }

    /* ------------------------------------------------------- completions */

    maybeComplete(force = false) {
        if (!this.completionSource) return;

        const src = this.input.value;
        const caret = this.caret;
        const word = wordAt(src, caret);

        if (!force && word.text.length < 2) {
            this.closeCompletions();
            return;
        }
        if (inLiteral(this.tokens, caret) && !force) {
            this.closeCompletions();
            return;
        }

        const query = word.text.replace(/^[$@]/, "").toLowerCase();
        const candidates = this.completionSource()
            .filter((item) => matches(item, query))
            .sort((a, b) => rank(a, query) - rank(b, query) || a.name.localeCompare(b.name))
            .slice(0, 40);

        if (candidates.length === 0) {
            this.closeCompletions();
            return;
        }

        this.completionState = { items: candidates, index: 0, word };
        this.renderCompletions();
    }

    renderCompletions() {
        const { items, index } = this.completionState;
        this.completionBox.innerHTML = items
            .map(
                (item, i) => `
                <div class="completion" role="option" data-index="${i}" aria-selected="${i === index}">
                    <span class="completion-name">${escapeHTML(item.name)}${
                    item.kind ? `<span class="kind">${escapeHTML(item.kind)}</span>` : ""
                }</span>
                    ${item.detail ? `<span class="completion-desc">${escapeHTML(item.detail)}</span>` : ""}
                </div>`,
            )
            .join("");
        this.completionBox.classList.remove("hidden");
        this.positionCompletions();

        for (const node of this.completionBox.querySelectorAll(".completion")) {
            // mousedown, not click: the editor loses focus on click, and the
            // blur handler would close the list before the click landed.
            node.addEventListener("mousedown", (event) => {
                event.preventDefault();
                this.completionState.index = Number(node.dataset.index);
                this.acceptCompletion();
            });
        }
        this.scrollSelectionIntoView();
    }

    positionCompletions() {
        const { word } = this.completionState;
        const coords = this.caretCoordinates(word.start);
        const hostHeight = this.host.clientHeight;
        const below = hostHeight - coords.top - coords.height;

        this.completionBox.style.left = `${Math.min(coords.left, this.host.clientWidth - 280)}px`;
        if (below < 180 && coords.top > below) {
            this.completionBox.style.bottom = `${hostHeight - coords.top}px`;
            this.completionBox.style.top = "auto";
        } else {
            this.completionBox.style.top = `${coords.top + coords.height}px`;
            this.completionBox.style.bottom = "auto";
        }
    }

    /* caretCoordinates measures where an offset sits, by laying the same text
     * out in a hidden copy of the editor and asking the browser. There is no
     * API for this on a textarea, and the mirror is the standard answer. */
    caretCoordinates(offset) {
        const style = getComputedStyle(this.input);
        const mirror = this.mirror;
        this.syncMirrorStyle(style);
        mirror.style.width = `${this.input.clientWidth}px`;

        const value = this.input.value;
        mirror.textContent = value.slice(0, offset);
        const marker = document.createElement("span");
        marker.textContent = value.slice(offset) || ".";
        mirror.appendChild(marker);

        const rect = marker.getBoundingClientRect();
        const mirrorRect = mirror.getBoundingClientRect();

        // The mirror sits at the host's origin, but the text does not: the
        // gutter is to its left, and the scroll container moves under it. The
        // textarea's own position carries both, so the offset within the
        // mirror is measured against where the textarea actually is.
        const inputRect = this.input.getBoundingClientRect();
        const hostRect = this.host.getBoundingClientRect();

        return {
            left: rect.left - mirrorRect.left + (inputRect.left - hostRect.left),
            top: rect.top - mirrorRect.top + (inputRect.top - hostRect.top),
            height: parseFloat(style.lineHeight) || 18,
        };
    }

    handleCompletionKey(event) {
        const state = this.completionState;
        switch (event.key) {
            case "ArrowDown":
                event.preventDefault();
                state.index = (state.index + 1) % state.items.length;
                this.renderCompletions();
                return true;
            case "ArrowUp":
                event.preventDefault();
                state.index = (state.index - 1 + state.items.length) % state.items.length;
                this.renderCompletions();
                return true;
            case "Enter":
            case "Tab":
                event.preventDefault();
                this.acceptCompletion();
                return true;
            case "Escape":
                event.preventDefault();
                this.closeCompletions();
                return true;
            default:
                return false;
        }
    }

    acceptCompletion() {
        const { items, index, word } = this.completionState;
        const item = items[index];
        this.closeCompletions();

        const text = item.insert ?? item.name;
        this.replaceRange(word.start, word.end, text);

        // A call with arguments is more useful with the caret inside the
        // parentheses than after them.
        if (text.endsWith("()")) {
            const caret = word.start + text.length - 1;
            this.input.setSelectionRange(caret, caret);
        }
    }

    closeCompletions() {
        this.completionState = null;
        this.completionBox.classList.add("hidden");
        this.completionBox.innerHTML = "";
    }

    scrollSelectionIntoView() {
        const selected = this.completionBox.querySelector('[aria-selected="true"]');
        selected?.scrollIntoView({ block: "nearest" });
    }
}

function matches(item, query) {
    if (!query) return true;
    return item.name.toLowerCase().includes(query) || (item.search || "").includes(query);
}

/* rank puts a prefix match above a match in the middle of a name, and a cmdlet
 * above a jq builtin: this is pwrq's page, and its own vocabulary is the part
 * a visitor cannot guess. */
function rank(item, query) {
    const name = item.name.toLowerCase();
    let score = name.startsWith(query) ? 0 : 10;
    if (item.kind === "cmdlet") score -= 2;
    if (item.kind === "alias") score -= 1;
    return score + Math.min(name.length / 40, 1);
}

function el(tag, className = "") {
    const node = document.createElement(tag);
    if (className) node.className = className;
    return node;
}
