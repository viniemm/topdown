// Claude Code: the first full adapter.
import * as fs from "node:fs";
import * as path from "node:path";
import { appendJsonl, eachLine, type FileStoreSpec, fileFor, getFile, globFiles, listFiles, type Parsed, trash } from "../core/file-store";
import type { AgentAdapter, OptionSpec, Session } from "../core/types";
import { home, isUserText, textOf } from "../core/util";

export function parseClaude(file: string): Parsed {
	let cwd = "";
	let custom = "";
	let ai = "";
	const prompts: string[] = [];
	eachLine(file, line => {
		// Cheap substring checks first; only promising lines are JSON-decoded.
		const i = line.indexOf('"cwd":"');
		if (i !== -1) {
			const j = line.indexOf('"', i + 7);
			if (j !== -1) cwd = line.slice(i + 7, j); // the latest wins: a moved session resumes where it lives now
		}
		if (line.slice(0, 40).includes('-title"') || (prompts.length < 5 && line.includes('"type":"user"'))) {
			let d: any;
			try {
				d = JSON.parse(line);
			} catch {
				return;
			}
			if (d.type === "custom-title") custom = d.customTitle || custom;
			else if (d.type === "ai-title") ai = d.aiTitle || ai;
			else if (d.type === "user" && !d.isMeta) {
				const t = textOf(d.message?.content);
				if (isUserText(t)) prompts.push(t.trim().slice(0, 600));
			}
		}
	});
	return { title: custom || ai || prompts[0] || "", cwd, prompts };
}

const files: FileStoreSpec = { agent: "claude", pattern: ".claude/projects/*/*.jsonl", parse: parseClaude, id: "stem" };

/** Running sessions: Claude Code records each in ~/.claude/sessions/<pid>.json. */
export function claudeLive(): Map<string, number> {
	const live = new Map<string, number>();
	for (const f of globFiles(".claude/sessions/*.json")) {
		try {
			const rec = JSON.parse(fs.readFileSync(f, "utf8"));
			const pid = Number(rec.pid);
			process.kill(pid, 0); // throws if that process is gone
			if (rec.sessionId) live.set(String(rec.sessionId), pid);
		} catch {}
	}
	return live;
}

// Flags from `claude --help` (2.1.330). The first choice is Claude's own default.
const options: OptionSpec[] = [
	{
		key: "model",
		label: "Model",
		choices: ["default", "opus", "sonnet", "haiku", "fable"],
		help: "--model (alias or full id)",
		args: v => ["--model", v],
	},
	{
		key: "effort",
		label: "Effort",
		choices: ["default", "low", "medium", "high", "xhigh", "max"],
		help: "--effort",
		args: v => ["--effort", v],
	},
	{
		key: "permission",
		label: "Permissions",
		choices: ["default", "auto", "manual", "acceptEdits", "plan", "dontAsk", "bypassPermissions"],
		help: "--permission-mode",
		args: v => ["--permission-mode", v],
	},
	{
		key: "fork",
		label: "On resume",
		choices: ["continue", "fork"],
		help: "--fork-session: resume into a new session id",
		resumeOnly: true,
		args: v => (v === "fork" ? ["--fork-session"] : []),
	},
	{
		key: "chrome",
		label: "Chrome",
		choices: ["default", "on", "off"],
		help: "--chrome / --no-chrome",
		args: v => [v === "on" ? "--chrome" : "--no-chrome"],
	},
];

export const claude: AgentAdapter = {
	id: "claude",
	label: "Claude Code",
	bin: "claude",
	// Omarchy's omarchy-agent uses this too; without Omarchy topdown adds it itself.
	launchArgs: ["--permission-mode", "auto"],
	options,
	sessions: {
		list: limit => listFiles(files, limit),
		get: id => getFile(files, id),
		resumeArgs: id => ["--resume", id],
		live: claudeLive,
		rename(s: Session, title: string) {
			// What Claude's own /rename writes.
			appendJsonl(s.path ?? fileFor(files, s.id)!, { type: "custom-title", customTitle: title, sessionId: s.id });
		},
		async delete(s: Session) {
			const file = s.path ?? fileFor(files, s.id);
			if (!file) throw new Error("That conversation is already gone.");
			const side = file.slice(0, -".jsonl".length); // subagent transcripts live in a sibling folder
			await trash(file, ...(fs.existsSync(side) ? [side] : []));
		},
	},
};

export const claudeHome = () => path.join(home(), ".claude");
