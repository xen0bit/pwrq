/* The command palette: one keystroke to reach anything the page can do.
 *
 * It is a list of items with a fuzzy filter, not a menu, because the point is
 * that you type what you want rather than remember where it lives. Items come
 * from the page's own actions, the cmdlet catalog and the example gallery, so
 * a visitor who has read none of the UI can still find `sha256` by name.
 */

export class CommandPalette {
    constructor(modal, input, list, { source }) {
        this.modal = modal;
        this.input = input;
        this.list = list;
        this.source = source;
        this.items = [];
        this.index = 0;
        this.bind();
    }

    bind() {
        this.input.addEventListener("input", () => this.filter());
        this.input.addEventListener("keydown", (event) => {
            switch (event.key) {
                case "ArrowDown":
                    event.preventDefault();
                    this.move(1);
                    break;
                case "ArrowUp":
                    event.preventDefault();
                    this.move(-1);
                    break;
                case "Enter":
                    event.preventDefault();
                    this.accept();
                    break;
                case "Escape":
                    event.preventDefault();
                    this.close();
                    break;
            }
        });
        this.modal.addEventListener("mousedown", (event) => {
            if (event.target === this.modal) this.close();
        });
    }

    open() {
        this.modal.classList.remove("hidden");
        this.input.value = "";
        this.filter();
        this.input.focus();
    }

    close() {
        this.modal.classList.add("hidden");
        this.previousFocus?.focus?.();
    }

    get isOpen() {
        return !this.modal.classList.contains("hidden");
    }

    filter() {
        const query = this.input.value.trim().toLowerCase();
        const all = this.source();
        this.items = query ? all.filter((item) => score(item, query) !== null).sort((a, b) => score(a, query) - score(b, query)) : all.slice(0, 60);
        this.index = 0;
        this.render();
    }

    render() {
        if (this.items.length === 0) {
            this.list.innerHTML = `<li class="palette-item"><span class="title">Nothing matches</span></li>`;
            return;
        }
        this.list.innerHTML = this.items
            .slice(0, 60)
            .map(
                (item, i) => `
            <li class="palette-item" role="option" data-index="${i}" aria-selected="${i === this.index}">
                <span class="kind">${escape(item.kind)}</span>
                <span class="title">${escape(item.title)}</span>
                ${item.detail ? `<span class="detail">${escape(item.detail)}</span>` : ""}
            </li>`,
            )
            .join("");

        for (const node of this.list.querySelectorAll(".palette-item[data-index]")) {
            node.addEventListener("mousedown", (event) => {
                event.preventDefault();
                this.index = Number(node.dataset.index);
                this.accept();
            });
        }
        this.list.querySelector('[aria-selected="true"]')?.scrollIntoView({ block: "nearest" });
    }

    move(delta) {
        if (this.items.length === 0) return;
        this.index = (this.index + delta + this.items.length) % this.items.length;
        this.render();
    }

    accept() {
        const item = this.items[this.index];
        if (!item) return;
        this.close();
        item.run();
    }
}

/* score ranks a match. A subsequence match is enough to keep an item - typing
 * "b64e" should find base64_encode - but a contiguous match ranks higher, and
 * a match at the start higher still. */
function score(item, query) {
    const haystack = `${item.title} ${item.detail || ""}`.toLowerCase();
    const at = haystack.indexOf(query);
    if (at === 0) return 0;
    if (at > 0) return 1 + at / 100;

    let position = 0;
    for (const char of query) {
        position = haystack.indexOf(char, position);
        if (position === -1) return null;
        position++;
    }
    return 50 + position / 100;
}

function escape(text) {
    return String(text ?? "")
        .replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;");
}
