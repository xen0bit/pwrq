/* The pwrq engine, off the main thread.
 *
 * Running the WASM module in the page would mean every query blocks rendering:
 * a slow diagram freezes the editor, and a runaway query freezes the tab with
 * no way out but closing it. Here, the page stays responsive whatever the
 * engine is doing, and a query that will not stop can be ended by terminating
 * the worker - the browser's only real kill switch.
 *
 * This file is served as-is rather than bundled, because it is loaded by URL
 * and pulls in wasm_exec.js with importScripts.
 */

importScripts("wasm_exec.js");

let ready = false;
let failure = null;
const pending = [];

// The Go side calls this as soon as its exports are in place.
self.pwrqReady = function (version) {
    ready = true;
    postMessage({ type: "ready", version: version || "" });
    for (const message of pending.splice(0)) {
        handle(message);
    }
};

async function boot() {
    const go = new Go();

    // Fetching by hand rather than with instantiateStreaming buys a progress
    // bar: the module is tens of megabytes, and a page that looks broken for
    // ten seconds is a page people close.
    const response = await fetch("web.wasm");
    if (!response.ok) {
        throw new Error(`fetching web.wasm: ${response.status} ${response.statusText}`);
    }

    const bytes = await readWithProgress(response);
    const { instance } = await WebAssembly.instantiate(bytes, go.importObject);

    // go.run resolves only when the Go program exits, which it never does:
    // main blocks so the exported functions stay callable.
    go.run(instance).catch((err) => {
        ready = false;
        failure = String(err);
        postMessage({ type: "failed", error: failure });
    });

    // pwrqReady normally fires during go.run. Polling covers the case where a
    // future runtime defers main, so readiness is never missed.
    for (let i = 0; i < 200 && !ready; i++) {
        if (typeof self.pwrqCall === "function") {
            self.pwrqReady(self.pwrqVersion || "");
            return;
        }
        await new Promise((resolve) => setTimeout(resolve, 25));
    }
    if (!ready) {
        throw new Error("the engine loaded but never became callable");
    }
}

// readWithProgress streams the module and reports how far it has got. The
// total is not always known - a server that gzips without a length header
// gives none - and the page shows an indeterminate state when it is missing.
async function readWithProgress(response) {
    const total = Number(response.headers.get("content-length")) || 0;

    if (!response.body || typeof response.body.getReader !== "function") {
        postMessage({ type: "progress", loaded: 0, total });
        return await response.arrayBuffer();
    }

    const reader = response.body.getReader();
    const chunks = [];
    let loaded = 0;
    let lastReport = 0;

    for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        chunks.push(value);
        loaded += value.length;
        // Reporting every chunk floods the page with messages it cannot use.
        if (loaded - lastReport > 262144) {
            lastReport = loaded;
            postMessage({ type: "progress", loaded, total });
        }
    }
    postMessage({ type: "progress", loaded, total: total || loaded });

    const bytes = new Uint8Array(loaded);
    let offset = 0;
    for (const chunk of chunks) {
        bytes.set(chunk, offset);
        offset += chunk.length;
    }
    return bytes.buffer;
}

function handle(message) {
    const { id, method, request } = message;
    if (failure) {
        postMessage({ id, ok: false, error: failure });
        return;
    }
    try {
        const raw = self.pwrqCall(method, request);
        postMessage({ id, ok: true, raw });
    } catch (err) {
        postMessage({ id, ok: false, error: err && err.message ? err.message : String(err) });
    }
}

self.onmessage = (event) => {
    const message = event.data;
    if (!message || typeof message !== "object") return;
    // A request that arrives while the module is still downloading waits for
    // it rather than failing: the page is usable from the first keystroke.
    if (!ready && !failure) {
        pending.push(message);
        return;
    }
    handle(message);
};

boot().catch((err) => {
    failure = err && err.message ? err.message : String(err);
    postMessage({ type: "failed", error: failure });
    for (const message of pending.splice(0)) {
        postMessage({ id: message.id, ok: false, error: failure });
    }
});
