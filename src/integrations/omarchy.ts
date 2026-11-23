// Omarchy integration: optional. When `omarchy-agent` exists, agents launch
// through it (auto-permission flags, account pinning, ~/Work cwd, agent window
// class) and plan usage comes from the records the bar's agents plugin reads.
import * as fs from "node:fs";
import * as path from "node:path";
import { USAGE_IDS } from "../agents";
import type { App, ExecSpec } from "../core/types";
import { home, which, xdg } from "../core/util";

export const isOmarchy = () => !!which("omarchy-agent");
export const WINDOW_CLASS = "org.omarchy.agent";

// ---- usage (read-only) ----

export interface Limit {
	title?: string;
	label?: string;
	percent?: number;
	resetsAt?: string;
}
export interface UsageRecord {
	limits?: Limit[];
	balance?: { remaining?: number; funded?: number; currency?: string };
	tierLabel?: string;
	usageStatusText?: string;
	authHelpText?: string;
	limitsStale?: boolean;
	hasLocalStats?: boolean;
	todayPrompts?: number;
	todaySessions?: number;
	todayTotalTokens?: number;
	totalPrompts?: number;
	updatedAt?: string;
}

const usageDir = () => path.join(xdg("STATE", ".local/state"), "omarchy/agents/usage");

export function usageId(app: App | undefined, agentId: string): string {
	return app?.usage || USAGE_IDS[agentId] || agentId;
}

export function usageRecord(id: string): UsageRecord | undefined {
	try {
		return JSON.parse(fs.readFileSync(path.join(usageDir(), `${id}.json`), "utf8"));
	} catch {
		return undefined;
	}
}

/** The tightest limit, 0..1, for the at-a-glance meter. */
export function tightest(rec: UsageRecord | undefined): number | undefined {
	const limits = rec?.limits ?? [];
	return limits.length ? Math.max(...limits.map(l => Number(l.percent) || 0)) : undefined;
}

export async function refreshUsage(id: string): Promise<void> {
	if (!which("omarchy-agent-usage-update")) return;
	await Bun.spawn(["omarchy-agent-usage-update", id], { stdout: "ignore", stderr: "ignore" }).exited;
}

// ---- launching through omarchy-agent ----

const shimDir = () => path.join(xdg("DATA", ".local/share"), "topdown/shim");

/**
 * omarchy-agent builds each agent's command. Two stand-ins on PATH make it run
 * the picked agent rather than the default, and hand the command back so extra
 * and resume arguments are appended, the session's directory pinned, and the
 * result run in place or in a new terminal.
 */
function ensureShim(): string {
	const dir = shimDir();
	fs.mkdirSync(dir, { recursive: true });
	fs.writeFileSync(path.join(dir, "omarchy-default-agent"), '#!/bin/bash\necho "$TOPDOWN_AGENT"\n', { mode: 0o755 });
	fs.writeFileSync(
		path.join(dir, "omarchy-launch-tui"),
		`#!/bin/bash
# Leave PATH as it was, so nothing inside the agent session sees these stand-ins.
PATH=$(tr ':' '\\n' <<<"$PATH" | grep -vxF "${dir}" | paste -sd:)
app_id=--app-id=${WINDOW_CLASS}
[[ $1 == --app-id=* ]] && app_id=$1 && shift
cmd=("$@")
[[ -n \${TOPDOWN_ARGS:-} ]] && mapfile -t -O "\${#cmd[@]}" cmd <<<"$TOPDOWN_ARGS"
# omarchy-agent moves launches from $HOME to ~/Work; a resumed session must
# start in its own project dir, so pin it on the command itself.
[[ -n \${TOPDOWN_CWD:-} ]] && cmd=(env -C "$TOPDOWN_CWD" "\${cmd[@]}")
inline=\${TOPDOWN_INLINE:-}
unset TOPDOWN_AGENT TOPDOWN_ARGS TOPDOWN_CWD TOPDOWN_INLINE
[[ -n $inline ]] && exec "\${cmd[@]}"
exec omarchy-launch-tui "$app_id" "\${cmd[@]}"
`,
		{ mode: 0o755 },
	);
	return dir;
}

/** Run `agent` the way omarchy-agent would, plus `args`, optionally in `cwd`. */
export function omarchyExec(agentId: string, args: string[], cwd: string, inline: boolean): ExecSpec {
	const shim = ensureShim();
	return {
		argv: ["omarchy-agent"],
		env: {
			TOPDOWN_AGENT: agentId,
			TOPDOWN_ARGS: args.join("\n"),
			TOPDOWN_CWD: cwd,
			...(inline ? { TOPDOWN_INLINE: "1" } : {}),
			PATH: `${shim}:${process.env.PATH ?? ""}`,
		},
		// Same convention as omarchy-agent: launches from $HOME start in ~/Work.
		cwd: process.cwd() === home() && fs.existsSync(path.join(home(), "Work")) ? path.join(home(), "Work") : undefined,
	};
}

/** A plain command in a terminal with the agent window class. */
export function omarchyCommand(command: string, inline: boolean): ExecSpec {
	return inline
		? { argv: ["bash", "-lc", command] }
		: { argv: ["omarchy-launch-tui", `--app-id=${WINDOW_CLASS}`, "bash", "-lc", command] };
}
