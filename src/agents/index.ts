// Every agent topdown knows. Full adapters first; the rest launch only (M2 adds more).
import type { AgentAdapter } from "../core/types";
import { which } from "../core/util";
import { claude } from "./claude";
import { omp, pi } from "./pi";
import { agy, hermes } from "./sqlite-agents";

const launchOnly: [id: string, label: string][] = [
	["codex", "Codex"],
	["opencode", "OpenCode"],
	["copilot", "GitHub Copilot"],
	["crush", "Crush"],
	["grok", "Grok"],
	["cursor-agent", "Cursor Agent"],
	["amp", "Amp"],
	["kimi", "Kimi"],
	["qwen", "Qwen Code"],
	["kiro-cli", "Kiro"],
	["kilo", "Kilo"],
	["devin", "Devin"],
	["muse", "Muse"],
	["ori", "Ori"],
	["dsh", "DeepSeek Shell"],
	["mcode", "MiniMax Code"],
	["freebuff", "Freebuff"],
	["commandcode", "Command Code"],
	["openclaw", "OpenClaw"],
];

export const AGENTS: AgentAdapter[] = [
	claude,
	omp,
	pi,
	hermes,
	agy,
	...launchOnly.map(([id, label]) => ({ id, label, bin: id })),
];

const byId = new Map(AGENTS.map(a => [a.id, a]));
export const agent = (id: string | undefined): AgentAdapter | undefined => (id ? byId.get(id) : undefined);
export const installed = (a: AgentAdapter) => !!which(a.bin);

/** Omarchy usage record ids that differ from the agent id. */
export const USAGE_IDS: Record<string, string> = { "cursor-agent": "cursor", mcode: "minimax", dsh: "deepseek", ori: "openrouter" };
