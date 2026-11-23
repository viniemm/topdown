// The user's app list and default agent.
//   ~/.config/topdown/apps.json      the list (migrated from Omarchy's agent-launcher.json)
//   default agent: Omarchy's ~/.config/omarchy/defaults/agent when on Omarchy, else ~/.config/topdown/default
import * as fs from "node:fs";
import * as path from "node:path";
import { agent as findAgent, installed } from "../agents";
import { isOmarchy } from "../integrations/omarchy";
import type { App } from "./types";
import { configDir, home } from "./util";

export const appsFile = () => path.join(configDir(), "apps.json");
const legacyFile = () => path.join(home(), ".config/omarchy/agent-launcher.json");
const defaultFile = () =>
	isOmarchy() ? path.join(home(), ".config/omarchy/defaults/agent") : path.join(configDir(), "default");

function seed(): App[] {
	// First run: the installed agents topdown fully supports.
	return ["claude", "codex", "opencode", "omp", "pi", "hermes", "agy"]
		.map(id => findAgent(id)!)
		.filter(a => a && installed(a))
		.map(a => ({ id: a.id, label: a.label, agent: a.id }));
}

export function loadApps(): App[] {
	for (const f of [appsFile(), legacyFile()]) {
		try {
			const apps = JSON.parse(fs.readFileSync(f, "utf8"));
			if (Array.isArray(apps)) {
				if (f !== appsFile()) saveApps(apps); // migrate once
				return apps;
			}
		} catch {}
	}
	const apps = seed();
	saveApps(apps);
	return apps;
}

export function saveApps(apps: App[]): void {
	fs.mkdirSync(path.dirname(appsFile()), { recursive: true });
	fs.writeFileSync(`${appsFile()}.tmp`, `${JSON.stringify(apps, null, 2)}\n`);
	fs.renameSync(`${appsFile()}.tmp`, appsFile());
}

export function getDefault(): string {
	try {
		return fs.readFileSync(defaultFile(), "utf8").split("\n")[0]!.trim();
	} catch {
		return "";
	}
}

export function setDefault(agentId: string): void {
	fs.mkdirSync(path.dirname(defaultFile()), { recursive: true });
	fs.writeFileSync(defaultFile(), `${agentId}\n`);
}

/** Extra arguments for an app's agent: its own "args", else the adapter's defaults. */
export function appArgs(app: App | undefined, agentId: string): string[] {
	const a = app ?? loadApps().find(x => x.agent === agentId);
	const args = a?.args ?? findAgent(agentId)?.defaultArgs ?? [];
	return typeof args === "string" ? shellSplit(args) : args.map(String);
}

/** Minimal POSIX-ish word splitting for a shell-quoted "args" string. */
export function shellSplit(s: string): string[] {
	const out: string[] = [];
	let cur = "";
	let quote: string | null = null;
	let has = false;
	for (let i = 0; i < s.length; i++) {
		const c = s[i]!;
		if (quote) {
			if (c === quote) quote = null;
			else if (c === "\\" && quote === '"' && i + 1 < s.length) cur += s[++i];
			else cur += c;
		} else if (c === "'" || c === '"') {
			quote = c;
			has = true;
		} else if (c === "\\" && i + 1 < s.length) {
			cur += s[++i];
			has = true;
		} else if (/\s/.test(c)) {
			if (cur || has) out.push(cur);
			cur = "";
			has = false;
		} else {
			cur += c;
			has = true;
		}
	}
	if (cur || has) out.push(cur);
	return out;
}
