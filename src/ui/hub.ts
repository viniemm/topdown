// The main screen, in the layout of omp's model picker (see src/tui/frame.ts).
//   sidebar  Apps · Recent · one entry per agent with conversations
//   body     scope title, search line, list, detail panel
//   footer   key hints for whatever has focus
import type { App as Ui, Component } from "../tui/app";
import { HubFrame, type SidebarEntry } from "../tui/frame";
import { rank } from "../tui/fuzzy";
import { Input } from "../tui/input";
import type { Key } from "../tui/keys";
import { truncate, width as textWidth } from "../tui/text";
import { theme } from "../tui/theme";
import { AGENTS, agent as findAgent, installed } from "../agents";
import { getSession, liveIds, recent } from "../core/catalog";
import { getDefault, loadApps, saveApps, setDefault } from "../core/config";
import type { App, OptionSpec, Session } from "../core/types";
import { ago, oneline, project, shortPath, stamp } from "../core/util";
import { windowForPid } from "../integrations/hyprland";
import { refreshUsage, tightest, usageId, usageRecord } from "../integrations/omarchy";
import { bar, fit, usageLines } from "./format";

const PAGE = 40;
const SEARCH_LIMIT = 300;

type ScopeKind = "apps" | "recent" | "agent" | "separator";
type Row =
	| { kind: "app"; app: App }
	| { kind: "add" }
	| { kind: "session"; s: Session }
	| { kind: "more" }
	| { kind: "empty"; text: string };

export type Where = "here" | "window";
export type HubResult =
	| { action: "launch"; app: App; where: Where; extra: string[] }
	| { action: "resume"; s: Session; where: Where; extra: string[] }
	| { action: "focus"; address: string }
	| { action: "quit" };

type Modal =
	| { kind: "message"; lines: string[] }
	| { kind: "confirm"; lines: string[]; onYes: () => Promise<void> | void }
	| { kind: "input"; title: string; input: Input; onSubmit: (v: string) => Promise<void> | void }
	| { kind: "add"; items: { label: string; app?: App }[]; cursor: number }
	| { kind: "sheet"; row: Row; agentId: string; options: OptionSpec[]; values: number[]; cursor: number };

export interface HubOptions {
	chooseDefault?: boolean;
}

export class Hub implements Component {
	#ui: Ui;
	#done: (r: HubResult) => void;
	#opts: HubOptions;
	#apps: App[] = [];
	#scope = "apps";
	#prevScope: { scope: string; cursor: number } | undefined;
	#focus: "scope" | "body" = "body";
	#rows: Row[] = [];
	#cursor = 0;
	#scroll = 0;
	#limit = PAGE;
	#more = false;
	#live = new Map<string, number>();
	#search = new Input();
	#modal: Modal | undefined;
	#bodyRows = 20;

	#frame: HubFrame<ScopeKind> = new HubFrame<ScopeKind>(
		"topdown",
		{ min: 16, max: 26 },
		(width, rows) =>
			this.#frame.sidebar(this.#entries(), width, rows, { id: this.#scope, focused: this.#focus === "scope" }, e => ({
				icon: e.kind === "apps" ? theme.fg("accent", "▦") : e.kind === "recent" ? theme.fg("accent", "◷") : theme.fg("muted", "◆"),
				annotation: theme.fg("dim", e.annotation ?? ""),
			})),
		(width, rows) => this.#renderBody(width, rows),
	);

	constructor(ui: Ui, done: (r: HubResult) => void, opts: HubOptions = {}) {
		this.#ui = ui;
		this.#done = done;
		this.#opts = opts;
		this.#apps = loadApps();
		this.#reload();
	}

	// ---------- data ----------

	#sessionAgents(): string[] {
		const ids = this.#apps.map(a => a.agent).filter((a): a is string => !!a && !!findAgent(a)?.sessions);
		return [...new Set(ids)];
	}

	#entries(): SidebarEntry<ScopeKind>[] {
		const entries: SidebarEntry<ScopeKind>[] = [
			{ id: "apps", kind: "apps", label: "Apps", annotation: String(this.#apps.length) },
			{ id: "recent", kind: "recent", label: "Recent" },
		];
		const agents = this.#sessionAgents();
		if (agents.length) entries.push({ id: "sep", kind: "separator", label: "" });
		for (const id of agents) {
			const t = tightest(usageRecord(usageId(this.#apps.find(a => a.agent === id), id)));
			entries.push({ id: `agent:${id}`, kind: "agent", label: id, annotation: t === undefined ? "" : `${Math.round(t * 100)}%` });
		}
		return entries;
	}

	#reload(keepCursor = false): void {
		const query = this.#search.getValue().trim();
		const prev = this.#rows[this.#cursor];
		if (this.#scope === "apps") {
			let apps = this.#apps;
			if (query) apps = rank(apps, query, a => `${a.label} ${a.agent ?? a.command ?? ""}`);
			this.#rows = [...apps.map(app => ({ kind: "app", app }) as Row), ...(query ? [] : [{ kind: "add" } as Row])];
		} else {
			const agents = this.#scope === "recent" ? this.#sessionAgents() : [this.#scope.slice("agent:".length)];
			const page = recent(agents, query ? Math.max(this.#limit, SEARCH_LIMIT) : this.#limit);
			this.#live = liveIds(agents);
			let sessions = page.sessions;
			if (query) sessions = rank(sessions, query, s => `${s.agent} ${project(s.cwd)} ${s.title}`);
			this.#more = !query && page.more;
			this.#rows = sessions.map(s => ({ kind: "session", s }) as Row);
			if (this.#more) this.#rows.push({ kind: "more" });
			if (!this.#rows.length) this.#rows.push({ kind: "empty", text: query ? "no matches" : "no conversations yet" });
		}
		if (keepCursor && prev) {
			const i = this.#rows.findIndex(r => sameRow(r, prev));
			if (i >= 0) this.#cursor = i;
		}
		this.#cursor = Math.max(0, Math.min(this.#cursor, this.#rows.length - 1));
	}

	#setScope(scope: string, cursor = 0): void {
		this.#scope = scope;
		this.#limit = PAGE;
		this.#cursor = cursor;
		this.#scroll = 0;
		this.#search.setValue("");
		this.#reload();
	}

	#row(): Row | undefined {
		return this.#rows[this.#cursor];
	}

	#rowAgent(row = this.#row()): string | undefined {
		if (!row) return undefined;
		if (row.kind === "app") return row.app.agent;
		if (row.kind === "session") return row.s.agent;
		return undefined;
	}

	// ---------- input ----------

	handleKey(k: Key): void {
		if (this.#modal) void this.#modalKey(k);
		else if (this.#focus === "scope") this.#scopeKey(k.name);
		else void this.#bodyKey(k);
	}

	#scopeKey(key: string): void {
		const entries = this.#entries().filter(e => e.kind !== "separator");
		const i = entries.findIndex(e => e.id === this.#scope);
		if (key === "up" && i > 0) this.#setScope(entries[i - 1]!.id);
		else if (key === "down" && i < entries.length - 1) this.#setScope(entries[i + 1]!.id);
		else if (key === "enter" || key === "right" || key === "tab") this.#focus = "body";
		else if (key === "escape" || key === "ctrl+c") this.#done({ action: "quit" });
	}

	async #bodyKey(k: Key): Promise<void> {
		const key = k.name;
		const query = this.#search.getValue();
		const row = this.#row();
		switch (key) {
			case "up":
				return this.#move(-1);
			case "down":
				return this.#move(1);
			case "pageUp":
				return this.#move(-this.#bodyRows);
			case "pageDown":
				return this.#move(this.#bodyRows);
			case "tab":
				return this.#setScope(this.#scope === "apps" ? "recent" : "apps");
			case "right":
				if (query) break;
				if (row?.kind === "app" && row.app.agent && findAgent(row.app.agent)?.sessions) {
					this.#prevScope = { scope: this.#scope, cursor: this.#cursor };
					this.#setScope(`agent:${row.app.agent}`);
				}
				return;
			case "left":
				if (this.#scope.startsWith("agent:") && this.#prevScope) {
					// Back out of an agent's view, search and all.
					const p = this.#prevScope;
					this.#prevScope = undefined;
					return this.#setScope(p.scope, p.cursor);
				}
				if (query) break;
				this.#focus = "scope";
				return;
			case "escape":
				if (query) {
					this.#search.setValue("");
					return this.#reload();
				}
				if (this.#scope.startsWith("agent:") && this.#prevScope) return this.#bodyKey({ name: "left" });
				return this.#done({ action: "quit" });
			case "ctrl+c":
				return this.#done({ action: "quit" });
			case "enter":
				return this.#activate("here", []);
			case "alt+enter":
				return this.#activate("window", []);
			case "ctrl+o":
				return this.#openSheet();
			case "ctrl+d":
				return this.#makeDefault();
			case "ctrl+r":
				return this.#refresh();
			case "ctrl+a":
				return this.#openAdd();
			case "ctrl+e":
			case "f2":
				return this.#rename();
			case "ctrl+x":
				return this.#delete();
			case "delete":
				if (!query) return this.#delete();
				break;
		}
		// Anything else edits the search.
		this.#search.handle(k);
		if (this.#search.getValue() !== query) {
			this.#cursor = 0;
			this.#scroll = 0;
			this.#reload();
		}
	}

	#move(delta: number): void {
		this.#cursor = Math.max(0, Math.min(this.#rows.length - 1, this.#cursor + delta));
		// Near the end of what's loaded: fetch the next page, keeping the cursor.
		if (this.#more && this.#cursor >= this.#rows.length - 4) {
			this.#limit += PAGE;
			this.#reload(true);
		}
	}

	// ---------- actions ----------

	async #activate(where: Where, extra: string[]): Promise<void> {
		const row = this.#row();
		if (!row) return;
		if (row.kind === "add") return this.#openAdd();
		if (row.kind === "more") return this.#move(1);
		if (row.kind === "app") {
			if (this.#opts.chooseDefault) {
				if (!row.app.agent) return this.#message(["Only agents can be the default agent."]);
				setDefault(row.app.agent);
			}
			return this.#done({ action: "launch", app: row.app, where, extra });
		}
		if (row.kind === "session") {
			const pid = this.#live.get(row.s.id);
			if (pid) {
				// Already open: switch to its window instead of running it twice.
				const address = windowForPid(pid);
				if (address) return this.#done({ action: "focus", address });
				return this.#message(["That conversation is open in a terminal topdown can't switch to."]);
			}
			return this.#done({ action: "resume", s: row.s, where, extra });
		}
	}

	#makeDefault(): void {
		const row = this.#row();
		if (row?.kind !== "app") return;
		if (!row.app.agent) return this.#message(["Only agents can be the default agent."]);
		setDefault(row.app.agent);
	}

	async #refresh(): Promise<void> {
		const id = this.#rowAgent();
		if (!id) return;
		const row = this.#row();
		await refreshUsage(usageId(row?.kind === "app" ? row.app : undefined, id));
		this.#ui.requestRender();
	}

	#message(lines: string[]): void {
		this.#modal = { kind: "message", lines };
	}

	#openAdd(): void {
		const listed = new Set(this.#apps.map(a => a.agent).filter(Boolean));
		const items: { label: string; app?: App }[] = AGENTS.filter(a => !listed.has(a.id) && installed(a)).map(a => ({
			label: `${a.label}  ${theme.fg("dim", a.id)}`,
			app: { id: a.id, label: a.label, agent: a.id },
		}));
		items.push({ label: theme.fg("accent", "Custom command…") });
		this.#modal = { kind: "add", items, cursor: 0 };
	}

	#addCustom(): void {
		const input = new Input();
		input.prompt = "command › ";
		this.#modal = {
			kind: "input",
			title: "Command to run in a terminal (e.g. lazygit, aider --model sonnet)",
			input,
			onSubmit: command => {
				const c = command.trim();
				if (!c) return;
				const id = c.split(/\s+/)[0]!.replace(/[^\w.-]/g, "") || "app";
				this.#apps.push({ id: this.#uniqueId(id), label: id, command: c });
				saveApps(this.#apps);
				this.#reload();
			},
		};
	}

	#uniqueId(id: string): string {
		let out = id;
		for (let n = 2; this.#apps.some(a => a.id === out); n++) out = `${id}-${n}`;
		return out;
	}

	#rename(): void {
		const row = this.#row();
		const input = new Input();
		input.prompt = "name › ";
		if (row?.kind === "app") {
			input.setValue(row.app.label);
			this.#modal = {
				kind: "input",
				title: `Rename ${row.app.label}`,
				input,
				onSubmit: v => {
					if (!v.trim()) return;
					row.app.label = v.trim();
					saveApps(this.#apps);
					this.#reload(true);
				},
			};
		} else if (row?.kind === "session") {
			const store = findAgent(row.s.agent)?.sessions;
			if (!store?.rename) return this.#message([`Renaming ${row.s.agent} conversations isn't supported yet.`]);
			if (this.#live.has(row.s.id)) return this.#message([`"${oneline(row.s.title, 60)}" is open right now; rename it there (/rename).`]);
			input.setValue(row.s.title);
			this.#modal = {
				kind: "input",
				title: "Rename conversation",
				input,
				onSubmit: async v => {
					const title = v.split(/\s+/).filter(Boolean).join(" ");
					if (!title || title === row.s.title) return;
					await store.rename!(row.s, title);
					this.#reload(true);
				},
			};
		}
	}

	#delete(): void {
		const row = this.#row();
		if (row?.kind === "app") {
			this.#modal = {
				kind: "confirm",
				lines: [`Remove ${theme.bold(row.app.label)} from the list?`, theme.fg("dim", "It stays installed.")],
				onYes: () => {
					this.#apps = this.#apps.filter(a => a !== row.app);
					saveApps(this.#apps);
					this.#reload();
				},
			};
		} else if (row?.kind === "session") {
			const store = findAgent(row.s.agent)?.sessions;
			if (!store?.delete) return this.#message([`${row.s.agent} has no delete command, so its conversations can't be deleted here.`]);
			if (this.#live.has(row.s.id))
				return this.#message([`"${oneline(row.s.title, 60)}" is open right now.`, "Close it first: a running agent writes it straight back."]);
			const fresh = Date.now() / 1000 - row.s.when < 120;
			this.#modal = {
				kind: "confirm",
				lines: [
					`Delete ${theme.bold(`"${oneline(row.s.title, 60)}"`)} (${row.s.agent})?`,
					...(fresh ? [theme.fg("warning", `It changed ${ago(row.s.when)}. If it's still open somewhere, the agent will write it back.`)] : []),
					theme.fg("dim", row.s.path ? "It goes to the Trash." : "This uses the agent's own delete command."),
				],
				onYes: async () => {
					await store.delete!(row.s);
					this.#reload();
				},
			};
		}
	}

	#openSheet(): void {
		const row = this.#row();
		const agentId = this.#rowAgent(row);
		const options = (findAgent(agentId)?.options ?? []).filter(o => !o.resumeOnly || row?.kind === "session");
		if (!row || !agentId || !options.length) return this.#message([`No launch options for ${agentId ?? "this row"} yet.`]);
		this.#modal = { kind: "sheet", row, agentId, options, values: options.map(() => 0), cursor: 0 };
	}

	async #modalKey(k: Key): Promise<void> {
		const key = k.name;
		const m = this.#modal!;
		const close = () => {
			this.#modal = undefined;
		};
		try {
			if (m.kind === "message") return close();
			if (m.kind === "confirm") {
				if (key === "y" || key === "enter") {
					close();
					await m.onYes();
				} else if (key === "n" || key === "escape" || key === "ctrl+c") close();
				return;
			}
			if (m.kind === "input") {
				if (key === "escape" || key === "ctrl+c") return close();
				if (key === "enter") {
					close();
					return await m.onSubmit(m.input.getValue());
				}
				m.input.handle(k);
				return;
			}
			if (m.kind === "add") {
				if (key === "escape" || key === "ctrl+c") return close();
				if (key === "up") m.cursor = Math.max(0, m.cursor - 1);
				if (key === "down") m.cursor = Math.min(m.items.length - 1, m.cursor + 1);
				if (key === "enter") {
					const item = m.items[m.cursor]!;
					if (!item.app) return this.#addCustom();
					close();
					this.#apps.push({ ...item.app, id: this.#uniqueId(item.app.id) });
					saveApps(this.#apps);
					this.#reload();
				}
				return;
			}
			if (m.kind === "sheet") {
				const options = m.options;
				const opt = options[m.cursor]!;
				if (key === "escape" || key === "ctrl+c") return close();
				if (key === "up") m.cursor = Math.max(0, m.cursor - 1);
				if (key === "down") m.cursor = Math.min(options.length - 1, m.cursor + 1);
				if (key === "left") m.values[m.cursor] = (m.values[m.cursor]! + opt.choices.length - 1) % opt.choices.length;
				if (key === "right" || key === "space") m.values[m.cursor] = (m.values[m.cursor]! + 1) % opt.choices.length;
				if (key === "enter" || key === "alt+enter") {
					const extra = options.flatMap((o, i) => (m.values[i] ? o.args(o.choices[m.values[i]!]!) : []));
					close();
					return this.#activate(key === "enter" ? "here" : "window", extra);
				}
			}
		} catch (e) {
			this.#message([theme.fg("error", e instanceof Error ? e.message : String(e))]);
		} finally {
			this.#ui.requestRender();
		}
	}

	// ---------- render ----------

	render(width: number, height: number): string[] {
		return this.#frame.render(width, height, this.#entries(), this.#footer());
	}

	#footer(): string {
		const k = (key: string, what: string) => `${theme.fg("accent", key)} ${what}`;
		const sep = theme.fg("dim", " · ");
		if (this.#modal?.kind === "sheet") return [k("↑/↓", "option"), k("←/→", "value"), k("⏎", "launch"), k("Alt-⏎", "new window"), k("esc", "back")].join(sep);
		if (this.#modal) return [k("⏎", "ok"), k("esc", "cancel")].join(sep);
		if (this.#focus === "scope") return [k("↑/↓", "scope"), k("⏎/→", "list"), k("esc", "close")].join(sep);
		const parts = [k("⏎", "open"), k("Alt-⏎", "window"), k("^O", "options"), k("Tab", this.#scope === "apps" ? "recent" : "apps")];
		if (this.#scope === "apps") parts.push(k("→", "chats"), k("^D", "default"), k("^R", "usage"), k("^A", "add"));
		else if (this.#scope.startsWith("agent:")) parts.push(k("←", "back"));
		parts.push(k("^E", "rename"), k("^X", "delete"), k("esc", "close"));
		return parts.join(sep);
	}

	#renderBody(width: number, height: number): readonly string[] {
		const out: string[] = [];
		const title =
			this.#scope === "apps" ? "Apps" : this.#scope === "recent" ? "Recent conversations" : `${this.#scope.slice(6)} conversations`;
		const banner = this.#opts.chooseDefault ? theme.fg("warning", "  No default agent yet: Enter makes it the default and launches it") : "";
		out.push(fit(` ${theme.bold(title)}${banner}`, width));
		if (this.#modal) {
			out.push("");
			out.push(...this.#renderModal(width - 2).map(l => ` ${l}`));
			while (out.length < height) out.push("");
			return out.slice(0, height);
		}
		const searchLine = this.#search.render(Math.max(1, width - 4), this.#focus === "body");
		out.push(` ${theme.fg("accent", "›")} ${searchLine}`);

		const detail = this.#renderDetail(width - 2).map(l => ` ${l}`);
		const listRows = Math.max(3, height - out.length - detail.length - 1);
		this.#bodyRows = listRows;
		if (this.#cursor < this.#scroll) this.#scroll = this.#cursor;
		if (this.#cursor >= this.#scroll + listRows) this.#scroll = this.#cursor - listRows + 1;
		const aw = Math.max(6, ...this.#rows.map(r => (r.kind === "session" ? r.s.agent.length : 0)));
		const lw = Math.max(10, ...this.#apps.map(a => textWidth(a.label)));
		const thumb = scrollbar(this.#rows.length, listRows, this.#scroll);
		for (let i = 0; i < listRows; i++) {
			const r = this.#rows[this.#scroll + i];
			const selected = this.#scroll + i === this.#cursor && this.#focus === "body";
			let line = r ? this.#renderRow(r, aw, lw) : "";
			line = fit(line, width - 2);
			if (selected) line = theme.selected(line);
			out.push(`${line}${thumb[i] ?? " "} `);
		}
		out.push(theme.fg("border", "─".repeat(width)));
		out.push(...detail);
		while (out.length < height) out.push("");
		return out.slice(0, height);
	}

	#renderRow(r: Row, aw: number, lw: number): string {
		switch (r.kind) {
			case "app": {
				const def = r.app.agent && r.app.agent === getDefault() ? theme.fg("warning", "★") : " ";
				const t = r.app.agent ? tightest(usageRecord(usageId(r.app, r.app.agent))) : undefined;
				const meter = t === undefined ? " ".repeat(6) : bar(t, 6);
				const kind = r.app.agent ? `agent: ${r.app.agent}` : `tui: ${r.app.command ?? ""}`;
				return ` ${def} ${r.app.label.padEnd(lw)}  ${meter}  ${theme.fg("dim", kind)}`;
			}
			case "add":
				return `   ${theme.fg("accent", "+ Add app…")}`;
			case "more":
				return theme.fg("dim", "   ↓ more conversations…");
			case "empty":
				return theme.fg("dim", `   ${r.text}`);
			case "session": {
				const live = this.#live.has(r.s.id) ? theme.fg("success", "●") : " ";
				return ` ${live} ${theme.fg("dim", stamp(r.s.when).padEnd(11))}${theme.fg("accent", r.s.agent.padEnd(aw))}  ${theme.fg("muted", project(r.s.cwd).padEnd(12).slice(0, 12))} ${oneline(r.s.title, 200)}`;
			}
		}
	}

	#renderDetail(width: number): string[] {
		const r = this.#row();
		if (!r) return [];
		if (r.kind === "app") {
			const star = r.app.agent && r.app.agent === getDefault() ? `  ${theme.fg("warning", "★ default")}` : "";
			const what = r.app.agent ? `agent: ${r.app.agent}` : `command: ${r.app.command ?? ""}`;
			const head = `${theme.bold(r.app.label)}${star}  ${theme.fg("dim", what)}`;
			if (!r.app.agent) return [head];
			return [head, ...usageLines(usageRecord(usageId(r.app, r.app.agent)), width)];
		}
		if (r.kind === "session") {
			const s = getSession(r.s.agent, r.s.id) ?? r.s;
			const live = this.#live.has(s.id) ? `  ${theme.fg("success", "● open")}` : "";
			return [
				theme.bold(oneline(s.title, width)),
				`${theme.fg("accent", s.agent)}  ${theme.fg("muted", shortPath(s.cwd))}  ${theme.fg("dim", `${stamp(s.when)} · ${ago(s.when)}`)}${live}`,
				...s.prompts.slice(0, 2).map(p => `${theme.fg("accent", "›")} ${oneline(p, width - 2)}`),
			];
		}
		if (r.kind === "add") return [theme.fg("dim", "Installed agents topdown knows, or any command to run in a terminal.")];
		return [];
	}

	#renderModal(width: number): string[] {
		const m = this.#modal!;
		const hint = (s: string) => theme.fg("dim", s);
		if (m.kind === "message") return [...m.lines, "", hint("any key to go back")];
		if (m.kind === "confirm") return [...m.lines, "", `${theme.fg("accent", "y")} yes   ${theme.fg("accent", "n")} no`];
		if (m.kind === "input") return [theme.bold(m.title), "", truncate(m.input.render(width), width), "", hint("⏎ save · esc cancel")];
		if (m.kind === "add")
			return [
				theme.bold("Add which app?"),
				"",
				...m.items.map((it, i) => (i === m.cursor ? theme.selected(fit(` ${it.label}`, width)) : ` ${it.label}`)),
			];
		const options = m.options;
		const lines = [theme.bold(`${m.row.kind === "session" ? "Resume" : "Launch"} ${m.agentId} with…`), ""];
		options.forEach((o, i) => {
			const choices = o.choices
				.map((c, j) => (j === m.values[i] ? theme.selected(theme.fg("accent", ` ${c} `)) : theme.fg("dim", ` ${c} `)))
				.join("");
			const label = (i === m.cursor ? theme.fg("accent", "▸ ") : "  ") + o.label.padEnd(12);
			lines.push(truncate(`${label}${choices}`, width));
		});
		const extra = options.flatMap((o, i) => (m.values[i] ? o.args(o.choices[m.values[i]!]!) : []));
		lines.push("", hint(`${findAgent(m.agentId)!.bin} ${extra.join(" ")}`.trim()));
		const help = options[m.cursor]?.help;
		if (help) lines.push(hint(help));
		return lines;
	}
}

function sameRow(a: Row, b: Row): boolean {
	if (a.kind !== b.kind) return false;
	if (a.kind === "app" && b.kind === "app") return a.app.id === b.app.id;
	if (a.kind === "session" && b.kind === "session") return a.s.agent === b.s.agent && a.s.id === b.s.id;
	return true;
}

/** One cell per visible row: a thumb (█) over a track (│), blank when it all fits. */
function scrollbar(total: number, visible: number, offset: number): string[] {
	if (total <= visible) return [];
	const size = Math.max(1, Math.round((visible / total) * visible));
	const start = Math.min(visible - size, Math.round((offset / Math.max(1, total - visible)) * (visible - size)));
	return Array.from({ length: visible }, (_, i) => (i >= start && i < start + size ? theme.fg("accent", "█") : theme.fg("dim", "│")));
}
