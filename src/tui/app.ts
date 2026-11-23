// The terminal session and the renderer.
//
// Rendering follows omp's approach: a frame is an array of lines; only lines
// that changed since the last frame are rewritten, and each paint is wrapped in
// DEC synchronized output (CSI ?2026h … ?2026l) so it appears atomically.
import { type Key, parseKeys } from "./keys";

export interface Component {
	/** Exactly `height` lines, each at most `width` columns. */
	render(width: number, height: number): string[];
	handleKey(key: Key): void;
}

const SYNC_BEGIN = "\x1b[?2026h";
const SYNC_END = "\x1b[?2026l";
// Alternate screen, hidden cursor, no autowrap (a full last column must not
// scroll the screen), bracketed paste.
const ENTER = "\x1b[?1049h\x1b[?25l\x1b[?7l\x1b[?2004h\x1b[H\x1b[2J";
const LEAVE = "\x1b[?2004l\x1b[?7h\x1b[?25h\x1b[0m\x1b[?1049l";

export class App {
	#root: Component | undefined;
	#previous: string[] = [];
	#scheduled = false;
	#running = false;
	#onData = (chunk: Buffer | string) => {
		for (const key of parseKeys(chunk.toString())) this.#root?.handleKey(key);
		this.requestRender();
	};
	#onResize = () => {
		this.#previous = []; // full repaint
		process.stdout.write("\x1b[2J");
		this.requestRender();
	};
	#onExit = () => this.stop();

	get columns(): number {
		return process.stdout.columns || 80;
	}

	get rows(): number {
		return process.stdout.rows || 24;
	}

	start(root: Component): void {
		this.#root = root;
		this.#running = true;
		process.stdout.write(ENTER);
		if (process.stdin.isTTY) process.stdin.setRawMode(true);
		process.stdin.resume();
		process.stdin.on("data", this.#onData);
		process.stdout.on("resize", this.#onResize);
		// Always give the terminal back, however the process ends.
		process.on("exit", this.#onExit);
		for (const sig of ["SIGTERM", "SIGHUP"] as const) process.once(sig, () => process.exit(129));
		this.#paint();
	}

	stop(): void {
		if (!this.#running) return;
		this.#running = false;
		process.stdin.off("data", this.#onData);
		process.stdout.off("resize", this.#onResize);
		process.off("exit", this.#onExit);
		if (process.stdin.isTTY) process.stdin.setRawMode(false);
		process.stdin.pause();
		process.stdout.write(LEAVE);
	}

	/** Coalesce render requests into one paint per tick. */
	requestRender(): void {
		if (this.#scheduled || !this.#running) return;
		this.#scheduled = true;
		queueMicrotask(() => {
			this.#scheduled = false;
			if (this.#running) this.#paint();
		});
	}

	#paint(): void {
		const lines = this.#root!.render(this.columns, this.rows);
		process.stdout.write(diff(this.#previous, lines));
		this.#previous = lines;
	}
}

/** The bytes that turn frame `prev` into `next`: only changed rows, inside a sync wrapper. */
export function diff(prev: readonly string[], next: readonly string[]): string {
	let out = "";
	const n = Math.max(prev.length, next.length);
	for (let i = 0; i < n; i++) {
		if (prev[i] === next[i]) continue;
		// Clear first: after a full-width line the cursor sits on the last column, so a
		// trailing erase would wipe that column (the right border).
		out += `\x1b[${i + 1};1H\x1b[2K${next[i] ?? ""}\x1b[0m`;
	}
	return out ? SYNC_BEGIN + out + SYNC_END : "";
}
