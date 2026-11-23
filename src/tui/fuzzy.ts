// Fuzzy ranking: every space-separated term must match as a subsequence
// (like fzf's extended mode); matches at word starts and in a row score higher,
// as do earlier ones. Case-insensitive.

const BOUNDARY = /[\s/_.\-:]/;

/** Score of `term` in `text`, or -1 when it doesn't match. */
export function score(term: string, text: string): number {
	const t = text.toLowerCase();
	const q = term.toLowerCase();
	if (!q) return 0;
	// A plain substring beats any scattered match.
	const at = t.indexOf(q);
	if (at !== -1) return 1000 - at + (at === 0 || BOUNDARY.test(t[at - 1]!) ? 200 : 0);
	let s = 0;
	let ti = 0;
	let prev = -2;
	for (const ch of q) {
		const found = t.indexOf(ch, ti);
		if (found === -1) return -1;
		s += 10;
		if (found === prev + 1) s += 15;
		if (found === 0 || BOUNDARY.test(t[found - 1]!)) s += 20;
		s -= Math.min(found - ti, 10);
		prev = found;
		ti = found + 1;
	}
	return s;
}

/** Items matching every term of `query`, best first (ties keep their order). */
export function rank<T>(items: readonly T[], query: string, text: (item: T) => string): T[] {
	const terms = query.split(/\s+/).filter(Boolean);
	if (!terms.length) return [...items];
	const scored: { item: T; s: number; i: number }[] = [];
	items.forEach((item, i) => {
		const t = text(item);
		let total = 0;
		for (const term of terms) {
			const s = score(term, t);
			if (s < 0) return;
			total += s;
		}
		scored.push({ item, s: total, i });
	});
	return scored.sort((a, b) => b.s - a.s || a.i - b.i).map(x => x.item);
}
