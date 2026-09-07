/* The native page's side of the engine.
 *
 * The same contract as engine.js (the WASM worker transport): requests are
 * promises answered by id, with supersession (a newer request in the same
 * slot drops the older one) and a watchdog (a request answered by nothing is
 * aborted). Two things differ, both because HTTP replaces postMessage:
 *
 *   - there is no worker to terminate. Cancelling or timing out aborts the
 *     fetch, and the server's handler sees the disconnect and cancels the
 *     running query. The engine itself never needs restarting.
 *   - authentication is a bearer token. A loopback server needs none; a gated
 *     one reads it from `?token=` once (stored for the tab, stripped from the
 *     URL so it is not shared with the link) or from the tab's storage.
 *
 * This file is only ever bundled into the native page. The WASM page imports
 * engine.js instead, which is what keeps a static deployment from ever
 * calling the server's API. See build-native.mjs and isolation.test.js.
 */

const TOKEN_PARAM = "token";
const TOKEN_KEY = "pwrq.native.token";

export class Engine extends EventTarget {
    constructor(apiURL = "api/call") {
        super();
        this.apiURL = apiURL;
        this.healthURL = apiURL.replace(/call$/, "health");
        this.nextID = 1;
        this.pending = new Map();
        this.state = "loading";
        this.version = "";
        this.health = null;
        this.latest = new Map(); // method -> id of the newest request
        this.adoptTokenFromURL();
        this.checkHealth();
    }

    /* adoptTokenFromURL keeps a bootstrap token out of shared links: it is
     * read once, kept for the tab, and removed from the address bar (leaving
     * the # fragment, which carries the query, untouched). */
    adoptTokenFromURL() {
        try {
            const url = new URL(window.location.href);
            const token = url.searchParams.get(TOKEN_PARAM);
            if (token) {
                sessionStorage.setItem(TOKEN_KEY, token);
                url.searchParams.delete(TOKEN_PARAM);
                history.replaceState(null, "", url.toString());
            }
        } catch {
            /* non-browser test harness or an unparseable URL: no token */
        }
    }

    token() {
        try {
            return sessionStorage.getItem(TOKEN_KEY) || "";
        } catch {
            return "";
        }
    }

    headers() {
        const headers = { "Content-Type": "application/json" };
        const token = this.token();
        if (token) headers["Authorization"] = `Bearer ${token}`;
        return headers;
    }

    setState(state, detail = "") {
        this.state = state;
        this.dispatchEvent(new CustomEvent("state", { detail: { state, detail, version: this.version } }));
    }

    async checkHealth() {
        try {
            const response = await fetch(this.healthURL, { headers: this.headers() });
            if (!response.ok) {
                throw new Error(`the native server answered ${response.status}`);
            }
            this.health = await response.json();
            this.version = this.health.version || "";
            this.setState(this.pending.size ? "busy" : "ready");
        } catch (err) {
            this.setState("error", err && err.message ? err.message : "the native server is unreachable");
        }
    }

    /* call sends one request.
     *
     * `supersede` names a slot: a newer request in the same slot makes this one
     * irrelevant, and its promise rejects with a superseded error the caller is
     * expected to ignore. `watchdogMs` is the outer bound - always longer than
     * whatever timeout the request itself carries, because the server stopping
     * a query cleanly is the good case.
     */
    call(method, payload = {}, { supersede = null, watchdogMs = 20000 } = {}) {
        const id = this.nextID++;
        if (supersede) this.latest.set(supersede, id);

        const controller = new AbortController();
        const entry = {
            method,
            controller,
            resolve: null,
            reject: null,
            watchdog: 0,
        };

        const promise = new Promise((resolve, reject) => {
            entry.resolve = (value) => {
                if (supersede && this.latest.get(supersede) !== id) {
                    reject(superseded());
                    return;
                }
                resolve(value);
            };
            entry.reject = reject;
            entry.watchdog = setTimeout(() => this.timedOut(id), watchdogMs);
        });
        this.pending.set(id, entry);
        this.setState("busy");
        this.post(id, method, payload, controller.signal);
        return promise;
    }

    async post(id, method, payload, signal) {
        const entry = this.pending.get(id);
        if (!entry) return;
        const done = (fn, value) => {
            if (!this.pending.has(id)) return;
            this.pending.delete(id);
            clearTimeout(entry.watchdog);
            if (this.pending.size === 0 && this.state === "busy") {
                this.setState("ready");
            }
            fn(value);
        };
        try {
            const response = await fetch(this.apiURL, {
                method: "POST",
                headers: this.headers(),
                body: JSON.stringify({ method, request: JSON.stringify(payload) }),
                signal,
            });
            if (!response.ok) {
                if (response.status === 401) {
                    throw new Error("the server refused the request: missing or wrong token");
                }
                throw new Error(`the server answered ${response.status}`);
            }
            const raw = await response.text();
            let parsed;
            try {
                parsed = JSON.parse(raw);
            } catch {
                throw new Error("the engine returned something that is not JSON");
            }
            done(entry.resolve, parsed);
        } catch (err) {
            if (err && err.name === "AbortError") {
                // Aborts are reported by timedOut/cancel themselves; the
                // fetch rejection is just the mechanism.
                return;
            }
            done(entry.reject, err instanceof Error ? err : new Error(String(err)));
        }
    }

    timedOut(id) {
        const entry = this.pending.get(id);
        if (!entry) return;
        // The server is not answering. Abort the request; unlike the WASM
        // worker, the engine itself stays up for the next call.
        try {
            entry.controller.abort();
        } catch {
            /* already gone */
        }
        this.pending.delete(id);
        clearTimeout(entry.watchdog);
        if (this.pending.size === 0 && this.state === "busy") {
            this.setState("ready");
        }
        entry.reject(new Error("the engine stopped responding; the request was aborted"));
    }

    /* cancel ends whatever is running by aborting every pending request. The
     * server sees each disconnect and cancels the query it was running. */
    cancel() {
        if (this.pending.size === 0) return false;
        for (const [, entry] of this.pending) {
            clearTimeout(entry.watchdog);
            try {
                entry.controller.abort();
            } catch {
                /* already gone */
            }
            entry.reject(cancelled());
        }
        this.pending.clear();
        if (this.state === "busy") this.setState("ready");
        return true;
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
