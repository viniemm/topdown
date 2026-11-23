// Hyprland: find and focus the window an agent process runs in.
import * as fs from "node:fs";
import { which } from "../core/util";

export const hasHyprland = () => !!process.env.HYPRLAND_INSTANCE_SIGNATURE && !!which("hyprctl");

/** The window address of the terminal that owns `pid`, walking up its parents. */
export function windowForPid(pid: number): string | undefined {
	if (!hasHyprland()) return undefined;
	let clients: Map<number, string>;
	try {
		const out = Bun.spawnSync(["hyprctl", "clients", "-j"]).stdout.toString();
		clients = new Map((JSON.parse(out) as { pid: number; address: string }[]).map(c => [c.pid, c.address]));
	} catch {
		return undefined;
	}
	let p = pid;
	for (let hops = 0; p > 1 && hops < 64; hops++) {
		const addr = clients.get(p);
		if (addr) return addr;
		try {
			// /proc/<pid>/stat: "pid (comm) state ppid …"; comm may contain spaces.
			const stat = fs.readFileSync(`/proc/${p}/stat`, "utf8");
			p = Number(stat.slice(stat.lastIndexOf(")") + 2).split(" ")[1]);
		} catch {
			return undefined;
		}
	}
	return undefined;
}

export function focusWindow(address: string): void {
	Bun.spawnSync(["hyprctl", "dispatch", `hl.dsp.focus({ window = "address:${address}" })`]);
}
