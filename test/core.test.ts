// Core behaviour against a throwaway HOME. Every XDG base dir is pointed inside
// it too: the real session's XDG_* vars would otherwise reach the real config.
import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import * as fs from "node:fs";
import * as path from "node:path";

// Inside the repo (git-ignored): gio refuses to trash on /tmp ("system internal mounts").
const HOME = fs.mkdtempSync(path.join(import.meta.dir, ".home-"));
const REAL_PATH = process.env.PATH;
process.env.HOME = HOME;
for (const v of ["XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"]) delete process.env[v];

const { claude } = await import("../src/agents/claude");
const { pi } = await import("../src/agents/pi");
const { recent } = await import("../src/core/catalog");
const { shellSplit, saveApps } = await import("../src/core/config");
const { resumeSpec, appSpec } = await import("../src/core/launch");

const write = (rel: string, lines: unknown[], ageMin = 0) => {
	const f = path.join(HOME, rel);
	fs.mkdirSync(path.dirname(f), { recursive: true });
	fs.writeFileSync(f, lines.map(l => JSON.stringify(l)).join("\n") + "\n");
	const t = new Date(Date.now() - ageMin * 60_000);
	fs.utimesSync(f, t, t);
	return f;
};
const user = (text: string, cwd = "/w/a") => ({ type: "user", cwd, message: { role: "user", content: text } });

beforeAll(() => {
	write(".claude/projects/-w-a/c1.jsonl", [user("newest prompt")], 1);
	write(".claude/projects/-w-a/c2.jsonl", [user("<command-name>/clear</command-name>"), user("real prompt")], 10);
	write(".claude/projects/-w-b/c3.jsonl", [user("old"), { type: "ai-title", aiTitle: "AI title" }], 60);
	write(".claude/projects/-w-b/c4.jsonl", [user("x"), { type: "ai-title", aiTitle: "AI" }, { type: "custom-title", customTitle: "Mine" }], 120);
	write(".pi/agent/sessions/--w--/2026_p1.jsonl", [
		{ type: "session", id: "p1", cwd: "/w/p" },
		{ type: "message", id: "aa11bb22", parentId: null, message: { role: "user", content: "pi prompt" } },
	], 5);
});
afterAll(() => {
	process.env.PATH = REAL_PATH;
	fs.rmSync(HOME, { recursive: true, force: true });
});

describe("sessions", () => {
	test("newest first across agents, with paging", () => {
		const page = recent(["claude", "pi"], 3);
		expect(page.sessions.map(s => s.id.split("/").pop())).toEqual(["c1", "2026_p1.jsonl", "c2"]);
		expect(page.more).toBe(true);
		expect(recent(["claude", "pi"], 10).more).toBe(false);
	});

	test("titles: custom > ai > first real prompt", () => {
		const byId = Object.fromEntries(claude.sessions!.list(10).map(s => [s.id, s.title]));
		expect(byId).toMatchObject({ c1: "newest prompt", c2: "real prompt", c3: "AI title", c4: "Mine" });
	});

	test("claude rename appends a custom-title entry", async () => {
		const s = claude.sessions!.get("c1")!;
		await claude.sessions!.rename!(s, "Renamed");
		const last = fs.readFileSync(s.path!, "utf8").trim().split("\n").pop()!;
		expect(JSON.parse(last)).toEqual({ type: "custom-title", customTitle: "Renamed", sessionId: "c1" });
		expect(claude.sessions!.get("c1")!.title).toBe("Renamed");
	});

	test("pi rename appends session_info on the leaf", async () => {
		const s = pi.sessions!.list(1)[0]!;
		await pi.sessions!.rename!(s, "Pi name");
		const last = JSON.parse(fs.readFileSync(s.path!, "utf8").trim().split("\n").pop()!);
		expect(last).toMatchObject({ type: "session_info", parentId: "aa11bb22", name: "Pi name" });
		expect(pi.sessions!.get(s.id)!.title).toBe("Pi name");
	});

	test("open Claude sessions come from ~/.claude/sessions with a live pid", () => {
		write(".claude/sessions/1.json", [{ pid: process.pid, sessionId: "c2" }]);
		write(".claude/sessions/2.json", [{ pid: 2 ** 22 + 12345, sessionId: "c3" }]); // no such process
		const live = claude.sessions!.live!();
		expect(live.get("c2")).toBe(process.pid);
		expect(live.has("c3")).toBe(false);
	});

	test("delete moves the transcript and its sibling folder to the Trash", async () => {
		const s = claude.sessions!.get("c4")!;
		fs.mkdirSync(s.path!.replace(/\.jsonl$/, ""), { recursive: true });
		await claude.sessions!.delete!(s);
		expect(fs.existsSync(s.path!)).toBe(false);
		expect(fs.readdirSync(path.join(HOME, ".local/share/Trash/files")).sort()).toEqual(["c4", "c4.jsonl"]);
	});
});

describe("launching", () => {
	const stubs = path.join(HOME, "stubs");
	const setPath = (omarchy: boolean) => {
		fs.rmSync(stubs, { recursive: true, force: true });
		fs.mkdirSync(stubs, { recursive: true });
		for (const b of ["claude", "hermes", ...(omarchy ? ["omarchy-agent"] : [])])
			fs.writeFileSync(path.join(stubs, b), "#!/bin/sh\n", { mode: 0o755 });
		process.env.PATH = stubs;
	};

	test("without Omarchy: the agent's own argv, in the session's folder", () => {
		setPath(false);
		fs.mkdirSync(path.join(HOME, "w/a"), { recursive: true });
		const s = { ...claude.sessions!.get("c1")!, cwd: path.join(HOME, "w/a") };
		expect(resumeSpec(s, "here", ["--effort", "high"])).toEqual({
			argv: ["claude", "--permission-mode", "auto", "--effort", "high", "--resume", "c1"],
			cwd: path.join(HOME, "w/a"),
		});
	});

	test("with Omarchy: through omarchy-agent, args and cwd handed to the shim", () => {
		setPath(true);
		saveApps([{ id: "hermes", label: "Hermes", agent: "hermes" }]);
		const spec = appSpec({ id: "hermes", label: "Hermes", agent: "hermes" }, "here");
		expect(spec.argv).toEqual(["omarchy-agent"]);
		expect(spec.env).toMatchObject({ TOPDOWN_AGENT: "hermes", TOPDOWN_ARGS: "--tui", TOPDOWN_INLINE: "1" });
		expect(spec.env!.PATH!.split(":")[0]).toEndWith("topdown/shim");
	});

	test("an app's own args replace the agent's defaults", () => {
		setPath(true);
		const spec = appSpec({ id: "h", label: "H", agent: "hermes", args: "--cli 'two words'" }, "here");
		expect(spec.env!.TOPDOWN_ARGS).toBe("--cli\ntwo words");
	});
});

test("shellSplit", () => {
	expect(shellSplit(`a "b c" 'd e' f\\ g ""`)).toEqual(["a", "b c", "d e", "f g", ""]);
});
