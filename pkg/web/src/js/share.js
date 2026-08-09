/* Sharing, in the URL fragment.
 *
 * A fragment is the right place for this: browsers never send it to a server,
 * so a shared link carries the query and its sample data without the data ever
 * leaving the two machines at either end of the conversation. It also means
 * the page needs no backend to be shareable.
 *
 * Fragments are compressed, because a real query with a real sample document
 * is easily a few kilobytes and some chat clients still truncate long links.
 * The format is self-describing:
 *
 *   #z=<base64url>   deflate-raw of the JSON state
 *   #j=<base64url>   the JSON state, for browsers without CompressionStream
 *   #q=…&i=…         plain percent-encoded query and input, for hand-writing
 */

const STATE_VERSION = 1;

export async function encodeState(state) {
    const payload = JSON.stringify(compact(state));

    if (typeof CompressionStream === "function") {
        try {
            const bytes = await deflate(payload);
            return `z=${toBase64URL(bytes)}`;
        } catch {
            /* fall through to the uncompressed form */
        }
    }
    return `j=${toBase64URL(new TextEncoder().encode(payload))}`;
}

export async function decodeHash(hash) {
    const raw = (hash || "").replace(/^#/, "");
    if (!raw) return null;

    const params = new URLSearchParams(raw);

    if (params.has("z")) {
        try {
            const bytes = fromBase64URL(params.get("z"));
            return validate(JSON.parse(await inflate(bytes)));
        } catch {
            return null;
        }
    }

    if (params.has("j")) {
        try {
            return validate(JSON.parse(new TextDecoder().decode(fromBase64URL(params.get("j")))));
        } catch {
            return null;
        }
    }

    // The hand-written form: #q=.a|.b&i={"a":1}
    if (params.has("q") || params.has("i")) {
        return validate({
            v: STATE_VERSION,
            q: params.get("q") || "",
            i: params.get("i") || "",
        });
    }

    return null;
}

/* compact keeps the fragment short by dropping everything at its default. The
 * keys are single letters for the same reason. */
function compact(state) {
    const out = { v: STATE_VERSION, q: state.query || "" };
    if (state.input) out.i = state.input;
    if (state.args?.length) out.a = state.args.map((arg) => [arg.name, arg.value]);
    const options = {};
    for (const [key, value] of Object.entries(state.options || {})) {
        if (value !== undefined && value !== null && value !== "") options[key] = value;
    }
    if (Object.keys(options).length) out.o = options;
    return out;
}

/* validate is the trust boundary: a fragment is user input, and it arrives
 * from whoever sent the link rather than from the person opening it. Nothing
 * here is executed - the query is text in a textarea until the reader runs it -
 * but every field is still coerced to the type the page expects, so a hostile
 * fragment cannot make the page render an object where it expected a string. */
function validate(state) {
    if (!state || typeof state !== "object") return null;

    const text = (value) => (typeof value === "string" ? value : "");
    const args = Array.isArray(state.a)
        ? state.a
              .filter((entry) => Array.isArray(entry) && entry.length >= 1)
              .slice(0, 32)
              .map((entry) => ({ name: text(entry[0]).slice(0, 64), value: text(entry[1]).slice(0, 65536) }))
              .filter((arg) => arg.name)
        : [];

    const options = {};
    if (state.o && typeof state.o === "object" && !Array.isArray(state.o)) {
        for (const [key, value] of Object.entries(state.o)) {
            if (typeof value === "string" || typeof value === "boolean" || typeof value === "number") {
                options[key] = value;
            }
        }
    }

    return {
        query: text(state.q),
        input: text(state.i),
        args,
        options,
    };
}

async function deflate(text) {
    const stream = new Blob([text]).stream().pipeThrough(new CompressionStream("deflate-raw"));
    return new Uint8Array(await new Response(stream).arrayBuffer());
}

async function inflate(bytes) {
    const stream = new Blob([bytes]).stream().pipeThrough(new DecompressionStream("deflate-raw"));
    return await new Response(stream).text();
}

function toBase64URL(bytes) {
    let binary = "";
    // A spread over a large array overflows the argument limit, so this goes
    // in chunks: sample documents are routinely tens of kilobytes.
    for (let i = 0; i < bytes.length; i += 0x8000) {
        binary += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
    }
    return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function fromBase64URL(text) {
    const padded = text.replace(/-/g, "+").replace(/_/g, "/");
    const binary = atob(padded + "=".repeat((4 - (padded.length % 4)) % 4));
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
    return bytes;
}
