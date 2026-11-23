// Where colours come from, best first:
//   1. Omarchy's current theme (~/.local/state/omarchy/current/theme/colors.toml):
//      its semantic palette (accent, fg/bg, muted, red…cyan, orange), so topdown
//      changes with `omarchy theme set`.
//   2. The terminal itself: OSC 10/11 (fg/bg) and OSC 4 (the 16 colours) queries,
//      so any themed terminal works without Omarchy.
//   3. Nothing known: the 16 ANSI colour codes (handled in theme.ts).
import * as fs from "node:fs";
import * as path from "node:path";
import { home } from "../core/util";

export type RGB = [number, number, number];

export interface Palette {
	source: "omarchy" | "terminal";
	fg: RGB;
	bg: RGB;
	accent: RGB;
	red: RGB;
	green: RGB;
	yellow: RGB;
	blue: RGB;
	magenta: RGB;
	cyan: RGB;
	orange: RGB;
	/** Bright variants, for more distinct agent/project hues. */
	bright: RGB[];
}

export function hex(v: unknown): RGB | undefined {
	const m = typeof v === "string" ? /^#?([0-9a-f]{6})$/i.exec(v.trim()) : null;
	if (!m) return undefined;
	const n = Number.parseInt(m[1]!, 16);
	return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

export const mix = (a: RGB, b: RGB, t: number): RGB =>
	[0, 1, 2].map(i => Math.round(a[i]! + (b[i]! - a[i]!) * t)) as RGB;

// ---- 1. Omarchy theme ----

function readOmarchy(): Palette | undefined {
	const state = process.env.XDG_STATE_HOME || path.join(home(), ".local/state");
	let text: string;
	try {
		text = fs.readFileSync(path.join(state, "omarchy/current/theme/colors.toml"), "utf8");
	} catch {
		return undefined;
	}
	const kv: Record<string, RGB> = {};
	for (const line of text.split("\n")) {
		const m = /^\s*([a-z_0-9]+)\s*=\s*"(#[0-9a-fA-F]{6})"/.exec(line);
		if (m) kv[m[1]!] = hex(m[2])!;
	}
	const fg = kv.fg ?? kv.foreground;
	const bg = kv.bg ?? kv.background;
	if (!fg || !bg) return undefined;
	const c = (name: string, n: number) => kv[name] ?? kv[`color${n}`] ?? fg;
	return {
		source: "omarchy",
		fg,
		bg,
		accent: kv.accent ?? c("blue", 4),
		red: c("red", 1),
		green: c("green", 2),
		yellow: c("yellow", 3),
		blue: c("blue", 4),
		magenta: c("magenta", 5),
		cyan: c("cyan", 6),
		orange: kv.orange ?? c("yellow", 3),
		bright: [9, 10, 11, 12, 13, 14].map(n => kv[`color${n}`] ?? fg),
	};
}

// ---- 2. Ask the terminal ----

/** "rgb:RRRR/GGGG/BBBB" (1–4 hex digits per channel) → RGB. */
function oscRgb(s: string): RGB | undefined {
	const m = /rgb:([0-9a-f]{1,4})\/([0-9a-f]{1,4})\/([0-9a-f]{1,4})/i.exec(s);
	if (!m) return undefined;
	return [m[1]!, m[2]!, m[3]!].map(h => Math.round((Number.parseInt(h, 16) / (16 ** h.length - 1)) * 255)) as RGB;
}

/**
 * Query fg, bg and the 16 colours. A DA1 request goes last: every terminal
 * answers it, so its reply means all the colour replies that are coming have
 * arrived (no fixed wait). Gives up after `timeoutMs`.
 */
async function queryTerminal(timeoutMs = 300): Promise<Palette | undefined> {
	if (!process.stdin.isTTY || !process.stdout.isTTY) return undefined;
	const ansi: (RGB | undefined)[] = [];
	let fg: RGB | undefined;
	let bg: RGB | undefined;
	let buf = "";
	const done = Promise.withResolvers<void>();
	const onData = (d: Buffer) => {
		buf += d.toString("latin1");
		for (const m of buf.matchAll(/\x1b\](\d+);(?:(\d+);)?(rgb:[0-9a-f/]+)(?:\x07|\x1b\\)/gi)) {
			const rgb = oscRgb(m[3]!);
			if (m[1] === "10") fg = rgb;
			else if (m[1] === "11") bg = rgb;
			else if (m[1] === "4" && m[2]) ansi[Number(m[2])] = rgb;
		}
		if (/\x1b\[\?[\d;]*c/.test(buf)) done.resolve();
	};
	const wasRaw = process.stdin.isRaw;
	process.stdin.setRawMode(true);
	process.stdin.on("data", onData);
	process.stdin.resume();
	const queries = ["\x1b]10;?\x1b\\", "\x1b]11;?\x1b\\"];
	for (let n = 0; n < 16; n++) queries.push(`\x1b]4;${n};?\x1b\\`);
	process.stdout.write(`${queries.join("")}\x1b[c`);
	const timer = setTimeout(() => done.resolve(), timeoutMs);
	await done.promise;
	clearTimeout(timer);
	process.stdin.off("data", onData);
	process.stdin.pause();
	process.stdin.setRawMode(wasRaw);
	if (!fg || !bg || ansi.filter(Boolean).length < 8) return undefined;
	const a = (n: number) => ansi[n] ?? fg!;
	return {
		source: "terminal",
		fg,
		bg,
		accent: a(4),
		red: a(1),
		green: a(2),
		yellow: a(3),
		blue: a(4),
		magenta: a(5),
		cyan: a(6),
		orange: mix(a(1), a(3), 0.5),
		bright: [9, 10, 11, 12, 13, 14].map(a),
	};
}

export async function detectPalette(): Promise<Palette | undefined> {
	return readOmarchy() ?? (await queryTerminal());
}
