// Colours: semantic roles plus colour coding, in true colour from the palette
// (see palette.ts: the Omarchy theme, else the terminal's own colours). Shades
// such as the selection background, dim text and borders are blended from the
// palette, so they stay right in any theme. With no palette or no true-colour
// terminal, everything falls back to the 16 ANSI colours.
//
// ~/.config/topdown/theme.json overrides with hex colours: any role
// ("accent", "border", "dim", "muted", "success", "warning", "error",
// "selected") and per-agent colours: {"agents": {"claude": "#d97757"}}.
import * as fs from "node:fs";
import * as path from "node:path";
import { configDir } from "../core/util";
import { hex, mix, type Palette, type RGB } from "./palette";

export type Role = "accent" | "border" | "dim" | "muted" | "success" | "warning" | "error";
type Hue = "orange" | "green" | "blue" | "magenta" | "cyan" | "yellow" | "red";

// Agents' preferred hues (roughly their brands), resolved against the theme.
const AGENT_HUE: Record<string, Hue> = {
	claude: "orange", codex: "green", opencode: "cyan", copilot: "magenta", omp: "blue", pi: "cyan",
	hermes: "magenta", agy: "yellow", grok: "red", crush: "magenta", "cursor-agent": "blue", amp: "red",
};
const HUES: Hue[] = ["orange", "green", "blue", "magenta", "cyan", "yellow", "red"];
const ANSI_HUE: Record<Hue, string> = { orange: "33", green: "32", blue: "34", magenta: "35", cyan: "36", yellow: "93", red: "31" };
const ANSI_ROLE: Record<Role, string> = {
	accent: "34", border: "34", dim: "2", muted: "37", success: "32", warning: "33", error: "31",
};

let pal: Palette | undefined;
let truecolor = false;
let roles: Record<Role, RGB> | undefined;
let selectedBg: RGB | undefined;
const agentColor = new Map<string, RGB | Hue>();
let agentOverrides: Record<string, RGB> = {};

const sgr = (c: RGB, bg = false) => `${bg ? 48 : 38};2;${c[0]};${c[1]};${c[2]}`;
const hueRgb = (h: Hue): RGB => pal![h];

export function initTheme(palette: Palette | undefined): void {
	pal = palette;
	truecolor = !!palette && /truecolor|24bit/i.test(process.env.COLORTERM ?? "");
	roles = undefined;
	selectedBg = undefined;
	agentOverrides = {};
	if (truecolor && pal) {
		roles = {
			accent: pal.accent,
			border: mix(pal.bg, pal.accent, 0.55),
			dim: mix(pal.fg, pal.bg, 0.5),
			muted: mix(pal.fg, pal.bg, 0.22),
			success: pal.green,
			warning: pal.yellow,
			error: pal.red,
		};
		selectedBg = mix(pal.bg, pal.accent, 0.2);
	}
	try {
		const o = JSON.parse(fs.readFileSync(path.join(configDir(), "theme.json"), "utf8"));
		if (roles) {
			for (const r of Object.keys(roles) as Role[]) roles[r] = hex(o[r]) ?? roles[r];
			selectedBg = hex(o.selected) ?? selectedBg;
		}
		for (const [id, v] of Object.entries(o.agents ?? {})) {
			const c = hex(v);
			if (c) agentOverrides[id] = c;
		}
	} catch {}
}

/** Give each listed agent a colour: its preferred hue unless an earlier agent has it. */
export function assignAgentColors(ids: readonly string[]): void {
	agentColor.clear();
	const used = new Set<Hue>();
	for (const id of ids) {
		if (agentOverrides[id]) {
			agentColor.set(id, agentOverrides[id]!);
			continue;
		}
		const want = AGENT_HUE[id];
		const hue = want && !used.has(want) ? want : (HUES.find(h => !used.has(h)) ?? HUES[hashOf(id) % HUES.length]!);
		used.add(hue);
		agentColor.set(id, hue);
	}
}

function hashOf(s: string): number {
	let h = 2166136261;
	for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 16777619);
	return h >>> 0;
}

function paint(c: RGB | Hue, s: string): string {
	if (Array.isArray(c)) return truecolor ? `\x1b[${sgr(c)}m${s}\x1b[39m` : s;
	return truecolor ? `\x1b[${sgr(hueRgb(c))}m${s}\x1b[39m` : `\x1b[${ANSI_HUE[c]}m${s}\x1b[39m`;
}

export const theme = {
	fg(role: Role, s: string): string {
		if (roles) return `\x1b[${sgr(roles[role])}m${s}\x1b[39m`;
		const code = ANSI_ROLE[role];
		return `\x1b[${code}m${s}\x1b[${code === "2" ? "22" : "39"}m`;
	},
	bold: (s: string) => `\x1b[1m${s}\x1b[22m`,
	inverse: (s: string) => `\x1b[7m${s}\x1b[27m`,
	/** Selection background; inner colour changes never reset it (only \x1b[49m would). */
	selected: (s: string) => (selectedBg ? `\x1b[${sgr(selectedBg, true)}m${s}\x1b[49m` : `\x1b[7m${s}\x1b[27m`),

	// ---- colour coding ----

	/** Each agent in its own colour, consistently everywhere. */
	agent(id: string, s = id): string {
		if (!agentColor.has(id)) assignAgentColors([...agentColor.keys(), id]);
		return paint(agentColor.get(id)!, s);
	},
	/** A project's colour: a stable hue from its name, softened toward the text colour. */
	project(name: string, s = name): string {
		const hue = HUES[hashOf(name) % HUES.length]!;
		if (!truecolor || !pal) return theme.fg("muted", s);
		return paint(mix(hueRgb(hue), pal.fg, 0.45), s);
	},
	/** Timestamps fade with age: today bright, this week muted, older dim. */
	age(ts: number, s: string): string {
		const days = (Date.now() / 1000 - ts) / 86_400;
		if (!truecolor || !pal) return theme.fg(days < 1 ? "muted" : "dim", s);
		const t = days < 1 ? 0.1 : days < 7 ? 0.35 : days < 30 ? 0.55 : 0.68;
		return paint(mix(pal.fg, pal.bg, t), s);
	},
	/** 0..1 → green, through yellow, to red (usage meters). */
	level(p: number, s: string): string {
		const v = Math.max(0, Math.min(1, p));
		if (!truecolor || !pal) return theme.fg(v < 0.6 ? "success" : v < 0.85 ? "warning" : "error", s);
		const c = v < 0.6 ? mix(pal.green, pal.yellow, v / 0.6) : mix(pal.yellow, pal.red, (v - 0.6) / 0.4);
		return paint(c, s);
	},
};
