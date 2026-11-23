// Conversations across agents, newest first, loaded a page at a time.
import { agent as findAgent } from "../agents";
import { saveCache } from "./file-store";
import type { Session } from "./types";

export interface Page {
	sessions: Session[];
	/** Older conversations exist beyond `limit`. */
	more: boolean;
}

export function recent(agentIds: string[], limit: number): Page {
	const all: Session[] = [];
	for (const id of agentIds) {
		try {
			all.push(...(findAgent(id)?.sessions?.list(limit + 1) ?? []));
		} catch {} // a store changing shape must never break the picker
	}
	saveCache();
	all.sort((a, b) => b.when - a.when);
	return { sessions: all.slice(0, limit), more: all.length > limit };
}

export function liveIds(agentIds: string[]): Map<string, number> {
	const live = new Map<string, number>();
	for (const id of agentIds) for (const [sid, pid] of findAgent(id)?.sessions?.live?.() ?? []) live.set(sid, pid);
	return live;
}

export function getSession(agentId: string, id: string): Session | undefined {
	return findAgent(agentId)?.sessions?.get(id);
}
