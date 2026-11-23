// Terminal input → key events. Handles xterm/VT sequences (arrows, Home/End,
// PgUp/PgDn, Del, F1–F12 with ctrl/alt/shift modifiers), control and alt
// combinations, and bracketed paste. One read can hold several keys.

export interface Key {
	/** Normalised name: "up", "ctrl+x", "alt+enter", "f2", "a", "paste", … */
	name: string;
	/** Text to insert, for printable keys and pastes. */
	text?: string;
}

const CSI_TILDE: Record<string, string> = {
	"1": "home", "2": "insert", "3": "delete", "4": "end", "5": "pageUp", "6": "pageDown", "7": "home", "8": "end",
	"11": "f1", "12": "f2", "13": "f3", "14": "f4", "15": "f5", "17": "f6", "18": "f7", "19": "f8",
	"20": "f9", "21": "f10", "23": "f11", "24": "f12",
};
const CSI_LETTER: Record<string, string> = {
	A: "up", B: "down", C: "right", D: "left", H: "home", F: "end", P: "f1", Q: "f2", R: "f3", S: "f4", Z: "shift+tab",
};

/** xterm modifier parameter (1 + bits: shift 1, alt 2, ctrl 4) → prefix. */
function mods(param: string | undefined): string {
	const m = Number(param ?? 1) - 1;
	if (!(m > 0)) return "";
	return `${m & 4 ? "ctrl+" : ""}${m & 2 ? "alt+" : ""}${m & 1 ? "shift+" : ""}`;
}

function control(code: number): string | undefined {
	if (code === 13 || code === 10) return "enter";
	if (code === 9) return "tab";
	if (code === 127 || code === 8) return "backspace";
	if (code === 0) return "ctrl+space";
	if (code >= 1 && code <= 26) return `ctrl+${String.fromCharCode(code + 96)}`;
	return undefined;
}

const graphemes = new Intl.Segmenter(undefined, { granularity: "grapheme" });
const PASTE_START = "\x1b[200~";
const PASTE_END = "\x1b[201~";

export function parseKeys(data: string): Key[] {
	const out: Key[] = [];
	let i = 0;
	while (i < data.length) {
		if (data.startsWith(PASTE_START, i)) {
			const end = data.indexOf(PASTE_END, i + PASTE_START.length);
			const stop = end === -1 ? data.length : end;
			out.push({ name: "paste", text: data.slice(i + PASTE_START.length, stop).replace(/\r\n?/g, "\n") });
			i = end === -1 ? data.length : end + PASTE_END.length;
			continue;
		}
		const c = data.charCodeAt(i);
		if (c === 0x1b) {
			const next = data[i + 1];
			if (next === undefined) {
				out.push({ name: "escape" });
				i++;
				continue;
			}
			if (next === "[" || next === "O") {
				// CSI/SS3: parameters, then one final byte.
				const m = /^[\x1b](?:\[|O)([0-9;]*)([~A-Za-z])/.exec(data.slice(i));
				if (m) {
					const [all, params, final] = m;
					const parts = params!.split(";");
					let name: string | undefined;
					if (final === "~") name = CSI_TILDE[parts[0]!] && mods(parts[1]) + CSI_TILDE[parts[0]!];
					else if (final === "u") {
						// CSI u (kitty / modifyOtherKeys style): codepoint;modifiers
						const code = Number(parts[0]);
						const base = control(code) ?? (code === 27 ? "escape" : String.fromCodePoint(code));
						name = mods(parts[1]) + base;
					} else if (CSI_LETTER[final!]) name = mods(parts[1]) + CSI_LETTER[final!];
					out.push({ name: name ?? `unknown:${all}` });
					i += all!.length;
					continue;
				}
			}
			// ESC + key = alt+key (Alt-Enter is ESC CR).
			const inner = parseKeys(data.slice(i + 1, i + 2))[0];
			if (inner && !inner.name.startsWith("unknown")) {
				out.push({ name: `alt+${inner.name}` });
				i += 2;
				continue;
			}
			out.push({ name: "escape" });
			i++;
			continue;
		}
		const ctl = control(c);
		if (ctl) {
			out.push({ name: ctl });
			i++;
			continue;
		}
		// A run of printable text: one key per grapheme.
		let j = i;
		while (j < data.length && data.charCodeAt(j) >= 0x20 && data.charCodeAt(j) !== 0x7f && data.charCodeAt(j) !== 0x1b) j++;
		for (const { segment } of graphemes.segment(data.slice(i, j)))
			out.push({ name: segment === " " ? "space" : segment, text: segment });
		i = j;
	}
	return out;
}
