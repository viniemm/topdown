// Semantic colours. By default they map to the terminal's own 16-colour
// palette, so topdown follows whatever theme the terminal has (Omarchy themes
// set foot/alacritty/kitty colours). ~/.config/topdown/theme.json may override
// any role with a hex colour: {"accent": "#febc38", "selected": "#31363f"}.
import * as fs from "node:fs";
import * as path from "node:path";
import { configDir } from "../core/util";

export type Role = "accent" | "border" | "dim" | "muted" | "success" | "warning" | "error";

// SGR foreground codes (omp's roles; amber accent, blue borders like its dark theme).
const ANSI: Record<Role, string> = {
	accent: "33",
	border: "34",
	// Faint default text rather than bright black: bright black is also the
	// selection background, which would hide dim text on the selected row.
	dim: "2",
	muted: "37",
	success: "32",
	warning: "33",
	error: "31",
};
let fg: Record<Role, string> = { ...ANSI };
let selected = "100"; // bright-black background

function hex(v: unknown): [number, number, number] | undefined {
	const m = typeof v === "string" ? /^#?([0-9a-f]{6})$/i.exec(v.trim()) : null;
	if (!m) return undefined;
	const n = Number.parseInt(m[1]!, 16);
	return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

export function loadTheme(): void {
	fg = { ...ANSI };
	selected = "100";
	try {
		const o = JSON.parse(fs.readFileSync(path.join(configDir(), "theme.json"), "utf8"));
		for (const role of Object.keys(ANSI) as Role[]) {
			const c = hex(o[role]);
			if (c) fg[role] = `38;2;${c.join(";")}`;
		}
		const s = hex(o.selected);
		if (s) selected = `48;2;${s.join(";")}`;
	} catch {}
}

export const theme = {
	fg: (role: Role, s: string) => `\x1b[${fg[role]}m${s}\x1b[${fg[role] === "2" ? "22" : "39"}m`,
	bold: (s: string) => `\x1b[1m${s}\x1b[22m`,
	/** Selection background; inner colour changes never reset it (only \x1b[49m would). */
	selected: (s: string) => `\x1b[${selected}m${s}\x1b[49m`,
	inverse: (s: string) => `\x1b[7m${s}\x1b[27m`,
};
