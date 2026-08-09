/* The diagram viewport: zoom, pan, fit, and export.
 *
 * The SVG arrives from the Go renderer already laid out and coloured, so this
 * only has to make a large picture navigable in a small panel. Transform on a
 * wrapper rather than on the SVG itself keeps the SVG's own coordinates intact,
 * which is what lets "download" hand over exactly what was rendered.
 */

const MIN_SCALE = 0.1;
const MAX_SCALE = 6;

export class DiagramView {
    constructor(stage, canvas, { onZoom = () => {} } = {}) {
        this.stage = stage;
        this.canvas = canvas;
        this.onZoom = onZoom;
        this.scale = 1;
        this.x = 0;
        this.y = 0;
        this.svg = "";
        this.natural = { width: 0, height: 0 };
        this.bind();
    }

    bind() {
        this.stage.addEventListener(
            "wheel",
            (event) => {
                if (!this.svg) return;
                // Ctrl+wheel is the pinch gesture on a trackpad; plain wheel
                // zooms too, because the panel does not scroll otherwise.
                event.preventDefault();
                const rect = this.stage.getBoundingClientRect();
                const factor = Math.exp(-event.deltaY * 0.0015);
                this.zoomAt(event.clientX - rect.left, event.clientY - rect.top, factor);
            },
            { passive: false },
        );

        let pointer = null;
        this.stage.addEventListener("pointerdown", (event) => {
            if (!this.svg || event.button !== 0) return;
            pointer = { id: event.pointerId, x: event.clientX, y: event.clientY };
            this.stage.setPointerCapture(event.pointerId);
            this.stage.classList.add("panning");
        });
        this.stage.addEventListener("pointermove", (event) => {
            if (!pointer || event.pointerId !== pointer.id) return;
            this.x += event.clientX - pointer.x;
            this.y += event.clientY - pointer.y;
            pointer = { id: event.pointerId, x: event.clientX, y: event.clientY };
            this.apply();
        });
        const release = (event) => {
            if (!pointer || event.pointerId !== pointer.id) return;
            pointer = null;
            this.stage.classList.remove("panning");
        };
        this.stage.addEventListener("pointerup", release);
        this.stage.addEventListener("pointercancel", release);

        this.stage.addEventListener("dblclick", () => this.fit());
    }

    /* show replaces the picture. `keepView` holds the current zoom and pan,
     * which is what you want while editing: re-fitting on every keystroke
     * would make the diagram jump under the reader. */
    show(svg, { keepView = false } = {}) {
        this.svg = svg;
        this.canvas.innerHTML = svg;

        const node = this.canvas.querySelector("svg");
        if (node) {
            // d2 emits an SVG sized only by its viewBox. Inside an absolutely
            // positioned wrapper that resolves to nothing - percentage sizes
            // need a parent with a size - so the intrinsic dimensions are
            // written back explicitly. The wrapper's transform is then what
            // scales it, and the element keeps its natural size.
            const box = node.viewBox?.baseVal;
            const width = attributeSize(node, "width") || box?.width || 0;
            const height = attributeSize(node, "height") || box?.height || 0;

            this.natural = { width, height };
            if (width && height) {
                node.setAttribute("width", String(width));
                node.setAttribute("height", String(height));
            }
            node.style.maxWidth = "none";
        }

        if (keepView) this.apply();
        else this.fit();
    }

    clear() {
        this.svg = "";
        this.canvas.innerHTML = "";
    }

    fit() {
        const { width, height } = this.natural;
        if (!width || !height) return;
        const margin = 24;
        const available = {
            width: Math.max(this.stage.clientWidth - margin, 40),
            height: Math.max(this.stage.clientHeight - margin, 40),
        };
        // Never enlarge past 1: a two-node diagram blown up to fill the panel
        // looks like an error, not a feature.
        this.scale = Math.min(available.width / width, available.height / height, 1);
        this.x = (this.stage.clientWidth - width * this.scale) / 2;
        this.y = (this.stage.clientHeight - height * this.scale) / 2;
        this.apply();
    }

    zoomBy(factor) {
        this.zoomAt(this.stage.clientWidth / 2, this.stage.clientHeight / 2, factor);
    }

    zoomAt(originX, originY, factor) {
        const next = clamp(this.scale * factor, MIN_SCALE, MAX_SCALE);
        const ratio = next / this.scale;
        // Keep the point under the cursor where it is.
        this.x = originX - (originX - this.x) * ratio;
        this.y = originY - (originY - this.y) * ratio;
        this.scale = next;
        this.apply();
    }

    apply() {
        this.canvas.style.transform = `translate(${this.x}px, ${this.y}px) scale(${this.scale})`;
        this.onZoom(this.scale);
    }
}

/* attributeSize reads a width or height that is written in absolute units.
 * A percentage is no use here: it would be resolved against the very box this
 * is trying to establish. */
function attributeSize(node, name) {
    const raw = node.getAttribute(name);
    if (!raw || raw.includes("%")) return 0;
    return parseFloat(raw) || 0;
}

function clamp(value, min, max) {
    return Math.min(Math.max(value, min), max);
}

/* download hands the file to the browser. Object URLs are revoked on the next
 * task rather than immediately, because Safari cancels a download whose URL is
 * revoked while the click is still being processed. */
export function download(filename, content, type = "text/plain") {
    const url = URL.createObjectURL(new Blob([content], { type }));
    const link = document.createElement("a");
    link.href = url;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
}
