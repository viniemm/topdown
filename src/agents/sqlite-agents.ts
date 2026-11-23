// Agents whose sessions live in SQLite: Hermes and Antigravity (read-only access).
import { Database } from "bun:sqlite";
import * as fs from "node:fs";
import * as path from "node:path";
import type { AgentAdapter, Session } from "../core/types";
import { epoch, home, isUserText, userBin } from "../core/util";

function query<T>(file: string, sql: string, params: (string | number)[]): T[] {
	if (!fs.existsSync(file)) return [];
	let db: Database | undefined;
	try {
		db = new Database(file, { readonly: true });
		return db.query(sql).all(...params) as T[];
	} catch {
		return []; // a store changing shape must never break the picker
	} finally {
		db?.close();
	}
}

async function run(argv: string[]): Promise<void> {
	const p = Bun.spawn([userBin(argv[0]!), ...argv.slice(1)], { stdout: "ignore", stderr: "pipe" });
	if ((await p.exited) !== 0) throw new Error((await new Response(p.stderr).text()).trim() || `${argv[0]} failed`);
}

type HermesRow = { id: string; cwd: string | null; title: string | null; ts: unknown; first: string | null };

function hermesSessions(limit: number, sid?: string): Session[] {
	const file = path.join(home(), ".hermes/state.db");
	const rows = query<HermesRow>(
		file,
		`select s.id as id, s.cwd as cwd, s.title as title, coalesce(s.last_activity_at, s.started_at) as ts,
		  (select content from messages m where m.session_id = s.id and m.role = 'user' order by m.id limit 1) as first
		 from sessions s where coalesce(s.hidden,0) = 0 and coalesce(s.archived,0) = 0
		  and s.source in ('cli','tui','local') and s.message_count > 0 ${sid ? "and s.id = ?" : ""}
		 order by 4 desc limit ?`,
		sid ? [sid, 1] : [limit],
	);
	return rows
		.filter(r => r.title || isUserText(r.first ?? ""))
		.map(r => ({ agent: "hermes", id: r.id, cwd: r.cwd ?? "", title: r.title || r.first || "", when: epoch(r.ts), prompts: r.first ? [r.first] : [] }));
}

export const hermes: AgentAdapter = {
	id: "hermes",
	label: "Hermes",
	bin: "hermes",
	// Omarchy starts Hermes in its classic REPL; --tui is Hermes' modern TUI.
	defaultArgs: ["--tui"],
	launchArgs: ["--yolo"],
	sessions: {
		list: limit => hermesSessions(limit),
		get: id => hermesSessions(1, id)[0],
		resumeArgs: id => ["--resume", id],
		rename: (s, title) => run(["hermes", "sessions", "rename", s.id, title]),
		delete: s => run(["hermes", "sessions", "delete", "--yes", s.id]),
	},
};

type AgyRow = { id: string; title: string | null; preview: string | null; ts: unknown; ws: string | null };

function agySessions(limit: number, sid?: string): Session[] {
	const file = path.join(home(), ".gemini/antigravity-cli/conversation_summaries.db");
	const rows = query<AgyRow>(
		file,
		`select conversation_id as id, title, preview, last_modified_time as ts, workspace_uris as ws
		 from conversation_summaries ${sid ? "where conversation_id = ?" : ""} order by last_modified_time desc limit ?`,
		sid ? [sid, 1] : [limit],
	);
	return rows
		.filter(r => r.title || r.preview)
		.map(r => {
			let cwd = "";
			try {
				cwd = String((JSON.parse(r.ws ?? "[]") as string[])[0] ?? "").replace(/^file:\/\//, "");
			} catch {}
			return { agent: "agy", id: r.id, cwd, title: r.title || r.preview || "", when: epoch(r.ts), prompts: r.preview ? [r.preview] : [] };
		});
}

// Antigravity has no delete or rename command, so those stay unsupported.
export const agy: AgentAdapter = {
	id: "agy",
	label: "Antigravity",
	bin: "agy",
	sessions: {
		list: limit => agySessions(limit),
		get: id => agySessions(1, id)[0],
		resumeArgs: id => ["--conversation", id],
	},
};
