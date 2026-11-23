// The extensibility seam: everything agent-specific lives behind AgentAdapter.

export interface Session {
	agent: string;
	/** Store-specific id: a file stem, a file path, or a database key. */
	id: string;
	cwd: string;
	title: string;
	/** Last activity, seconds since the epoch. */
	when: number;
	/** First few things the user typed. */
	prompts: string[];
	/** Backing file, for file stores. */
	path?: string;
}

/** A per-launch choice offered in the launch sheet, mapped to CLI flags. */
export interface OptionSpec {
	key: string;
	label: string;
	/** Allowed values; the first is the agent's own default (no flag). */
	choices: string[];
	help?: string;
	/** Only offered when resuming a conversation. */
	resumeOnly?: boolean;
	/** Arguments for a chosen value (not called for the default). */
	args(value: string): string[];
}

export interface SessionStore {
	/** Up to `limit` sessions, newest first. */
	list(limit: number): Session[];
	get(id: string): Session | undefined;
	resumeArgs(id: string): string[];
	rename?(s: Session, title: string): void | Promise<void>;
	delete?(s: Session): void | Promise<void>;
	/** Sessions the agent has open right now: id -> pid. */
	live?(): Map<string, number>;
}

export interface AgentAdapter {
	id: string;
	label: string;
	/** Executable name. */
	bin: string;
	/** Extra arguments on every launch and resume (an app's "args" overrides). */
	defaultArgs?: string[];
	/** Arguments for a plain launch when not going through Omarchy. */
	launchArgs?: string[];
	options?: OptionSpec[];
	sessions?: SessionStore;
	/** Omarchy usage record id, when it differs from `id`. */
	usageId?: string;
}

/** An entry in the user's app list. */
export interface App {
	id: string;
	label: string;
	/** A known agent id … */
	agent?: string;
	/** … or any command to run in a terminal. */
	command?: string;
	/** Extra arguments for an agent: a list or one shell-quoted string. */
	args?: string[] | string;
	/** Omarchy usage record id override. */
	usage?: string;
}

/** What to run: the wrapper execs `env -C cwd ENV… argv…`. */
export interface ExecSpec {
	argv: string[];
	cwd?: string;
	env?: Record<string, string>;
}
