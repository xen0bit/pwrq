/* Tests for the sharing format.
 *
 * Run them with `bun test` from pkg/web/src. They need no browser and no
 * dependencies: this module touches nothing but text, compression and base64,
 * all of which the runtime already has.
 *
 * The property under test is the one a link depends on: whatever goes into a
 * fragment comes back out of it unchanged, on the other side of someone
 * else's chat client.
 */

import { describe, expect, test } from "bun:test";
import { decodeHash, encodeState } from "./share.js";

async function roundTrip(state) {
    return await decodeHash(`#${await encodeState(state)}`);
}

describe("share links", () => {
    test("carry the query and the input", async () => {
        const state = { query: '.items[] | select(.n > 1) | {Name}', input: '{"items":[{"n":2,"Name":"a"}]}' };
        const back = await roundTrip(state);

        expect(back.query).toBe(state.query);
        expect(back.input).toBe(state.input);
    });

    test("carry arguments", async () => {
        const back = await roundTrip({
            query: "$limit",
            args: [{ name: "limit", value: "1000" }],
        });

        expect(back.args).toEqual([{ name: "limit", value: "1000" }]);
    });

    test("carry the options that change what a reader sees", async () => {
        const back = await roundTrip({ query: ".", options: { output: "raw", slurp: true } });
        expect(back.options).toEqual({ output: "raw", slurp: true });
    });

    test("survive text that is awkward in a URL", async () => {
        const query = '.["a/b+c=d"] | @base64 "x\\(.y)" # comment with spaces & symbols';
        const input = '{"emoji":"🙂","quote":"\\"","newline":"a\\nb"}';
        const back = await roundTrip({ query, input });

        expect(back.query).toBe(query);
        expect(back.input).toBe(input);
    });

    test("are compressed, so a big sample document still fits in a link", async () => {
        const input = JSON.stringify(
            Array.from({ length: 400 }, (_, i) => ({ Name: `item-${i}`, Size: i * 37, Status: "Running" })),
        );
        const encoded = await encodeState({ query: ".", input });

        expect(encoded.startsWith("z=")).toBe(true);
        // Deflate on repetitive JSON should be worth an order of magnitude;
        // anything less means the compression path silently stopped working.
        expect(encoded.length).toBeLessThan(input.length / 5);

        const back = await decodeHash(`#${encoded}`);
        expect(back.input).toBe(input);
    });

    test("read the hand-written form too", async () => {
        const back = await decodeHash(`#q=${encodeURIComponent(".a | .b")}&i=${encodeURIComponent("{}")}`);
        expect(back.query).toBe(".a | .b");
        expect(back.input).toBe("{}");
    });

    test("read the uncompressed form", async () => {
        const payload = JSON.stringify({ v: 1, q: ".x" });
        const base64 = btoa(payload).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
        const back = await decodeHash(`#j=${base64}`);
        expect(back.query).toBe(".x");
    });

    test("ignore a fragment that is not one of ours", async () => {
        expect(await decodeHash("")).toBeNull();
        expect(await decodeHash("#section-3")).toBeNull();
        expect(await decodeHash("#z=not-valid-base64!!")).toBeNull();
        expect(await decodeHash("#j=" + btoa("not json"))).toBeNull();
    });

    /* A fragment arrives from whoever wrote the link, not from the person
     * opening it. Nothing in it is executed - the query is text in a textarea
     * until the reader chooses to run it - but every field still has to arrive
     * as the type the page expects. */
    test("coerce hostile shapes into the types the page expects", async () => {
        const hostile = { v: 1, q: { toString: "nope" }, i: 42, a: "not an array", o: { output: { evil: true } } };
        const base64 = btoa(JSON.stringify(hostile)).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
        const back = await decodeHash(`#j=${base64}`);

        expect(back.query).toBe("");
        expect(back.input).toBe("");
        expect(back.args).toEqual([]);
        expect(back.options).toEqual({});
    });

    test("cap how many arguments a link may carry", async () => {
        const args = Array.from({ length: 100 }, (_, i) => [`a${i}`, "1"]);
        const base64 = btoa(JSON.stringify({ v: 1, q: ".", a: args })).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
        const back = await decodeHash(`#j=${base64}`);

        expect(back.args.length).toBeLessThanOrEqual(32);
    });
});
