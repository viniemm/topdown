// Sessions kept one file per conversation (Claude, Pi, omp): cheap discovery by
// mtime, parse only what is shown, cache parses by path + mtime + size.
import * as fs from "node:fs";
import * as path from "node:path";
import { Glob } from "bun";
import type { Session } from "./types";
import { cacheDir, home, xdg } from "./util";

export interface Parsed {
	title: string;
	cwd: string;
	prompts: string[];
}

const CACHE_VERSION = 3;
type Entry = Parsed & { mtime: number; size: number };

class ParseCache {
	#data: Record<string, Entry> = {};
	#dirty = false;
	#file = path.join(cacheDir(), "sessions.json");

	constructor() {
		try {
			const raw = JSON.parse(fs.readFileSync(this.#file, "utf8"));
			if (raw.version === CACHE_VERSION) this.#data = raw.sessions ?? {};
		} catch {}
	}

	lookup(file: string, st: fs.Stats, parse: (file: string) => Parsed): Entry {
		const hit = this.#data[file];
		if (hit && hit.mtime === st.mtimeMs && hit.size === st.size) return hit;
		const entry = { ...parse(file), mtime: st.mtimeMs, size: st.size };
		this.#data[file] = entry;
		this.#dirty = true;
		return entry;
	}

	save(): void {
		if (!this.#dirty) return;
		fs.mkdirSync(path.dirname(this.#file), { recursive: true });
		fs.writeFileSync(`${this.#file}.tmp`, JSON.stringify({ version: CACHE_VERSION, sessions: this.#data }));
		fs.renameSync(`${this.#file}.tmp`, this.#file);
		this.#dirty = false;
	}
}

let shared: ParseCache | undefined;
const cache = () => (shared ??= new ParseCache());
export const saveCache = () => shared?.save();

export interface FileStoreSpec {
	agent: string;
	/** Glob under $HOME. */
	pattern: string;
	parse: (file: string) => Parsed;
	/** Session id: the file stem or the full path. */
	id: "stem" | "path";
}

export function globFiles(pattern: string): string[] {
	const base = home();
	try {
		return [...new Glob(pattern).scanSync({ cwd: base, absolute: true, onlyFiles: true })];
	} catch {
		return [];
	}
}

function toSession(spec: FileStoreSpec, file: string, st?: fs.Stats): Session | undefined {
	let stat: fs.Stats;
	try {
		stat = st ?? fs.statSync(file);
	} catch {
		return undefined;
	}
	const info = cache().lookup(file, stat, spec.parse);
	if (!info.title) return undefined;
	const id = spec.id === "stem" ? path.basename(file, ".jsonl") : file;
	return { agent: spec.agent, id, cwd: info.cwd, title: info.title, when: stat.mtimeMs / 1000, prompts: info.prompts, path: file };
}

/** Newest-first sessions; only the files that end up shown are parsed. */
export function listFiles(spec: FileStoreSpec, limit: number): Session[] {
	const files: [number, string, fs.Stats][] = [];
	for (const f of globFiles(spec.pattern)) {
		try {
			const st = fs.statSync(f);
			files.push([st.mtimeMs, f, st]);
		} catch {}
	}
	files.sort((a, b) => b[0] - a[0]);
	const out: Session[] = [];
	for (const [, f, st] of files) {
		if (out.length >= limit) break;
		const s = toSession(spec, f, st);
		if (s) out.push(s);
	}
	return out;
}

export function fileFor(spec: FileStoreSpec, id: string): string | undefined {
	if (spec.id === "path") return fs.existsSync(id) ? id : undefined;
	const dir = path.dirname(spec.pattern);
	return globFiles(path.join(dir, `${id}.jsonl`))[0];
}

export function getFile(spec: FileStoreSpec, id: string): Session | undefined {
	const f = fileFor(spec, id);
	return f ? toSession(spec, f) : undefined;
}

/** Read a JSONL file line by line, calling `fn` with each raw line. */
export function eachLine(file: string, fn: (line: string, n: number) => boolean | void): void {
	const text = fs.readFileSync(file, "utf8");
	let start = 0;
	let n = 0;
	while (start < text.length) {
		let end = text.indexOf("\n", start);
		if (end === -1) end = text.length;
		if (fn(text.slice(start, end), n++) === false) return;
		start = end + 1;
	}
}

export function appendJsonl(file: string, entry: unknown): void {
	fs.appendFileSync(file, `${JSON.stringify(entry)}\n`);
}

/**
 * Move files to the home Trash (restorable from any file manager), following
 * the freedesktop.org Trash spec directly: no gvfs daemon needed, and it uses
 * this process's own $XDG_DATA_HOME. Files on another filesystem go through
 * `gio trash`, which knows the per-mount .Trash-$UID folders.
 */
export async function trash(...files: string[]): Promise<void> {
	const bin = path.join(xdg("DATA", ".local/share"), "Trash");
	for (const d of ["files", "info"]) fs.mkdirSync(path.join(bin, d), { recursive: true });
	for (const file of files) {
		const base = path.basename(file);
		const ext = path.extname(base);
		const stem = ext ? base.slice(0, -ext.length) : base;
		for (let n = 1; ; n++) {
			const name = n === 1 ? base : `${stem}.${n}${ext}`;
			const info = path.join(bin, "info", `${name}.trashinfo`);
			let fd: number;
			try {
				fd = fs.openSync(info, "wx"); // claims the name atomically
			} catch {
				continue;
			}
			const when = new Date();
			const local = new Date(when.getTime() - when.getTimezoneOffset() * 60_000).toISOString().slice(0, 19);
			const encoded = path.resolve(file).split("/").map(encodeURIComponent).join("/");
			fs.writeSync(fd, `[Trash Info]\nPath=${encoded}\nDeletionDate=${local}\n`);
			fs.closeSync(fd);
			try {
				fs.renameSync(file, path.join(bin, "files", name));
			} catch (e) {
				fs.rmSync(info, { force: true });
				if ((e as NodeJS.ErrnoException).code !== "EXDEV") throw e;
				const p = Bun.spawn(["gio", "trash", file], { stderr: "pipe" });
				if ((await p.exited) !== 0) throw new Error(`gio trash failed: ${await new Response(p.stderr).text()}`);
			}
			break;
		}
	}
}
