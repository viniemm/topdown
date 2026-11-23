// Width-aware text helpers. Widths come from Bun.stringWidth (ANSI escapes are
// zero-width, wide characters count as 2); cutting goes by grapheme.

const TOKEN = /\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)/y;
const graphemes = new Intl.Segmenter(undefined, { granularity: "grapheme" });
/** Resets what a cut may leave open (colour, bold, italic, underline), but not a background. */
const CLOSE = "\x1b[39;22;23;24m";

export const width = (s: string): number => Bun.stringWidth(s);

/** Split into ANSI escape tokens and text runs, in order. */
function* tokens(s: string): Generator<{ esc: boolean; text: string }> {
	let i = 0;
	let run = "";
	while (i < s.length) {
		if (s.charCodeAt(i) === 0x1b) {
			TOKEN.lastIndex = i;
			const m = TOKEN.exec(s);
			if (m) {
				if (run) yield { esc: false, text: run };
				run = "";
				yield { esc: true, text: m[0] };
				i += m[0].length;
				continue;
			}
		}
		run += s[i++];
	}
	if (run) yield { esc: false, text: run };
}

/** Cut `s` to at most `max` columns, ending with `ellipsis` when something was dropped. */
export function truncate(s: string, max: number, ellipsis = "…"): string {
	if (max <= 0) return "";
	if (width(s) <= max) return s;
	const room = max - width(ellipsis);
	let out = "";
	let used = 0;
	for (const t of tokens(s)) {
		if (t.esc) {
			out += t.text;
			continue;
		}
		for (const { segment } of graphemes.segment(t.text)) {
			const w = width(segment);
			if (used + w > room) return `${out}${CLOSE}${room >= 0 ? ellipsis : ""}`;
			out += segment;
			used += w;
		}
	}
	return out;
}

/** Exactly `w` columns: cut, or padded with spaces. */
export function fit(s: string, w: number): string {
	const cut = truncate(s, w);
	return cut + " ".repeat(Math.max(0, w - width(cut)));
}

/** Right-align `right` after `left` within `w` columns (left is cut first). */
export function spread(left: string, right: string, w: number): string {
	const rw = width(right);
	if (rw + 1 >= w) return fit(left, w);
	return fit(left, w - rw) + right;
}

export const strip = (s: string): string => {
	let out = "";
	for (const t of tokens(s)) if (!t.esc) out += t.text;
	return out;
};
