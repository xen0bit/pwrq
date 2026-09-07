/* Generate the native IDE page from the WASM page's sources.
 *
 * The native and WASM pages share everything except the engine transport and
 * a few words of copy (what "here" means, where queries run). Rather than
 * maintaining a 1400-line fork that rots, this script derives the native
 * sources from the WASM ones with a small list of replacements, and fails
 * loudly if any anchor is missing - so an edit to index.html or main.js that
 * the native page must know about breaks `make web.build-native` instead of
 * shipping a stale page.
 *
 * The WASM build never reads the generated files, and never imports
 * engine-native.js, which is what keeps a static deployment from ever calling
 * the server's API. See isolation.test.js.
 *
 * Run from pkg/web/src: `bun build-native.mjs`
 */

import { readFileSync, writeFileSync } from "node:fs";

const NATIVE_DISCLAIMER =
    "Queries run on the server, as the user serving this page, with the full cmdlet vocabulary: " +
    "files, processes, services, the network, and shell commands. Anyone who can reach this page can act as that user.";

function replaceOnce(haystack, needle, replacement, name) {
    const count = haystack.split(needle).length - 1;
    if (count !== 1) {
        throw new Error(`native build: anchor ${name} found ${count} times, want exactly once`);
    }
    return haystack.replace(needle, replacement);
}

/* generateNativeMain derives js/main-native.js from js/main.js. The only
 * functional changes are the transport import, the engine endpoint, and the
 * validate payload carrying the bound variables (the native validator
 * compiles, so $name must resolve). Everything else - editors, output,
 * diagram, catalog, examples, history, palette, sharing - is shared. */
export function generateNativeMain(mainJs) {
    let out = mainJs;
    out = replaceOnce(
        out,
        'import { Engine, isNoise } from "./engine.js";',
        'import { Engine, isNoise } from "./engine-native.js";',
        "engine import",
    );
    out = replaceOnce(out, 'new Engine("worker.js")', 'new Engine("api/call")', "engine endpoint");
    out = replaceOnce(
        out,
        'const result = await engine.call("validate", { query }, { supersede: "validate", watchdogMs: 10000 });',
        "const result = await engine.call(\n            // The native validator compiles, so the bound variables travel too.\n" +
            '            "validate",\n            { query, args: state.args.filter((arg) => arg.name.trim()) },\n' +
            '            { supersede: "validate", watchdogMs: 10000 },\n' +
            "        );",
        "validate args",
    );
    out = replaceOnce(
        out,
        'toast("stopped; the engine restarted");',
        'toast("stopped");',
        "cancel toast",
    );
    return out;
}

/* generateNativeIndex derives index-native.html from index.html: the title
 * and tagline name the mode, the catalog note and the help text say where
 * queries run, and the entry script is the generated main. */
export function generateNativeIndex(indexHtml) {
    let out = indexHtml;
    out = replaceOnce(out, "<title>pwrq — query editor</title>", "<title>pwrq — query editor (native)</title>", "title");
    out = replaceOnce(
        out,
        '<meta name="description" content="Write jq and PowerShell-style pipelines, run them against sample JSON, and see their structure as a coloured flow diagram. Everything runs in the tab; links carry the query.">',
        '<meta name="description" content="Write jq and PowerShell-style pipelines against the full native cmdlet vocabulary, and see their structure as a coloured flow diagram. Queries run on the server; links carry the query.">',
        "meta description",
    );
    out = replaceOnce(
        out,
        '<span class="tagline">query editor</span>',
        '<span class="tagline">query editor · native</span>',
        "tagline",
    );
    out = replaceOnce(
        out,
        `<p class="note">
                        Everything the CLI can do is listed here. A browser tab has no filesystem, process table or
                        service manager, so the cmdlets that need one are marked “not in the browser” — they run in
                        the pwrq CLI, not in this tab. Everything else does exactly what it says: codecs, hashes,
                        ciphers, compression, format conversion, and the object and formatting cmdlets.
                    </p>`,
        `<p class="note">
                        Everything the CLI can do is listed here, and everything listed runs here: this page is backed
                        by the server's full cmdlet vocabulary. ${NATIVE_DISCLAIMER}
                    </p>`,
        "catalog note",
    );
    out = replaceOnce(
        out,
        `<p>
            The whole of pwrq is compiled to WebAssembly and runs in a worker thread inside this tab. Nothing you type
            is uploaded anywhere: the sharing link carries the query and the sample input in its <code>#</code> fragment,
            which browsers never send to a server.
        </p>`,
        `<p>
            ${NATIVE_DISCLAIMER} The sharing link carries the query and the sample input in its
            <code>#</code> fragment, which browsers never send to a server — but the query itself runs on the
            server the moment it evaluates.
        </p>`,
        "help text",
    );
    out = replaceOnce(
        out,
        '<script src="js/main.js" type="module"></script>',
        '<script src="js/main-native.js" type="module"></script>',
        "entry script",
    );
    return out;
}

/* The entry point only runs when executed directly (`bun build-native.mjs`),
 * not when imported by the unit tests. */
if (import.meta.main) {
    const indexHtml = readFileSync("index.html", "utf8");
    const mainJs = readFileSync("js/main.js", "utf8");

    writeFileSync("index-native.html", generateNativeIndex(indexHtml));
    writeFileSync("js/main-native.js", generateNativeMain(mainJs));
    console.log("wrote index-native.html and js/main-native.js");
}
