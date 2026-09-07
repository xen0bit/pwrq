/* Isolation tests: the WASM-only page must never call the native server.
 *
 * A safe static deployment serves pkg/web/dist as plain files. If any WASM
 * source referenced the native API, that deployment would ship the code that
 * phones a shell home the moment a server offers one. These tests pin the
 * absence: the WASM entry points contain no native markers, and the native
 * transport contains no WASM markers (so a review of either side is enough).
 *
 * Run them with `bun test` from pkg/web/src.
 */

import { describe, expect, test } from "bun:test";

async function read(path) {
    return await Bun.file(new URL(path, import.meta.url)).text();
}

// Markers that may only appear on the native side.
const NATIVE_MARKERS = ["engine-native", "api/call", "PWRQ_IDE_TOKEN", "pwrq.native.token"];

// Markers that may only appear on the WASM side.
const WASM_MARKERS = ["worker.js", "web.wasm", "wasm_exec", "pwrqCall"];

describe("wasm sources stay native-free", () => {
    for (const path of ["../index.html", "./main.js", "./engine.js", "../worker.js"]) {
        test(`${path} mentions no native endpoint`, async () => {
            const source = await read(path);
            for (const marker of NATIVE_MARKERS) {
                expect(source).not.toContain(marker);
            }
        });
    }
});

describe("native transport stays wasm-free", () => {
    test("engine-native.js loads no worker or module", async () => {
        const source = await read("./engine-native.js");
        for (const marker of WASM_MARKERS) {
            expect(source).not.toContain(marker);
        }
    });
});
