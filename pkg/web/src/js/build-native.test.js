/* Tests for the native page generator.
 *
 * The property under test is derivation: the native sources come from the
 * WASM ones with a few deliberate changes, and the generator refuses to run
 * when its anchors are gone rather than shipping a stale page.
 *
 * Run them with `bun test` from pkg/web/src.
 */

import { describe, expect, test } from "bun:test";
import { generateNativeIndex, generateNativeMain } from "../build-native.mjs";

async function read(path) {
    return await Bun.file(new URL(path, import.meta.url)).text();
}

describe("native main derivation", () => {
    test("swaps the transport and endpoint", async () => {
        const mainJs = await read("./main.js");
        const native = generateNativeMain(mainJs);
        expect(native).toContain('from "./engine-native.js"');
        expect(native).toContain('new Engine("api/call")');
        expect(native).not.toContain('from "./engine.js"');
        expect(native).not.toContain("worker.js");
    });

    test("validate carries the bound variables", async () => {
        const mainJs = await read("./main.js");
        const native = generateNativeMain(mainJs);
        expect(native).toContain("state.args.filter");
    });

    test("fails loudly when an anchor disappears", () => {
        expect(() => generateNativeMain("nothing to see here")).toThrow();
    });
});

describe("native index derivation", () => {
    test("names the mode and warns where queries run", async () => {
        const indexHtml = await read("../index.html");
        const native = generateNativeIndex(indexHtml);
        expect(native).toContain("query editor · native");
        expect(native).toContain("js/main-native.js");
        expect(native).toContain("Queries run on the server");
        expect(native).not.toContain('src="js/main.js"');
        expect(native).not.toContain("compiled to WebAssembly");
    });

    test("fails loudly when an anchor disappears", () => {
        expect(() => generateNativeIndex("<html></html>")).toThrow();
    });
});
