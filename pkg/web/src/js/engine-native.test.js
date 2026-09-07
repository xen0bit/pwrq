/* Tests for the native transport.
 *
 * Run them with `bun test` from pkg/web/src. They need no browser: fetch,
 * sessionStorage and location are stubbed per test.
 */

import { beforeEach, describe, expect, test } from "bun:test";
import { Engine, cancelled, isNoise, superseded } from "./engine-native.js";

function installBrowser(href = "http://localhost:8080/tools/pwrq/") {
    const store = new Map();
    globalThis.window = { location: { href } };
    globalThis.history = {
        replaced: null,
        replaceState(_state, _title, url) {
            this.replaced = url;
        },
    };
    globalThis.sessionStorage = {
        getItem: (key) => (store.has(key) ? store.get(key) : null),
        setItem: (key, value) => void store.set(key, value),
        removeItem: (key) => void store.delete(key),
    };
    return store;
}

function jsonResponse(value, status = 200) {
    return new Response(JSON.stringify(value), { status });
}

beforeEach(() => {
    installBrowser();
    globalThis.fetch = async () => jsonResponse({ mode: "native", version: "test" });
});

describe("native engine transport", () => {
    test("posts method and stringified request as JSON", async () => {
        let seen = null;
        globalThis.fetch = async (url, init) => {
            if (String(url).endsWith("health")) return jsonResponse({ mode: "native", version: "v" });
            seen = { url, init };
            return jsonResponse({ ok: true });
        };
        const engine = new Engine("api/call");
        const result = await engine.call("run", { query: ".a" }, { watchdogMs: 5000 });
        expect(result).toEqual({ ok: true });
        expect(String(seen.url)).toBe("api/call");
        expect(seen.init.method).toBe("POST");
        const body = JSON.parse(seen.init.body);
        expect(body.method).toBe("run");
        expect(JSON.parse(body.request)).toEqual({ query: ".a" });
    });

    test("adopts a token from the URL once and strips it", async () => {
        installBrowser("http://localhost:8080/tools/pwrq/?token=abc123#frag");
        const engine = new Engine("api/call");
        expect(engine.token()).toBe("abc123");
        expect(globalThis.history.replaced).not.toContain("token=abc123");
        expect(globalThis.history.replaced).toContain("#frag");
    });

    test("sends the stored token as a bearer header", async () => {
        const store = installBrowser();
        store.set("pwrq.native.token", "s3cret");
        let seen = null;
        globalThis.fetch = async (url, init) => {
            if (String(url).endsWith("health")) return jsonResponse({});
            seen = init;
            return jsonResponse({});
        };
        const engine = new Engine("api/call");
        await engine.call("catalog", {}, { watchdogMs: 5000 });
        expect(seen.headers["Authorization"]).toBe("Bearer s3cret");
    });

    test("a newer request in the same slot supersedes the older one", async () => {
        globalThis.fetch = async (url) => {
            if (String(url).endsWith("health")) return jsonResponse({});
            await new Promise((resolve) => setTimeout(resolve, 5));
            return jsonResponse({ n: 1 });
        };
        const engine = new Engine("api/call");
        const first = engine.call("run", { query: ".a" }, { supersede: "run", watchdogMs: 5000 });
        const second = engine.call("run", { query: ".b" }, { supersede: "run", watchdogMs: 5000 });
        await expect(first).rejects.toThrow("superseded");
        await expect(second).resolves.toEqual({ n: 1 });
    });

    test("cancel rejects pending work as cancelled", async () => {
        globalThis.fetch = async (url) => {
            if (String(url).endsWith("health")) return jsonResponse({});
            await new Promise((resolve) => setTimeout(resolve, 50));
            return jsonResponse({});
        };
        const engine = new Engine("api/call");
        const pending = engine.call("run", { query: "." }, { watchdogMs: 5000 });
        expect(engine.cancel()).toBe(true);
        await expect(pending).rejects.toThrow("cancelled");
        expect(engine.cancel()).toBe(false);
    });

    test("a watchdog aborts a hung request", async () => {
        let aborted = false;
        globalThis.fetch = (url, init) => {
            if (String(url).endsWith("health")) return jsonResponse({});
            return new Promise((_resolve, reject) => {
                init.signal.addEventListener("abort", () => {
                    aborted = true;
                    reject(new DOMException("aborted", "AbortError"));
                });
            });
        };
        const engine = new Engine("api/call");
        await expect(engine.call("run", { query: "." }, { watchdogMs: 20 })).rejects.toThrow(
            "stopped responding",
        );
        expect(aborted).toBe(true);
        expect(engine.busy).toBe(false);
    });

    test("a refused token reads as an auth error", async () => {
        globalThis.fetch = async (url) => {
            if (String(url).endsWith("health")) return jsonResponse({});
            return new Response("unauthorized", { status: 401 });
        };
        const engine = new Engine("api/call");
        await expect(engine.call("run", { query: "." }, { watchdogMs: 5000 })).rejects.toThrow("token");
    });

    test("a non-JSON reply reads as an engine error", async () => {
        globalThis.fetch = async (url) => {
            if (String(url).endsWith("health")) return jsonResponse({});
            return new Response("not json", { status: 200 });
        };
        const engine = new Engine("api/call");
        await expect(engine.call("run", { query: "." }, { watchdogMs: 5000 })).rejects.toThrow("not JSON");
    });

    test("an unreachable server reports an error state", async () => {
        globalThis.fetch = async () => {
            throw new Error("connection refused");
        };
        const engine = new Engine("api/call");
        const states = [];
        engine.addEventListener("state", (event) => states.push(event.detail.state));
        await engine.checkHealth();
        expect(states).toContain("error");
    });
});

describe("noise helpers", () => {
    test("cancelled and superseded are noise, other errors are not", () => {
        expect(isNoise(cancelled())).toBe(true);
        expect(isNoise(superseded())).toBe(true);
        expect(isNoise(new Error("boom"))).toBe(false);
        expect(isNoise(null)).toBe(false);
    });
});
