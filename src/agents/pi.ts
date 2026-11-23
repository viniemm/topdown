// Pi and Oh My Pi share a session format: JSONL entries forming a tree (id/parentId).
import * as fs from "node:fs";
import { appendJsonl, eachLine, type FileStoreSpec, getFile, listFiles, type Parsed, trash } from "../core/file-store";
import type { AgentAdapter, Session } from "../core/types";
import { isUserText, textOf } from "../core/util";

/** A name is the latest session_info entry (Pi's /name); omp may write a `title` entry. */
export function parsePi(file: string): Parsed {
	let cwd = "";
	let title = "";
	let name = "";
	const prompts: string[] = [];
	eachLine(file, (line, n) => {
		if (n >= 300 && !line.includes('"session_info"')) return; // past the start, only renames matter
		let d: any;
		try {
			d = JSON.parse(line);
		} catch {
			return;
		}
		if (d.type === "session_info") name = d.name || name;
		else if (d.type === "title") title = d.title || "";
		else if (d.type === "session") cwd = d.cwd || "";
		else if (d.type === "message" && d.message?.role === "user" && prompts.length < 5) {
			const t = textOf(d.message.content);
			if (isUserText(t)) prompts.push(t.trim().slice(0, 600));
		}
	});
	return { title: name || title || prompts[0] || "", cwd, prompts };
}

/** A session_info entry on the current leaf (the last entry), as Pi's /name writes. */
export function renamePi(file: string, title: string): void {
	let last: string | null = null;
	eachLine(file, line => {
		if (!line.includes('"id"')) return;
		try {
			last = JSON.parse(line).id ?? last;
		} catch {}
	});
	const id = Array.from(crypto.getRandomValues(new Uint8Array(4)), b => b.toString(16).padStart(2, "0")).join("");
	appendJsonl(file, { type: "session_info", id, parentId: last, timestamp: new Date().toISOString(), name: title });
}

function piFamily(id: string, label: string, dir: string, resumeFlag: string, canRename: boolean): AgentAdapter {
	const spec: FileStoreSpec = { agent: id, pattern: `${dir}/agent/sessions/*/*.jsonl`, parse: parsePi, id: "path" };
	return {
		id,
		label,
		bin: id,
		sessions: {
			list: limit => listFiles(spec, limit),
			get: sid => getFile(spec, sid),
			resumeArgs: sid => [resumeFlag, sid],
			// omp's on-disk naming isn't confirmed yet (no omp sessions to check against).
			rename: canRename ? (s: Session, title: string) => renamePi(s.path ?? s.id, title) : undefined,
			async delete(s: Session) {
				const file = s.path ?? s.id;
				if (!fs.existsSync(file)) throw new Error("That conversation is already gone.");
				await trash(file);
			},
		},
	};
}

export const pi = piFamily("pi", "Pi", ".pi", "--session", true);
export const omp = piFamily("omp", "Oh My Pi", ".omp", "--resume", false);
