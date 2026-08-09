/* Everything the page remembers between visits.
 *
 * localStorage is not always there - private windows, storage disabled by
 * policy, quota exhausted - and a page that throws on load because it could not
 * save a preference is a broken page. Every access here is guarded, and the
 * fallback is simply not remembering.
 */

const PREFIX = "pwrq.ide.";
const HISTORY_LIMIT = 60;
const SNIPPET_LIMIT = 200;

function read(key, fallback) {
    try {
        const raw = localStorage.getItem(PREFIX + key);
        return raw === null ? fallback : JSON.parse(raw);
    } catch {
        return fallback;
    }
}

function write(key, value) {
    try {
        localStorage.setItem(PREFIX + key, JSON.stringify(value));
        return true;
    } catch {
        return false;
    }
}

export const DEFAULT_SETTINGS = {
    theme: "system",
    autoRun: true,
    output: "pretty",
    slurp: false,
    nullInput: false,
    limit: 1000,
    timeout: 5000,
    direction: "right",
    layout: "dagre",
    sketch: false,
    legend: true,
    tab: "output",
    editorWidth: 44,
    queryHeight: 55,
};

export function loadSettings() {
    const saved = read("settings", {});
    const settings = { ...DEFAULT_SETTINGS };
    for (const [key, value] of Object.entries(saved || {})) {
        if (key in DEFAULT_SETTINGS && typeof value === typeof DEFAULT_SETTINGS[key]) {
            settings[key] = value;
        }
    }
    return settings;
}

export function saveSettings(settings) {
    write("settings", settings);
}

export function loadSession() {
    const session = read("session", null);
    if (!session || typeof session !== "object") return null;
    return {
        query: typeof session.query === "string" ? session.query : "",
        input: typeof session.input === "string" ? session.input : "",
        args: Array.isArray(session.args) ? session.args : [],
    };
}

export function saveSession(session) {
    write("session", session);
}

/* History records what was run, not what was typed: a keystroke is not a
 * thought, and a list of half-written queries is noise. Consecutive duplicates
 * collapse for the same reason. */
export function loadHistory() {
    const history = read("history", []);
    return Array.isArray(history) ? history : [];
}

export function recordHistory(entry) {
    if (!entry.query || !entry.query.trim()) return loadHistory();
    const history = loadHistory().filter((item) => item.query !== entry.query);
    history.unshift({ ...entry, at: Date.now() });
    const trimmed = history.slice(0, HISTORY_LIMIT);
    write("history", trimmed);
    return trimmed;
}

export function clearHistory() {
    write("history", []);
    return [];
}

export function loadSnippets() {
    const snippets = read("snippets", []);
    return Array.isArray(snippets) ? snippets : [];
}

export function saveSnippet(snippet) {
    const snippets = loadSnippets().filter((item) => item.name !== snippet.name);
    snippets.unshift({ ...snippet, at: Date.now() });
    const trimmed = snippets.slice(0, SNIPPET_LIMIT);
    write("snippets", trimmed);
    return trimmed;
}

export function deleteSnippet(name) {
    const snippets = loadSnippets().filter((item) => item.name !== name);
    write("snippets", snippets);
    return snippets;
}

/* importSnippets merges a file someone exported. It is a file the user chose,
 * but the shapes in it are still checked: a malformed entry should be skipped,
 * not rendered as undefined. */
export function importSnippets(text) {
    let parsed;
    try {
        parsed = JSON.parse(text);
    } catch {
        throw new Error("that file is not JSON");
    }
    const incoming = Array.isArray(parsed) ? parsed : parsed?.snippets;
    if (!Array.isArray(incoming)) throw new Error("that file has no snippets in it");

    let added = 0;
    for (const entry of incoming) {
        if (!entry || typeof entry.name !== "string" || typeof entry.query !== "string") continue;
        saveSnippet({
            name: entry.name.slice(0, 120),
            query: entry.query,
            input: typeof entry.input === "string" ? entry.input : "",
            args: Array.isArray(entry.args) ? entry.args : [],
        });
        added++;
    }
    if (added === 0) throw new Error("that file had no usable snippets");
    return added;
}

export function exportSnippets() {
    return JSON.stringify({ tool: "pwrq", snippets: loadSnippets() }, null, 2);
}
