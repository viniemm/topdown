// A one-line text field with a visible cursor (search box, rename, add).
import type { Key } from "./keys";
import { theme } from "./theme";
import { width } from "./text";

const graphemes = new Intl.Segmenter(undefined, { granularity: "grapheme" });

export class Input {
	prompt = "";
	#chars: string[] = [];
	#cursor = 0;

	getValue(): string {
		return this.#chars.join("");
	}

	setValue(v: string): void {
		this.#chars = [...graphemes.segment(v)].map(g => g.segment);
		this.#cursor = this.#chars.length;
	}

	/** Apply an editing key; false when the key isn't an edit (the host handles it). */
	handle(k: Key): boolean {
		const c = this.#chars;
		switch (k.name) {
			case "left":
				this.#cursor = Math.max(0, this.#cursor - 1);
				return true;
			case "right":
				this.#cursor = Math.min(c.length, this.#cursor + 1);
				return true;
			case "home":
				this.#cursor = 0;
				return true;
			case "end":
				this.#cursor = c.length;
				return true;
			case "backspace":
				if (this.#cursor > 0) c.splice(--this.#cursor, 1);
				return true;
			case "delete":
				c.splice(this.#cursor, 1);
				return true;
			case "ctrl+u":
				c.splice(0, this.#cursor);
				this.#cursor = 0;
				return true;
			case "ctrl+w":
			case "alt+backspace": {
				let i = this.#cursor;
				while (i > 0 && c[i - 1] === " ") i--;
				while (i > 0 && c[i - 1] !== " ") i--;
				c.splice(i, this.#cursor - i);
				this.#cursor = i;
				return true;
			}
		}
		if (k.text !== undefined) {
			const add = [...graphemes.segment(k.name === "paste" ? k.text.replace(/\s+/g, " ") : k.text)].map(g => g.segment);
			c.splice(this.#cursor, 0, ...add);
			this.#cursor += add.length;
			return true;
		}
		return false;
	}

	/** prompt + value, scrolled so the cursor (shown inverted) stays visible. */
	render(w: number, focused = true): string {
		const room = Math.max(1, w - width(this.prompt) - 1);
		let start = 0;
		let used = 0;
		for (let i = this.#cursor - 1; i >= 0; i--) {
			used += width(this.#chars[i]!);
			if (used > room) {
				start = i + 1;
				break;
			}
		}
		let out = "";
		let cols = 0;
		for (let i = start; i <= this.#chars.length; i++) {
			const ch = this.#chars[i] ?? " ";
			const cw = width(ch);
			if (cols + cw > room + 1) break;
			out += i === this.#cursor && focused ? theme.inverse(ch) : ch;
			cols += cw;
		}
		return this.prompt + out;
	}
}
