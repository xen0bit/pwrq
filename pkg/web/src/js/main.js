let wasmModule = null;

// Polyfill for WebAssembly.instantiateStreaming if not available
if (!WebAssembly.instantiateStreaming) {
    WebAssembly.instantiateStreaming = async (resp, importObject) => {
        const source = await (await resp).arrayBuffer();
        return await WebAssembly.instantiate(source, importObject);
    };
}

// Initialize WASM
async function initWASM() {
    console.log("Initializing WASM...");

    // Wait a bit for wasm_exec.js to load and define Go
    let attempts = 0;
    while (typeof Go === 'undefined' && attempts < 50) {
        await new Promise(resolve => setTimeout(resolve, 10));
        attempts++;
    }

    // Check if Go is available
    if (typeof Go === 'undefined') {
        console.error("Go is not defined. Make sure wasm_exec.js is loaded.");
        throw new Error("Go is not defined");
    }

    console.log("Go is available, creating new Go()");
    const go = new Go();
    const result = await WebAssembly.instantiateStreaming(fetch("web.wasm"), go.importObject);
    wasmModule = result.instance;
    // Run Go program in background (non-blocking)
    go.run(result.instance);
    // Wait a bit for Go to initialize and expose functions
    await new Promise(resolve => setTimeout(resolve, 100));
    console.log("WASM module loaded");
}

// Validate query and update UI
function doValidateQuery() {
    const query = document.getElementById("query").value;
    const textarea = document.getElementById("query");
    const svgContainer = document.getElementById("svg-container");
    const errorSmall = document.getElementById("query-error");

    if (!query) {
        // Empty query - remove validation state
        textarea.removeAttribute("aria-invalid");
        errorSmall.textContent = "";
        svgContainer.innerHTML = "<p><em>Enter a valid query above to generate a flow diagram.</em></p>";
        return;
    }

    // Check if WASM functions are available
    if (typeof window.validateQuery !== 'function') {
        return;
    }

    try {
        const result = window.validateQuery(query);

        // Go WASM returns a plain JavaScript object from map[string]interface{}
        const ok = result && result.ok === true;
        const err = result && result.err ? result.err : "";

        if (ok) {
            textarea.setAttribute("aria-invalid", "false");
            errorSmall.textContent = "";
            // Automatically generate SVG for valid queries
            generateSVG();
        } else {
            textarea.setAttribute("aria-invalid", "true");
            errorSmall.textContent = err || "Query is invalid. Please check your syntax.";
            svgContainer.innerHTML = "<p><em>Query is invalid. Please check your syntax.</em></p>";
        }
    } catch (error) {
        textarea.setAttribute("aria-invalid", "true");
        errorSmall.textContent = "Error validating query: " + error.message;
        svgContainer.innerHTML = "<p><em>Error validating query.</em></p>";
    }
}

// Generate SVG
function generateSVG() {
    const query = document.getElementById("query").value;
    const svgContainer = document.getElementById("svg-container");

    if (!query) {
        svgContainer.innerHTML = "<p><em>Enter a valid query above to generate a flow diagram.</em></p>";
        return;
    }

    // Check if WASM functions are available
    if (typeof window.createSVG !== 'function') {
        return;
    }

    try {
        const result = window.createSVG(query);

        // Go WASM returns a plain JavaScript object from map[string]interface{}
        const svg = result && result.svg ? result.svg : "";
        const err = result && result.err ? result.err : "";

        if (err) {
            svgContainer.innerHTML = "<p><em>Error generating SVG: " + err + "</em></p>";
        } else if (svg) {
            svgContainer.innerHTML = svg;
        } else {
            svgContainer.innerHTML = "<p><em>No SVG generated.</em></p>";
        }
    } catch (error) {
        svgContainer.innerHTML = "<p><em>Error generating SVG.</em></p>";
    }
}

// Set up real-time validation as user types (attach immediately)
const queryInput = document.getElementById("query");
let validationTimeout;

queryInput.addEventListener("input", () => {
    // Debounce validation to avoid too many calls
    clearTimeout(validationTimeout);
    validationTimeout = setTimeout(() => {
        doValidateQuery();
    }, 300); // Wait 300ms after user stops typing
});

// Initialize WASM on page load
(async () => {
    try {
        await initWASM();

        // Verify WASM functions are available
        if (typeof window.validateQuery !== 'function' || typeof window.createSVG !== 'function') {
            console.error("WASM functions not available");
            const textarea = document.getElementById("query");
            textarea.setAttribute("aria-invalid", "true");
            textarea.placeholder = "Error: WASM functions not loaded. Please refresh the page.";
        }
    } catch (err) {
        console.error("Failed to load WASM:", err);
        const textarea = document.getElementById("query");
        textarea.setAttribute("aria-invalid", "true");
        textarea.placeholder = "Error: Failed to load WASM module. Please refresh the page.";
    }
})();