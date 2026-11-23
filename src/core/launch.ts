// Build what to run, then hand it over: in place (the wrapper execs it, so this
// terminal becomes the agent) or detached in a new terminal window.
import * as fs from "node:fs";
import * as path from "node:path";
import { agent as findAgent } from "../agents";
import { isOmarchy, omarchyCommand, omarchyExec, WINDOW_CLASS } from "../integrations/omarchy";
import { appArgs, loadApps } from "./config";
import type { App, ExecSpec, Session } from "./types";
import { stateDir, which } from "./util";

export type Where = "here" | "window";

function agentSpec(agentId: string, args: string[], cwd: string, where: Where): ExecSpec {
	if (isOmarchy()) return omarchyExec(agentId, args, cwd, where === "here");
	const a = findAgent(agentId);
	const argv = [a?.bin ?? agentId, ...(a?.launchArgs ?? []), ...args];
	return { argv: where === "here" ? argv : inTerminal(argv), cwd: cwd || undefined };
}

function inTerminal(argv: string[]): string[] {
	if (which("xdg-terminal-exec")) return ["xdg-terminal-exec", `--app-id=${WINDOW_CLASS}`, "-e", ...argv];
	for (const t of ["foot", "kitty", "alacritty", "wezterm", "ghostty"]) if (which(t)) return [t, "-e", ...argv];
	return argv;
}

/** Launch an app from the list, with launch-sheet arguments. */
export function appSpec(app: App, where: Where, extra: string[] = []): ExecSpec {
	if (app.agent) return agentSpec(app.agent, [...appArgs(app, app.agent), ...extra], "", where);
	const command = app.command ?? "";
	if (isOmarchy()) return omarchyCommand(command, where === "here");
	return { argv: where === "here" ? ["bash", "-lc", command] : inTerminal(["bash", "-lc", command]) };
}

export function resumeSpec(s: Session, where: Where, extra: string[] = []): ExecSpec {
	const a = findAgent(s.agent);
	const resume = a?.sessions?.resumeArgs(s.id) ?? [];
	const cwd = s.cwd && fs.existsSync(s.cwd) ? s.cwd : "";
	return agentSpec(s.agent, [...appArgs(undefined, s.agent), ...extra, ...resume], cwd, where);
}

export function defaultSpec(agentId: string, where: Where): ExecSpec {
	const app = loadApps().find(a => a.agent === agentId);
	return agentSpec(agentId, appArgs(app, agentId), "", where);
}

/** `env` arguments for a spec: [-C dir] [K=V…] argv… */
export function envArgs(spec: ExecSpec): string[] {
	return [
		...(spec.cwd ? ["-C", spec.cwd] : []),
		...Object.entries(spec.env ?? {}).map(([k, v]) => `${k}=${v}`),
		...spec.argv,
	];
}

/**
 * Run in place. Under the `topdown` wrapper, write the spec for it to exec
 * (no runtime left behind the agent); otherwise run it as a child and exit with
 * its status.
 */
export async function execHere(spec: ExecSpec, title?: string): Promise<never> {
	if (title) process.stdout.write(`\x1b]2;${title}\x07`);
	const handoff = process.env.TOPDOWN_EXEC_FILE;
	if (handoff) {
		fs.writeFileSync(handoff, envArgs(spec).map(a => `${a}\0`).join(""));
		process.exit(0);
	}
	const p = Bun.spawn(["env", ...envArgs(spec)], { stdio: ["inherit", "inherit", "inherit"] });
	process.exit(await p.exited);
}

/**
 * A separate window. omarchy-launch-tui ends in `systemd-run --scope`, which
 * stays in the foreground until that terminal closes, so detach fully (setsid
 * -f) and log the new terminal's chatter instead of printing it here.
 */
export function execWindow(spec: ExecSpec): void {
	const log = path.join(stateDir(), "launch.log");
	fs.mkdirSync(path.dirname(log), { recursive: true });
	const fd = fs.openSync(log, "a");
	Bun.spawnSync(["setsid", "-f", "env", ...envArgs(spec)], { stdin: "ignore", stdout: fd, stderr: fd });
	fs.closeSync(fd);
}
