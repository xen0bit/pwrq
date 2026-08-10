/* The pwrq query editor.
 *
 * The page has one job: make a query, its output and its shape visible at the
 * same time, and let you hand all three to someone else with a link. Everything
 * runs in the tab - the engine is pwrq itself, compiled to WebAssembly, in a
 * worker - so nothing typed here is uploaded anywhere.
 */

import { DiagramView, download } from "./diagram.js";
import { Editor } from "./editor.js";
import { Engine, isNoise } from "./engine.js";
import { escapeHTML, tokenizeJSON, toHTML } from "./highlight.js";
import { CommandPalette } from "./palette.js";
import { decodeHash, encodeState } from "./share.js";
import * as store from "./store.js";

const $ = (id) => document.getElementById(id);

const DEBOUNCE = { validate: 120, run: 220, diagram: 400, share: 500 };
const MAX_RENDERED_RESULTS = 300;

const state = {
    settings: store.loadSettings(),
    catalog: null,
    args: [],
    lastRun: null,
    lastDiagramQuery: null,
    diagramStale: true,
    d2Source: "",
    valid: true,
    ownHash: "",
    renderLimit: MAX_RENDERED_RESULTS,
};

const engine = new Engine("worker.js");
let queryEditor;
let inputEditor;
let diagram;
let palette;

function boot() {
    applyTheme(state.settings.theme);
    buildEditors();
    buildDiagram();
    buildPalette();
    wireEngine();
    wireControls();
    wireSplitters();
    wireShortcuts();
    applySettingsToControls();
    restoreState();
    renderShortcutHelp();
}

/* ------------------------------------------------------------------ theme */

function applyTheme(choice) {
    const root = document.documentElement;
    if (choice === "system") root.removeAttribute("data-theme");
    else root.setAttribute("data-theme", choice);
}

/* effectiveTheme is what the diagram has to be drawn in: the renderer needs a
 * concrete answer, and "system" is not one. */
function effectiveTheme() {
    if (state.settings.theme !== "system") return state.settings.theme;
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => {
    if (state.settings.theme === "system") {
        state.diagramStale = true;
        scheduleDiagram();
    }
});

/* ---------------------------------------------------------------- editors */

function buildEditors() {
    queryEditor = new Editor($("query-editor"), {
        language: "query",
        placeholder: 'e.g.  [.[] | select(.Size > 1000) | {Name, Size}] | sort_by(.Size)',
        completions: completionItems,
    });

    inputEditor = new Editor($("input-editor"), {
        language: "json",
        placeholder: '{"items": [{"Name": "b"}, {"Name": "a"}]}\n\nLeave empty to run against null. Several JSON values in a row are several inputs.',
    });

    queryEditor.addEventListener("change", () => {
        scheduleValidate();
        scheduleShare();
        updateCursor();
    });
    queryEditor.addEventListener("caret", updateCursor);

    inputEditor.addEventListener("change", () => {
        checkInput();
        scheduleRun();
        scheduleShare();
    });
}

function updateCursor() {
    const { line, column } = queryEditor.caretPosition();
    $("cursor-position").textContent = `${line}:${column}`;
}

/* completionItems is the vocabulary the editor offers. It is built from the
 * catalog the engine reports, so the page can only ever suggest a name it can
 * actually run. */
function completionItems() {
    const catalog = state.catalog;
    if (!catalog) return [];
    if (catalog.completions) return catalog.completions;

    const byName = new Map();
    for (const command of catalog.commands || []) {
        byName.set(command.name, command);
    }

    const items = [];
    for (const name of catalog.cmdlets || []) {
        const command = byName.get(name);
        items.push({
            name,
            kind: "cmdlet",
            detail: command?.description || "",
            insert: command && command.minArgs > 0 ? `${name}()` : name,
            search: (command?.description || "").toLowerCase(),
        });
    }
    for (const alias of catalog.aliases || []) {
        items.push({
            name: alias.name,
            kind: "alias",
            detail: `→ ${alias.target}`,
            insert: alias.name,
            search: alias.target,
        });
    }
    for (const name of catalog.builtins || []) {
        if (byName.has(name)) continue;
        items.push({ name, kind: "jq", detail: "", insert: name, search: "" });
    }
    for (const word of ["def", "as", "if", "then", "elif", "else", "end", "reduce", "foreach", "try", "catch", "label"]) {
        items.push({ name: word, kind: "keyword", detail: "", insert: word, search: "" });
    }

    catalog.completions = items;
    return items;
}

/* ---------------------------------------------------------------- diagram */

function buildDiagram() {
    diagram = new DiagramView($("diagram-stage"), $("diagram-canvas"), {
        onZoom: (scale) => {
            $("zoom-level").textContent = `${Math.round(scale * 100)}%`;
        },
    });
}

/* ----------------------------------------------------------------- engine */

function wireEngine() {
    engine.addEventListener("state", (event) => {
        const { state: engineState, detail } = event.detail;
        const dot = $("engine-dot");
        const text = $("engine-text");
        dot.className = `dot ${engineState}`;

        switch (engineState) {
            case "loading":
                text.textContent = "loading engine…";
                break;
            case "ready":
                text.textContent = "ready";
                $("cancel").classList.add("hidden");
                break;
            case "busy":
                text.textContent = "running…";
                $("cancel").classList.remove("hidden");
                break;
            case "error":
                text.textContent = detail || "the engine failed";
                toast(detail || "the engine failed", true);
                break;
        }
        if (engine.version) $("version").textContent = `pwrq ${engine.version}`;
    });

    engine.addEventListener("progress", (event) => {
        const { loaded, total } = event.detail;
        if (engine.state !== "loading") return;
        const megabytes = (loaded / 1048576).toFixed(1);
        $("engine-text").textContent = total
            ? `loading engine… ${Math.round((loaded / total) * 100)}%`
            : `loading engine… ${megabytes} MB`;
    });

    // The catalog is the first thing asked for: completion, highlighting, the
    // legend and the gallery all wait on it.
    engine
        .call("catalog", {})
        .then((catalog) => {
            state.catalog = catalog;
            const vocabulary = {
                cmdlets: new Set(catalog.cmdlets || []),
                builtins: new Set([...(catalog.builtins || []), ...(catalog.aliases || []).map((a) => a.name)]),
            };
            queryEditor.setVocabulary(vocabulary);
            renderCatalog();
            renderExamples();
            renderLegend();
            if (catalog.version) $("version").textContent = `pwrq ${catalog.version}`;
            // Anything typed while the engine was loading runs now.
            scheduleValidate(0);
        })
        .catch((err) => {
            if (!isNoise(err)) toast(`could not read the catalog: ${err.message}`, true);
        });
}

/* -------------------------------------------------------------- scheduling
 *
 * Three jobs run at different rhythms: validation is nearly free and should
 * feel instant, a run is cheap but not free, and a diagram is a layout engine
 * and costs real time. Each has its own debounce, and each supersedes its own
 * previous request rather than queueing behind it.
 */

const timers = {};

function schedule(name, delay, fn) {
    clearTimeout(timers[name]);
    timers[name] = setTimeout(fn, delay);
}

function scheduleValidate(delay = DEBOUNCE.validate) {
    schedule("validate", delay, validateNow);
}

function scheduleRun(delay = DEBOUNCE.run) {
    if (!state.settings.autoRun) return;
    schedule("run", delay, () => runNow(false));
}

function scheduleDiagram(delay = DEBOUNCE.diagram) {
    schedule("diagram", delay, drawNow);
}

function scheduleShare(delay = DEBOUNCE.share) {
    schedule("share", delay, updateHash);
}

async function validateNow() {
    const query = queryEditor.value;
    if (!query.trim()) {
        state.valid = true;
        queryEditor.setError(null);
        $("query-error").textContent = "";
        $("query-status").textContent = "empty";
        clearOutput("Write a query to see what it produces.");
        diagram.clear();
        $("diagram-placeholder").classList.remove("hidden");
        return;
    }

    try {
        const result = await engine.call("validate", { query }, { supersede: "validate", watchdogMs: 10000 });
        state.valid = Boolean(result.ok);

        if (result.ok) {
            queryEditor.setError(null);
            $("query-error").textContent = "";
            $("query-status").textContent = "valid";
            state.diagramStale = true;
            scheduleRun();
            scheduleDiagram();
        } else {
            queryEditor.setError(result.start !== result.end ? { start: result.start, end: result.end } : null);
            $("query-error").textContent = result.line
                ? `line ${result.line}, column ${result.column}: ${result.error}`
                : result.error;
            $("query-status").textContent = "syntax error";
            setBanner("warn", "The query does not parse; the results below are from the last version that did.");
        }
    } catch (err) {
        if (!isNoise(err)) $("query-error").textContent = err.message;
    }
}

async function runNow(manual) {
    const query = queryEditor.value;
    if (!query.trim()) return;
    if (!state.valid && !manual) return;

    const request = {
        query,
        input: inputEditor.value,
        slurp: state.settings.slurp,
        nullInput: state.settings.nullInput,
        raw: state.settings.output === "raw",
        compact: state.settings.output === "compact",
        indent: 2,
        limit: Number(state.settings.limit) || 1000,
        timeoutMs: Number(state.settings.timeout) || 5000,
        args: state.args.filter((arg) => arg.name.trim()),
    };

    try {
        const result = await engine.call("run", request, {
            supersede: "run",
            watchdogMs: request.timeoutMs + 15000,
        });
        state.lastRun = result;
        state.renderLimit = MAX_RENDERED_RESULTS;
        renderResults(result);
        recordRun(request);
    } catch (err) {
        if (isNoise(err)) return;
        setBanner("error", err.message);
    }
}

async function drawNow() {
    const query = queryEditor.value;
    if (!query.trim() || !state.valid) return;

    // Laying out a diagram is the most expensive thing the page does, so it is
    // only done when the diagram is actually on screen. Switching to the tab
    // draws whatever is pending.
    if (state.settings.tab !== "diagram") return;

    const request = {
        query,
        theme: effectiveTheme(),
        layout: state.settings.layout,
        direction: state.settings.direction,
        sketch: state.settings.sketch,
        d2: true,
    };

    try {
        const result = await engine.call("diagram", request, { supersede: "diagram", watchdogMs: 30000 });
        if (result.error) {
            $("diagram-placeholder").textContent = result.error;
            $("diagram-placeholder").classList.remove("hidden");
            return;
        }
        $("diagram-placeholder").classList.add("hidden");
        const sameQuery = state.lastDiagramQuery === query;
        diagram.show(result.svg, { keepView: sameQuery });
        state.lastDiagramQuery = query;
        state.d2Source = result.script || "";
        state.diagramStale = false;
    } catch (err) {
        if (!isNoise(err)) toast(err.message, true);
    }
}

/* ----------------------------------------------------------------- output */

function renderResults(result) {
    const output = $("output");

    if (result.error) {
        const kind = result.kind === "limit" || result.kind === "halt" ? "warn" : "error";
        setBanner(kind, result.error);
    } else {
        clearBanner();
    }

    const stats = [];
    if (result.count) stats.push(`${result.count} result${result.count === 1 ? "" : "s"}`);
    if (result.inputCount > 1) stats.push(`${result.inputCount} inputs`);
    if (typeof result.elapsedMs === "number") stats.push(`${formatMs(result.elapsedMs)}`);
    $("run-stats").textContent = stats.join(" · ");

    const values = result.values || [];
    if (values.length === 0) {
        clearOutput(result.error ? "" : "The query produced no output.");
        return;
    }

    const shown = values.slice(0, state.renderLimit);
    const raw = state.settings.output === "raw";

    // Results are the user's data: they are rendered as text, tokenised for
    // colour, never as markup.
    output.innerHTML = shown
        .map((value, index) => {
            const body = raw ? escapeHTML(value) : toHTML(value, tokenizeJSON(value));
            return `<div class="result"><span class="result-index">${index + 1}</span><pre class="result-value">${body}</pre></div>`;
        })
        .join("");

    if (values.length > shown.length) {
        const more = document.createElement("button");
        more.className = "btn small more-results";
        more.textContent = `Show ${Math.min(values.length - shown.length, MAX_RENDERED_RESULTS)} more of ${values.length}`;
        more.addEventListener("click", () => {
            state.renderLimit += MAX_RENDERED_RESULTS;
            renderResults(result);
        });
        output.appendChild(more);
    }
}

function clearOutput(message) {
    $("output").innerHTML = message ? `<p class="placeholder">${escapeHTML(message)}</p>` : "";
    $("run-stats").textContent = "";
}

function setBanner(kind, message) {
    const banner = $("output-banner");
    banner.className = `banner ${kind}`;
    banner.textContent = message;
    banner.classList.remove("hidden");
}

function clearBanner() {
    $("output-banner").classList.add("hidden");
    $("output-banner").textContent = "";
}

function formatMs(ms) {
    if (ms < 1) return "<1 ms";
    if (ms < 1000) return `${ms.toFixed(ms < 10 ? 1 : 0)} ms`;
    return `${(ms / 1000).toFixed(2)} s`;
}

/* checkInput reports malformed sample data before the engine has to. */
function checkInput() {
    const text = inputEditor.value.trim();
    const status = $("input-status");
    if (!text) {
        status.textContent = "null";
        inputEditor.setError(null);
        return;
    }
    try {
        // The input pane accepts a stream of values, which JSON.parse does not,
        // so a single failure is only reported when the whole text fails as one
        // value and as a stream.
        JSON.parse(text);
        status.textContent = "JSON";
        inputEditor.setError(null);
    } catch (err) {
        const values = countJSONValues(text);
        if (values > 0) {
            status.textContent = `${values} values`;
            inputEditor.setError(null);
        } else {
            status.textContent = "not JSON";
        }
    }
}

/* countJSONValues counts whole JSON values in a stream by scanning for
 * balanced brackets outside strings. It is a check for the status line, not a
 * parser: the engine is the authority on whether the input is readable. */
function countJSONValues(text) {
    let depth = 0;
    let count = 0;
    let inString = false;
    let escaped = false;
    let sawValue = false;

    for (const char of text) {
        if (inString) {
            if (escaped) escaped = false;
            else if (char === "\\") escaped = true;
            else if (char === '"') inString = false;
            continue;
        }
        if (char === '"') {
            inString = true;
            sawValue = true;
            continue;
        }
        if (char === "{" || char === "[") {
            depth++;
            sawValue = true;
            continue;
        }
        if (char === "}" || char === "]") {
            depth--;
            if (depth === 0) {
                count++;
                sawValue = false;
            }
            continue;
        }
        if (depth === 0 && /\s/.test(char) && sawValue) {
            count++;
            sawValue = false;
        } else if (depth === 0 && !/\s/.test(char)) {
            sawValue = true;
        }
    }
    if (sawValue && depth === 0) count++;
    return depth === 0 ? count : 0;
}

/* -------------------------------------------------------------- catalogue */

function renderCatalog(filter = "") {
    const container = $("catalog");
    const commands = state.catalog?.commands || [];
    const needle = filter.trim().toLowerCase();
    const hereOnly = $("catalog-here-only").checked;

    const matching = commands.filter((command) => {
        if (hereOnly && !command.available) return false;
        if (!needle) return true;
        return (
            command.name.includes(needle) ||
            (command.description || "").toLowerCase().includes(needle) ||
            (command.aliases || []).some((alias) => alias.includes(needle)) ||
            (command.category || "").toLowerCase().includes(needle)
        );
    });

    const available = commands.filter((command) => command.available).length;
    $("catalog-count").textContent = `${matching.length} of ${commands.length} (${available} run here)`;

    if (matching.length === 0) {
        container.innerHTML = `<p class="empty-note">Nothing matches “${escapeHTML(filter)}”.</p>`;
        return;
    }

    const categories = new Map();
    for (const command of matching) {
        const category = command.category || "Other";
        if (!categories.has(category)) categories.set(category, []);
        categories.get(category).push(command);
    }

    container.innerHTML = [...categories.entries()]
        .sort(([a], [b]) => a.localeCompare(b))
        .map(
            ([category, entries]) => `
        <h3 class="category">${escapeHTML(category)}</h3>
        ${entries.map(commandCard).join("")}`,
        )
        .join("");

    for (const node of container.querySelectorAll("[data-insert]")) {
        node.addEventListener("click", () => {
            queryEditor.insert(node.dataset.insert);
            queryEditor.focus();
        });
    }
    for (const node of container.querySelectorAll("[data-run]")) {
        node.addEventListener("click", () => {
            queryEditor.value = node.dataset.run;
            queryEditor.emit("change");
            selectTab("output");
            runNow(true);
        });
    }
}

function commandCard(command) {
    const arity = command.maxArgs === command.minArgs ? `${command.minArgs}` : `${command.minArgs}–${command.maxArgs}`;
    const aliases = (command.aliases || []).length ? `<span class="card-aliases">${command.aliases.map(escapeHTML).join(", ")}</span>` : "";
    const availability = command.available
        ? ""
        : `<span class="badge unavail" title="This cmdlet needs a filesystem, process table, service manager or the network, which a browser tab does not have. It runs in the pwrq CLI.">not in the browser</span>`;

    return `
    <article class="card">
        <div class="card-head">
            <span class="card-name">${escapeHTML(command.name)}</span>
            <span class="card-arity">${arity} arg${arity === "0" ? "" : "s"}</span>
            ${availability}
            ${aliases}
            <span class="card-actions">
                <button type="button" class="btn small" data-insert="${escapeHTML(command.name)}">Insert</button>
            </span>
        </div>
        <p class="card-desc">${escapeHTML(command.description || "")}</p>
        ${
            (command.examples || []).length
                ? `<div class="card-examples">${command.examples
                      .map((example) => `<button type="button" class="example-line" data-run="${escapeHTML(example)}">${escapeHTML(example)}</button>`)
                      .join("")}</div>`
                : ""
        }
    </article>`;
}

/* --------------------------------------------------------------- examples */

function renderExamples(filter = "") {
    const container = $("examples");
    const examples = state.catalog?.examples || [];
    const needle = filter.trim().toLowerCase();

    const matching = examples.filter(
        (example) =>
            !needle ||
            example.title.toLowerCase().includes(needle) ||
            example.description.toLowerCase().includes(needle) ||
            example.query.toLowerCase().includes(needle) ||
            example.category.toLowerCase().includes(needle),
    );

    if (matching.length === 0) {
        container.innerHTML = `<p class="empty-note">No example matches that.</p>`;
        return;
    }

    container.innerHTML = matching
        .map(
            (example, index) => `
        <article class="card">
            <div class="card-head">
                <span class="card-title">${escapeHTML(example.title)}</span>
                <span class="card-meta">${escapeHTML(example.category)}</span>
                <span class="card-actions">
                    <button type="button" class="btn small" data-example="${index}">Open</button>
                </span>
            </div>
            <p class="card-desc">${escapeHTML(example.description)}</p>
            <pre class="card-query">${escapeHTML(example.query)}</pre>
        </article>`,
        )
        .join("");

    for (const node of container.querySelectorAll("[data-example]")) {
        node.addEventListener("click", () => loadExample(matching[Number(node.dataset.example)]));
    }
}

function loadExample(example) {
    if (!example) return;
    queryEditor.value = example.query;
    inputEditor.value = example.input || "";
    state.args = (example.args || []).map((arg) => ({ ...arg }));
    renderArgs();
    checkInput();
    selectTab("output");
    scheduleValidate(0);
    scheduleShare(0);
    toast(`loaded “${example.title}”`);
}

/* ---------------------------------------------------------------- history */

function renderHistory() {
    const container = $("history");
    const history = store.loadHistory();
    const snippets = store.loadSnippets();

    const parts = [];

    if (snippets.length) {
        parts.push(`<h3 class="category">Saved</h3>`);
        parts.push(
            snippets
                .map(
                    (snippet, index) => `
            <article class="card">
                <div class="card-head">
                    <span class="card-title">${escapeHTML(snippet.name)}</span>
                    <span class="card-meta">${formatDate(snippet.at)}</span>
                    <span class="card-actions">
                        <button type="button" class="btn small" data-snippet="${index}">Open</button>
                        <button type="button" class="btn small danger" data-delete="${escapeHTML(snippet.name)}">Delete</button>
                    </span>
                </div>
                <pre class="card-query">${escapeHTML(snippet.query)}</pre>
            </article>`,
                )
                .join(""),
        );
    }

    parts.push(`<h3 class="category">Recent</h3>`);
    if (history.length === 0) {
        parts.push(`<p class="empty-note">Queries you run show up here.</p>`);
    } else {
        parts.push(
            history
                .map(
                    (entry, index) => `
            <article class="card">
                <div class="card-head">
                    <span class="card-meta">${formatDate(entry.at)}</span>
                    <span class="card-actions">
                        <button type="button" class="btn small" data-history="${index}">Open</button>
                    </span>
                </div>
                <pre class="card-query">${escapeHTML(entry.query)}</pre>
            </article>`,
                )
                .join(""),
        );
    }

    container.innerHTML = parts.join("");

    for (const node of container.querySelectorAll("[data-history]")) {
        node.addEventListener("click", () => restoreEntry(history[Number(node.dataset.history)]));
    }
    for (const node of container.querySelectorAll("[data-snippet]")) {
        node.addEventListener("click", () => restoreEntry(snippets[Number(node.dataset.snippet)]));
    }
    for (const node of container.querySelectorAll("[data-delete]")) {
        node.addEventListener("click", () => {
            store.deleteSnippet(node.dataset.delete);
            renderHistory();
        });
    }
}

function restoreEntry(entry) {
    if (!entry) return;
    queryEditor.value = entry.query || "";
    inputEditor.value = entry.input || "";
    state.args = (entry.args || []).map((arg) => ({ ...arg }));
    renderArgs();
    checkInput();
    selectTab("output");
    scheduleValidate(0);
    scheduleShare(0);
}

function recordRun(request) {
    store.recordHistory({ query: request.query, input: request.input, args: request.args });
    store.saveSession({ query: request.query, input: request.input, args: request.args });
    if (state.settings.tab === "history") renderHistory();
}

function formatDate(timestamp) {
    if (!timestamp) return "";
    const date = new Date(timestamp);
    return date.toLocaleString(undefined, { dateStyle: "short", timeStyle: "short" });
}

/* ------------------------------------------------------------------- args */

function renderArgs() {
    const list = $("args-list");
    list.innerHTML = "";

    state.args.forEach((arg, index) => {
        const row = document.createElement("div");
        row.className = "arg-row";

        const name = document.createElement("input");
        name.type = "text";
        name.className = "arg-name";
        name.placeholder = "name";
        name.value = arg.name;
        name.setAttribute("aria-label", `Argument ${index + 1} name`);

        const value = document.createElement("input");
        value.type = "text";
        value.className = "arg-value";
        value.placeholder = '{"any": "JSON"}';
        value.value = arg.value;
        value.setAttribute("aria-label", `Argument ${index + 1} value`);

        const remove = document.createElement("button");
        remove.type = "button";
        remove.className = "btn small";
        remove.textContent = "×";
        remove.title = "Remove this argument";

        name.addEventListener("input", () => {
            arg.name = name.value;
            argsChanged();
        });
        value.addEventListener("input", () => {
            arg.value = value.value;
            value.classList.toggle("arg-error", Boolean(value.value.trim()) && !isJSON(value.value));
            argsChanged();
        });
        remove.addEventListener("click", () => {
            state.args.splice(index, 1);
            renderArgs();
            argsChanged();
        });

        row.append(name, value, remove);
        list.append(row);
    });

    const count = $("args-count");
    count.textContent = String(state.args.length);
    count.hidden = state.args.length === 0;
    if (state.args.length) $("args-block").open = true;
}

function argsChanged() {
    scheduleRun();
    scheduleShare();
}

function isJSON(text) {
    try {
        JSON.parse(text);
        return true;
    } catch {
        return false;
    }
}

/* --------------------------------------------------------------- controls */

function wireControls() {
    $("run").addEventListener("click", () => runNow(true));
    $("cancel").addEventListener("click", () => {
        if (engine.cancel()) {
            toast("stopped; the engine restarted");
            setBanner("warn", "The run was stopped.");
        }
    });

    $("auto-run").addEventListener("change", (event) => {
        setSetting("autoRun", event.target.checked);
        if (event.target.checked) scheduleRun(0);
    });

    $("format").addEventListener("click", formatQuery);
    $("share").addEventListener("click", shareLink);
    $("palette-open").addEventListener("click", () => openPalette());
    $("help-open").addEventListener("click", () => toggleHelp(true));
    $("help-close").addEventListener("click", () => toggleHelp(false));
    $("help").addEventListener("mousedown", (event) => {
        if (event.target === $("help")) toggleHelp(false);
    });

    $("theme").addEventListener("change", (event) => {
        setSetting("theme", event.target.value);
        applyTheme(event.target.value);
        state.diagramStale = true;
        scheduleDiagram(0);
    });

    $("copy-query").addEventListener("click", () => copyText(queryEditor.value, "query copied"));
    $("clear-query").addEventListener("click", () => {
        queryEditor.value = "";
        queryEditor.emit("change");
        queryEditor.focus();
    });
    $("clear-input").addEventListener("click", () => {
        inputEditor.value = "";
        inputEditor.emit("change");
    });
    $("format-input").addEventListener("click", formatInput);
    $("load-file").addEventListener("click", () => $("file-input").click());
    $("file-input").addEventListener("change", async (event) => {
        const file = event.target.files?.[0];
        if (!file) return;
        inputEditor.value = await file.text();
        inputEditor.emit("change");
        event.target.value = "";
    });

    $("add-arg").addEventListener("click", () => {
        state.args.push({ name: "", value: "" });
        renderArgs();
        $("args-list").querySelector(".arg-name:last-of-type")?.focus();
    });

    for (const chip of document.querySelectorAll("[data-output]")) {
        chip.addEventListener("click", () => {
            setSetting("output", chip.dataset.output);
            syncOutputChips();
            runNow(true);
        });
    }

    $("slurp").addEventListener("change", (event) => {
        setSetting("slurp", event.target.checked);
        runNow(true);
    });
    $("null-input").addEventListener("change", (event) => {
        setSetting("nullInput", event.target.checked);
        runNow(true);
    });
    $("limit").addEventListener("change", (event) => {
        setSetting("limit", clampNumber(event.target, 1, 1000000, 1000));
        runNow(true);
    });
    $("timeout").addEventListener("change", (event) => {
        setSetting("timeout", clampNumber(event.target, 100, 120000, 5000));
    });

    $("copy-output").addEventListener("click", () => {
        const values = state.lastRun?.values || [];
        if (!values.length) {
            toast("there is no output to copy", true);
            return;
        }
        copyText(values.join("\n"), `${values.length} result${values.length === 1 ? "" : "s"} copied`);
    });
    $("download-output").addEventListener("click", () => {
        const values = state.lastRun?.values || [];
        if (!values.length) {
            toast("there is no output to download", true);
            return;
        }
        download("pwrq-output.json", values.join("\n") + "\n", "application/json");
    });

    $("zoom-in").addEventListener("click", () => diagram.zoomBy(1.25));
    $("zoom-out").addEventListener("click", () => diagram.zoomBy(0.8));
    $("zoom-fit").addEventListener("click", () => diagram.fit());
    $("direction").addEventListener("change", (event) => {
        setSetting("direction", event.target.value);
        redrawDiagram();
    });
    $("layout").addEventListener("change", (event) => {
        setSetting("layout", event.target.value);
        redrawDiagram();
    });
    $("sketch").addEventListener("change", (event) => {
        setSetting("sketch", event.target.checked);
        redrawDiagram();
    });
    $("show-legend").addEventListener("change", (event) => {
        setSetting("legend", event.target.checked);
        $("legend").classList.toggle("hidden", !event.target.checked);
    });
    $("download-svg").addEventListener("click", () => {
        if (!diagram.svg) {
            toast("there is no diagram to download", true);
            return;
        }
        download("pwrq-diagram.svg", diagram.svg, "image/svg+xml");
    });
    $("copy-d2").addEventListener("click", () => {
        if (!state.d2Source) {
            toast("there is no diagram source yet", true);
            return;
        }
        copyText(state.d2Source, "D2 source copied");
    });

    $("catalog-search").addEventListener("input", (event) => renderCatalog(event.target.value));
    $("catalog-here-only").addEventListener("change", () => renderCatalog($("catalog-search").value));
    $("examples-search").addEventListener("input", (event) => renderExamples(event.target.value));

    $("save-snippet").addEventListener("click", () => {
        const name = prompt("Name this query:", suggestName());
        if (!name) return;
        store.saveSnippet({ name, query: queryEditor.value, input: inputEditor.value, args: state.args });
        renderHistory();
        toast(`saved “${name}”`);
    });
    $("export-snippets").addEventListener("click", () => {
        download("pwrq-snippets.json", store.exportSnippets(), "application/json");
    });
    $("import-snippets").addEventListener("click", () => $("import-file").click());
    $("import-file").addEventListener("change", async (event) => {
        const file = event.target.files?.[0];
        if (!file) return;
        try {
            const added = store.importSnippets(await file.text());
            renderHistory();
            toast(`imported ${added} snippet${added === 1 ? "" : "s"}`);
        } catch (err) {
            toast(err.message, true);
        }
        event.target.value = "";
    });
    $("clear-history").addEventListener("click", () => {
        store.clearHistory();
        renderHistory();
    });

    for (const tab of document.querySelectorAll(".tab")) {
        tab.addEventListener("click", () => selectTab(tab.dataset.tab));
    }

    window.addEventListener("hashchange", () => {
        if (location.hash.slice(1) === state.ownHash) return;
        restoreFromHash();
    });
}

function redrawDiagram() {
    state.diagramStale = true;
    state.lastDiagramQuery = null; // a new layout deserves a fresh fit
    scheduleDiagram(0);
}

function clampNumber(input, min, max, fallback) {
    const value = Number(input.value);
    if (!Number.isFinite(value)) {
        input.value = fallback;
        return fallback;
    }
    const clamped = Math.min(Math.max(value, min), max);
    input.value = clamped;
    return clamped;
}

function setSetting(key, value) {
    state.settings[key] = value;
    store.saveSettings(state.settings);
}

function applySettingsToControls() {
    const s = state.settings;
    $("theme").value = s.theme;
    $("auto-run").checked = s.autoRun;
    $("slurp").checked = s.slurp;
    $("null-input").checked = s.nullInput;
    $("limit").value = s.limit;
    $("timeout").value = s.timeout;
    $("direction").value = s.direction;
    $("layout").value = s.layout;
    $("sketch").checked = s.sketch;
    $("show-legend").checked = s.legend;
    $("legend").classList.toggle("hidden", !s.legend);
    syncOutputChips();
    selectTab(s.tab, { remember: false });

    document.documentElement.style.setProperty("--editor-width", `${s.editorWidth}%`);
    document.documentElement.style.setProperty("--query-height", `${s.queryHeight}%`);
}

function syncOutputChips() {
    for (const chip of document.querySelectorAll("[data-output]")) {
        chip.setAttribute("aria-pressed", String(chip.dataset.output === state.settings.output));
    }
}

function selectTab(name, { remember = true } = {}) {
    for (const tab of document.querySelectorAll(".tab")) {
        tab.setAttribute("aria-selected", String(tab.dataset.tab === name));
    }
    for (const panel of document.querySelectorAll(".panel")) {
        panel.classList.toggle("hidden", panel.dataset.tab !== name);
    }
    if (remember) setSetting("tab", name);
    else state.settings.tab = name;

    if (name === "history") renderHistory();
    if (name === "diagram") {
        // The diagram is only laid out while it is visible, so arriving here
        // is what triggers a pending draw.
        if (state.diagramStale) scheduleDiagram(0);
        else diagram.fit();
    }
}

async function formatQuery() {
    const query = queryEditor.value;
    if (!query.trim()) return;
    try {
        const result = await engine.call("format", { query });
        if (result.error) {
            toast(`cannot format: ${result.error}`, true);
            return;
        }
        if (result.query === query) {
            toast("already tidy");
            return;
        }
        queryEditor.value = result.query;
        queryEditor.emit("change");
    } catch (err) {
        if (!isNoise(err)) toast(err.message, true);
    }
}

function formatInput() {
    const text = inputEditor.value.trim();
    if (!text) return;
    try {
        inputEditor.value = JSON.stringify(JSON.parse(text), null, 2);
        inputEditor.emit("change");
    } catch {
        toast("the input is not a single JSON value, so it was left alone", true);
    }
}

function suggestName() {
    const first = queryEditor.value.trim().split("\n")[0];
    return first.length > 40 ? `${first.slice(0, 40)}…` : first;
}

/* -------------------------------------------------------------- splitters */

function wireSplitters() {
    dragSplitter($("column-splitter"), (event) => {
        const workspace = $("workspace");
        const rect = workspace.getBoundingClientRect();
        const narrow = window.matchMedia("(max-width: 900px)").matches;
        const percent = narrow
            ? ((event.clientY - rect.top) / rect.height) * 100
            : ((event.clientX - rect.left) / rect.width) * 100;
        const clamped = Math.min(Math.max(percent, 20), 80);
        document.documentElement.style.setProperty("--editor-width", `${clamped}%`);
        return () => setSetting("editorWidth", clamped);
    });

    dragSplitter($("editor-splitter"), (event) => {
        const column = $("editor-column");
        const rect = column.getBoundingClientRect();
        const percent = ((event.clientY - rect.top) / rect.height) * 100;
        const clamped = Math.min(Math.max(percent, 15), 85);
        document.documentElement.style.setProperty("--query-height", `${clamped}%`);
        return () => setSetting("queryHeight", clamped);
    });
}

function dragSplitter(handle, onMove) {
    let commit = null;

    handle.addEventListener("pointerdown", (event) => {
        event.preventDefault();
        handle.setPointerCapture(event.pointerId);
        handle.classList.add("dragging");
        commit = onMove(event);
    });
    handle.addEventListener("pointermove", (event) => {
        if (!handle.classList.contains("dragging")) return;
        commit = onMove(event);
    });
    const stop = () => {
        if (!handle.classList.contains("dragging")) return;
        handle.classList.remove("dragging");
        commit?.();
        commit = null;
        // The editors size themselves to their container, which just changed.
        queryEditor.render();
        inputEditor.render();
    };
    handle.addEventListener("pointerup", stop);
    handle.addEventListener("pointercancel", stop);

    // Keyboard users get the same control; a splitter that only responds to a
    // mouse is a layout you cannot change without one.
    handle.addEventListener("keydown", (event) => {
        const horizontal = handle.classList.contains("vertical");
        const key = event.key;
        const step = event.shiftKey ? 5 : 2;
        const property = horizontal ? "--editor-width" : "--query-height";
        const setting = horizontal ? "editorWidth" : "queryHeight";
        const forward = horizontal ? key === "ArrowRight" : key === "ArrowDown";
        const back = horizontal ? key === "ArrowLeft" : key === "ArrowUp";
        if (!forward && !back) return;
        event.preventDefault();
        const current = state.settings[setting];
        const next = Math.min(Math.max(current + (forward ? step : -step), 15), 85);
        document.documentElement.style.setProperty(property, `${next}%`);
        setSetting(setting, next);
        queryEditor.render();
        inputEditor.render();
    });
}

/* -------------------------------------------------------------- shortcuts */

const SHORTCUTS = [
    ["Ctrl/⌘ + Enter", "Run the query"],
    ["Ctrl/⌘ + K", "Command palette"],
    ["Ctrl/⌘ + Shift + F", "Tidy the query"],
    ["Ctrl/⌘ + Shift + I", "Tidy the input JSON"],
    ["Ctrl/⌘ + S", "Copy a share link"],
    ["Ctrl/⌘ + /", "Comment or uncomment the selected lines"],
    ["Ctrl + Space", "Suggest a cmdlet"],
    ["Ctrl/⌘ + 1…5", "Switch to a tab"],
    ["Escape", "Close whatever is open"],
    ["?", "This list"],
];

function renderShortcutHelp() {
    $("shortcuts").innerHTML = SHORTCUTS.map(
        ([keys, what]) => `<dt><kbd>${escapeHTML(keys)}</kbd></dt><dd>${escapeHTML(what)}</dd>`,
    ).join("");
}

function wireShortcuts() {
    const TABS = ["output", "diagram", "catalog", "examples", "history"];

    document.addEventListener("keydown", (event) => {
        const meta = event.ctrlKey || event.metaKey;
        const inEditor = event.target instanceof HTMLTextAreaElement || event.target instanceof HTMLInputElement;

        if (event.key === "Escape") {
            if (palette.isOpen) palette.close();
            else if (!$("help").classList.contains("hidden")) toggleHelp(false);
            else if (engine.busy) $("cancel").click();
            return;
        }

        if (meta && event.key === "Enter") {
            event.preventDefault();
            runNow(true);
            return;
        }
        if (meta && event.key.toLowerCase() === "k") {
            event.preventDefault();
            openPalette();
            return;
        }
        if (meta && event.shiftKey && event.key.toLowerCase() === "f") {
            event.preventDefault();
            formatQuery();
            return;
        }
        if (meta && event.shiftKey && event.key.toLowerCase() === "i") {
            event.preventDefault();
            formatInput();
            return;
        }
        if (meta && event.key.toLowerCase() === "s") {
            event.preventDefault();
            shareLink();
            return;
        }
        if (meta && /^[1-5]$/.test(event.key)) {
            event.preventDefault();
            selectTab(TABS[Number(event.key) - 1]);
            return;
        }
        if (event.key === "?" && !inEditor) {
            event.preventDefault();
            toggleHelp(true);
        }
    });
}

function toggleHelp(open) {
    $("help").classList.toggle("hidden", !open);
    if (open) $("help-close").focus();
}

function buildPalette() {
    palette = new CommandPalette($("palette"), $("palette-input"), $("palette-list"), {
        source: paletteItems,
    });
}

function openPalette() {
    palette.previousFocus = document.activeElement;
    palette.open();
}

/* paletteItems is everything reachable by name: the page's own actions first,
 * because those are what a palette is for, then the vocabulary and the
 * gallery. */
function paletteItems() {
    const items = [
        { kind: "action", title: "Run the query", run: () => runNow(true) },
        { kind: "action", title: "Tidy the query", run: formatQuery },
        { kind: "action", title: "Tidy the input JSON", run: formatInput },
        { kind: "action", title: "Copy a share link", run: shareLink },
        { kind: "action", title: "Copy the output", run: () => $("copy-output").click() },
        { kind: "action", title: "Download the diagram as SVG", run: () => $("download-svg").click() },
        { kind: "action", title: "Copy the diagram's D2 source", run: () => $("copy-d2").click() },
        { kind: "action", title: "Save this query", run: () => $("save-snippet").click() },
        { kind: "action", title: "Clear the query", run: () => $("clear-query").click() },
        { kind: "action", title: "Switch theme: dark", run: () => setTheme("dark") },
        { kind: "action", title: "Switch theme: light", run: () => setTheme("light") },
        { kind: "action", title: "Switch theme: follow the system", run: () => setTheme("system") },
        { kind: "action", title: "Keyboard shortcuts", run: () => toggleHelp(true) },
    ];

    for (const tab of ["output", "diagram", "catalog", "examples", "history"]) {
        items.push({ kind: "tab", title: `Show ${tab}`, run: () => selectTab(tab) });
    }

    for (const example of state.catalog?.examples || []) {
        items.push({
            kind: "example",
            title: example.title,
            detail: example.category,
            run: () => loadExample(example),
        });
    }

    for (const command of state.catalog?.commands || []) {
        items.push({
            kind: command.available ? "cmdlet" : "cmdlet (CLI)",
            title: command.name,
            detail: command.description,
            run: () => {
                queryEditor.insert(command.name);
                queryEditor.focus();
            },
        });
    }

    return items;
}

function setTheme(theme) {
    setSetting("theme", theme);
    $("theme").value = theme;
    applyTheme(theme);
    redrawDiagram();
}

/* ------------------------------------------------------------------ share */

function currentState() {
    return {
        query: queryEditor.value,
        input: inputEditor.value,
        args: state.args.filter((arg) => arg.name.trim()),
        options: {
            output: state.settings.output,
            slurp: state.settings.slurp,
            nullInput: state.settings.nullInput,
            direction: state.settings.direction,
            layout: state.settings.layout,
        },
    };
}

async function updateHash() {
    const encoded = await encodeState(currentState());
    state.ownHash = encoded;
    // replaceState rather than assigning location.hash: this happens as you
    // type, and every assignment would be a history entry to back out of.
    history.replaceState(null, "", `${location.pathname}${location.search}#${encoded}`);
}

async function shareLink() {
    await updateHash();
    const url = location.href;
    const copied = await copyText(url, "link copied — it carries the query and the input");
    if (!copied) prompt("Copy this link:", url);
}

async function restoreFromHash() {
    const shared = await decodeHash(location.hash);
    if (!shared) return false;

    queryEditor.value = shared.query;
    inputEditor.value = shared.input;
    state.args = shared.args || [];
    renderArgs();

    for (const [key, value] of Object.entries(shared.options || {})) {
        if (key in store.DEFAULT_SETTINGS) state.settings[key] = value;
    }
    applySettingsToControls();
    checkInput();
    scheduleValidate(0);
    return true;
}

async function restoreState() {
    if (await restoreFromHash()) {
        toast("opened from a shared link");
        return;
    }

    const session = store.loadSession();
    if (session?.query) {
        queryEditor.value = session.query;
        inputEditor.value = session.input || "";
        state.args = session.args || [];
        renderArgs();
        checkInput();
        scheduleValidate(0);
        return;
    }

    // A first visit should show the thing working, not an empty box.
    queryEditor.value = '[.[] | select(.Size > 1000) | {Name, Hash: (.Name | sha256)}]\n| sort_by(.Name)';
    inputEditor.value = '[{"Name":"notes.txt","Size":812},\n {"Name":"report.pdf","Size":48211},\n {"Name":"image.png","Size":10240}]';
    renderArgs();
    checkInput();
    scheduleValidate(0);
}

/* ----------------------------------------------------------------- legend */

function renderLegend() {
    const classes = state.catalog?.classes || [];
    const theme = effectiveTheme();
    $("legend").innerHTML = classes
        .map((entry) => {
            const style = theme === "light" ? entry.light : entry.dark;
            return `<span class="legend-item" title="${escapeHTML(entry.description)}">
                <span class="legend-swatch" style="background:${escapeHTML(style.fill)};border-color:${escapeHTML(style.stroke)}"></span>
                ${escapeHTML(entry.label)}
            </span>`;
        })
        .join("");
}

/* ----------------------------------------------------------------- pieces */

async function copyText(text, message) {
    try {
        await navigator.clipboard.writeText(text);
        if (message) toast(message);
        return true;
    } catch {
        // Clipboard access needs a secure context and permission; when it is
        // refused, fall back to the old selection trick.
        try {
            const area = document.createElement("textarea");
            area.value = text;
            area.style.position = "fixed";
            area.style.opacity = "0";
            document.body.appendChild(area);
            area.select();
            const ok = document.execCommand("copy");
            area.remove();
            if (ok && message) toast(message);
            return ok;
        } catch {
            toast("this browser would not let the page copy anything", true);
            return false;
        }
    }
}

function toast(message, isError = false) {
    const node = document.createElement("div");
    node.className = `toast${isError ? " error" : ""}`;
    node.textContent = message;
    $("toasts").appendChild(node);
    setTimeout(() => node.remove(), isError ? 6000 : 3000);
}

// Sizes are read from the layout, so the editors have to be told when it
// changes for a reason other than a splitter drag.
window.addEventListener("resize", () => {
    queryEditor?.render();
    inputEditor?.render();
});

// Last, not first: boot reaches for constants declared throughout this module,
// and a module's bindings only exist once its body has run.
boot();
