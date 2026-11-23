// Paths, time and text helpers shared by adapters and the UI.
import * as os from "node:os";
import * as path from "node:path";

/** $HOME at call time (tests point it at a fixture home). */
export const home = (): string => process.env.HOME || os.homedir();
export const xdg = (name: "CONFIG" | "STATE" | "CACHE" | "DATA" | "RUNTIME", fallback: string): string =>
	process.env[`XDG_${name}_HOME`] || (name === "RUNTIME" ? process.env.XDG_RUNTIME_DIR || "/tmp" : path.join(home(), fallback));
export const configDir = () => path.join(xdg("CONFIG", ".config"), "topdown");
export const stateDir = () => path.join(xdg("STATE", ".local/state"), "topdown");
export const cacheDir = () => path.join(xdg("CACHE", ".cache"), "topdown");

/** Seconds since the epoch from seconds, milliseconds or an ISO string; 0 if unusable. */
export function epoch(v: unknown): number {
	if (v === null || v === undefined || v === "") return 0;
	if (typeof v === "number") return v > 1e11 ? v / 1000 : v;
	const n = Number(v);
	if (!Number.isNaN(n)) return n > 1e11 ? n / 1000 : n;
	const t = Date.parse(String(v).replace(" ", "T"));
	return Number.isNaN(t) ? 0 : t / 1000;
}

export const now = () => Date.now() / 1000;

export function span(seconds: number): string {
	const s = Math.floor(Math.abs(seconds));
	if (s < 60) return `${s}s`;
	const m = Math.floor(s / 60);
	if (m < 60) return `${m}m`;
	const h = Math.floor(m / 60);
	if (h < 24) return m % 60 ? `${h}h ${String(m % 60).padStart(2, "0")}m` : `${h}h`;
	const d = Math.floor(h / 24);
	return h % 24 ? `${d}d ${h % 24}h` : `${d}d`;
}

export const ago = (ts: number) => (ts ? `${span(now() - ts)} ago` : "?");

const two = (n: number) => String(n).padStart(2, "0");
const DAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

/** Compact local time: 21:40 today, 'Yest 14:02', 'Mon 09:10' this week, 'Nov 18' older. */
export function stamp(ts: number): string {
	if (!ts) return "";
	const t = new Date(ts * 1000);
	const today = new Date();
	const day = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
	const days = Math.round((day(today) - day(t)) / 86_400_000);
	const hm = `${two(t.getHours())}:${two(t.getMinutes())}`;
	if (days === 0) return hm;
	if (days === 1) return `Yest ${hm}`;
	if (days > 0 && days < 7) return `${DAYS[t.getDay()]} ${hm}`;
	const md = `${MONTHS[t.getMonth()]} ${two(t.getDate())}`;
	return t.getFullYear() === today.getFullYear() ? md : `${md} ${t.getFullYear()}`;
}

export function shortPath(p: string): string {
	const h = home();
	return p && p.startsWith(h) ? `~${p.slice(h.length)}` : p || "";
}

export function project(p: string): string {
	const s = (p || "").replace(/\/+$/, "");
	return s === home() ? "~" : path.basename(s) || s;
}

export function oneline(s: unknown, n: number): string {
	const t = String(s ?? "").split(/\s+/).filter(Boolean).join(" ");
	return t.length <= n ? t : `${t.slice(0, n - 1)}…`;
}

/** Text of a message's content: a string or a list of {type: "text", text} parts. */
export function textOf(content: unknown): string {
	if (typeof content === "string") return content;
	if (Array.isArray(content))
		return content
			.filter(c => c && typeof c === "object" && (c.type === "text" || c.type === "input_text"))
			.map(c => c.text ?? "")
			.join(" ");
	return "";
}

/** Something the user typed, not a harness insert (<command-…> tags, Claude's 'Caveat:' notes). */
export function isUserText(t: string): boolean {
	const s = (t || "").trimStart();
	return !!s && !s.startsWith("<") && !s.startsWith("Caveat:");
}

export function tokens(n: unknown): string {
	const v = Math.floor(Number(n) || 0);
	for (const [unit, size] of [["B", 1e9], ["M", 1e6], ["k", 1e3]] as const) if (v >= size) return `${(v / size).toFixed(1)}${unit}`;
	return String(v);
}

/** First executable named `name` on PATH, or undefined. */
export function which(name: string): string | undefined {
	return Bun.which(name, { PATH: process.env.PATH ?? "" }) ?? undefined;
}

/** Like omarchy-agent: a user install in ~/.local/bin wins over PATH. */
export function userBin(name: string): string {
	const local = path.join(home(), ".local/bin", name);
	try {
		if (require("node:fs").statSync(local).mode & 0o111) return local;
	} catch {}
	return which(name) ?? name;
}
