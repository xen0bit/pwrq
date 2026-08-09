/* The page's side of the engine.
 *
 * Requests are promises; the worker answers them by id. Two things make this
 * more than a postMessage wrapper:
 *
 *   - a watchdog, because the Go-side deadline can only stop a query, not a
 *     wedged runtime or a layout engine that never returns. When it trips, the
 *     worker is terminated and a fresh one started, which is the only kill
 *     switch a browser actually has.
 *   - supersession, because the page asks on every keystroke and the answer to
 *     a query the user has already changed is worth nothing. A superseded
 *     request is dropped rather than rendered.
 */

export class Engine extends EventTarget {
    constructor(workerURL = "worker.js") {
        super();
        this.workerURL = workerURL;
        this.worker = null;
        this.nextID = 1;
        this.pending = new Map();
        this.state = "loading";
        this.version = "";
        this.progress = { loaded: 0, total: 0 };
        this.latest = new Map(); // method -> id of the newest request
        this.start();
    }

    start() {
        try {
            this.worker = new Worker(this.workerURL);
        } catch (err) {
            // Opening the page from a file:// URL, or in a context where
            // workers are blocked, fails here. Reporting it is all that can be
            // done - but the page has to stay up to report it.
            this.worker = null;
            this.setState("error", `the engine could not start: ${err.message}`);
            return;
        }
        this.worker.onmessage = (event) => this.receive(event.data);
        this.worker.onerror = (event) => {
            this.setState("error", event.message || "the engine failed to load");
        };
        this.setState("loading");
    }

    setState(state, detail = "") {
        this.state = state;
        this.dispatchEvent(new CustomEvent("state", { detail: { state, detail, version: this.version } }));
    }

    receive(message) {
        if (!message || typeof message !== "object") return;

        if (message.type === "ready") {
            this.version = message.version || "";
            this.setState(this.pending.size ? "busy" : "ready");
            return;
        }
        if (message.type === "progress") {
            this.progress = { loaded: message.loaded, total: message.total };
            this.dispatchEvent(new CustomEvent("progress", { detail: this.progress }));
            return;
        }
        if (message.type === "failed") {
            this.setState("error", message.error);
            this.rejectAll(new Error(message.error || "the engine failed"));
            return;
        }

        const entry = this.pending.get(message.id);
        if (!entry) return;
        this.pending.delete(message.id);
        clearTimeout(entry.watchdog);

        if (this.pending.size === 0 && this.state === "busy") {
            this.setState("ready");
        }

        if (!message.ok) {
            entry.reject(new Error(message.error || "the engine failed"));
            return;
        }
        try {
            entry.resolve(JSON.parse(message.raw));
        } catch (err) {
            entry.reject(new Error(`the engine returned something that is not JSON: ${err.message}`));
        }
    }

    /* call sends one request.
     *
     * `supersede` names a slot: a newer request in the same slot makes this one
     * irrelevant, and its promise rejects with a superseded error the caller is
     * expected to ignore. `watchdogMs` is the outer bound - always longer than
     * whatever deadline the request itself carries, because the engine
     * stopping a query cleanly is the good case.
     */
    call(method, payload = {}, { supersede = null, watchdogMs = 20000 } = {}) {
        if (!this.worker) {
            return Promise.reject(new Error("the engine is not running"));
        }

        const id = this.nextID++;
        if (supersede) this.latest.set(supersede, id);

        return new Promise((resolve, reject) => {
            const entry = {
                method,
                resolve: (value) => {
                    if (supersede && this.latest.get(supersede) !== id) {
                        reject(superseded());
                        return;
                    }
                    resolve(value);
                },
                reject,
                watchdog: setTimeout(() => this.timedOut(id), watchdogMs),
            };
            this.pending.set(id, entry);
            this.setState("busy");
            this.worker.postMessage({ id, method, request: JSON.stringify(payload) });
        });
    }

    timedOut(id) {
        const entry = this.pending.get(id);
        if (!entry) return;
        // The engine is not answering. Nothing short of terminating it will
        // get the page back, so do that and rebuild.
        this.restart(new Error("the engine stopped responding and was restarted"));
    }

    /* cancel ends whatever is running. There is no cooperative way to
     * interrupt a WASM module mid-instruction, so this is a hard reset: the
     * worker is terminated and a new one boots. The module is already in the
     * HTTP cache, so it comes back quickly. */
    cancel() {
        if (this.pending.size === 0) return false;
        this.restart(cancelled());
        return true;
    }

    restart(reason) {
        try {
            this.worker.terminate();
        } catch {
            /* already gone */
        }
        this.rejectAll(reason);
        this.start();
    }

    rejectAll(reason) {
        for (const [, entry] of this.pending) {
            clearTimeout(entry.watchdog);
            entry.reject(reason);
        }
        this.pending.clear();
    }

    get busy() {
        return this.pending.size > 0;
    }
}

export function superseded() {
    const err = new Error("superseded");
    err.superseded = true;
    return err;
}

export function cancelled() {
    const err = new Error("cancelled");
    err.cancelled = true;
    return err;
}

/* isNoise reports whether a rejection is one the page deliberately caused and
 * should not show to anyone. */
export function isNoise(err) {
    return Boolean(err && (err.superseded || err.cancelled));
}
