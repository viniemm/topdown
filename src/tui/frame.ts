// The hub layout, after omp's model picker:
//
//   ╭─ title ──────┬────────────────────────╮
//   │ ▸ ▦ Apps   4 │ body                   │
//   │ ──────────── │                        │
//   ├──────────────┴────────────────────────┤
//   │ footer key hints                      │
//   ╰───────────────────────────────────────╯
import { theme } from "./theme";
import { fit, spread, truncate, width } from "./text";

export interface SidebarEntry<K extends string = string> {
	id: string;
	kind: K | "separator";
	label: string;
	annotation?: string;
}

export interface SidebarStyle {
	icon: string;
	annotation: string;
	muted?: boolean;
}

export interface Selection {
	id: string;
	focused: boolean;
}

const b = (s: string) => theme.fg("border", s);

export class HubFrame<K extends string = string> {
	#scroll = 0;

	constructor(
		readonly title: string,
		readonly bounds: { min: number; max: number },
		readonly renderSidebar: (width: number, rows: number) => string[],
		readonly renderBody: (width: number, rows: number) => readonly string[],
	) {}

	sidebarWidth(entries: readonly SidebarEntry<K>[]): number {
		let longest = 0;
		for (const e of entries) longest = Math.max(longest, width(e.label) + width(e.annotation ?? "") + 6);
		return Math.max(this.bounds.min, Math.min(this.bounds.max, longest));
	}

	/** Sidebar rows: cursor, icon, label, right-aligned annotation; the active row in bold accent. */
	sidebar(entries: readonly SidebarEntry<K>[], w: number, rows: number, sel: Selection, style: (e: SidebarEntry<K>) => SidebarStyle): string[] {
		const active = Math.max(0, entries.findIndex(e => e.id === sel.id));
		if (active < this.#scroll) this.#scroll = active;
		if (active >= this.#scroll + rows) this.#scroll = active - rows + 1;
		this.#scroll = Math.max(0, Math.min(this.#scroll, Math.max(0, entries.length - rows)));
		const out: string[] = [];
		for (const e of entries.slice(this.#scroll, this.#scroll + rows)) {
			if (e.kind === "separator") {
				out.push(b("─".repeat(w)));
				continue;
			}
			const isActive = e.id === sel.id;
			const s = style(e);
			const cursor = isActive && sel.focused ? theme.fg("accent", "▸") : " ";
			const label = s.muted ? theme.fg("dim", e.label) : isActive ? theme.bold(theme.fg("accent", e.label)) : e.label;
			const line = spread(`${cursor} ${s.icon} ${label}`, s.annotation, w);
			out.push(isActive && sel.focused ? theme.selected(line) : line);
		}
		while (out.length < rows) out.push(" ".repeat(w));
		return out;
	}

	/** The whole screen: `height` lines of exactly `width` columns. */
	render(totalWidth: number, height: number, entries: readonly SidebarEntry<K>[], footer: string): string[] {
		const W = Math.max(40, totalWidth);
		const rows = Math.max(3, height - 4);
		const sw = Math.min(this.sidebarWidth(entries), Math.max(10, Math.floor((W - 7) / 3)));
		const bw = W - sw - 7;
		const left = this.renderSidebar(sw, rows);
		const body = this.renderBody(bw, rows);

		const name = ` ${this.title} `;
		const top = b(`╭─${truncate(name, sw)}${"─".repeat(Math.max(0, sw + 1 - width(name)))}┬${"─".repeat(bw + 2)}╮`);
		const lines = [top];
		for (let i = 0; i < rows; i++) lines.push(`${b("│")} ${fit(left[i] ?? "", sw)} ${b("│")} ${fit(body[i] ?? "", bw)} ${b("│")}`);
		lines.push(b(`├${"─".repeat(sw + 2)}┴${"─".repeat(bw + 2)}┤`));
		lines.push(`${b("│")} ${fit(footer, W - 4)} ${b("│")}`);
		lines.push(b(`╰${"─".repeat(W - 2)}╯`));
		return lines;
	}
}
