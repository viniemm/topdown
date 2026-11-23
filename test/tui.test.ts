import { describe, expect, test } from "bun:test";
import { diff } from "../src/tui/app";
import { HubFrame, type SidebarEntry } from "../src/tui/frame";
import { rank } from "../src/tui/fuzzy";
import { Input } from "../src/tui/input";
import { parseKeys } from "../src/tui/keys";
import { fit, strip, truncate, width } from "../src/tui/text";

const names = (s: string) => parseKeys(s).map(k => k.name);

describe("keys", () => {
	test("sequences, modifiers and bursts", () => {
		expect(names("\x1b[A\x1b[B\x1bOC\x1b[D")).toEqual(["up", "down", "right", "left"]);
		expect(names("\x1b[3~\x1b[5~\x1b[6~\x1bOQ\x1b[12~\x1b[Z")).toEqual(["delete", "pageUp", "pageDown", "f2", "f2", "shift+tab"]);
		expect(names("\x1b[1;5C\x1b[3;3~")).toEqual(["ctrl+right", "alt+delete"]);
		expect(names("\r\t\x7f\x18\x05\x01")).toEqual(["enter", "tab", "backspace", "ctrl+x", "ctrl+e", "ctrl+a"]);
		expect(names("\x1b\r\x1bx\x1b")).toEqual(["alt+enter", "alt+x", "escape"]);
		expect(names("ab c")).toEqual(["a", "b", "space", "c"]);
		expect(names("\x1b[13;3u")).toEqual(["alt+enter"]);
	});

	test("bracketed paste is one event", () => {
		expect(parseKeys("\x1b[200~hello\r\nworld\x1b[201~x")).toEqual([
			{ name: "paste", text: "hello\nworld" },
			{ name: "x", text: "x" },
		]);
	});
});

describe("text", () => {
	const red = (s: string) => `\x1b[31m${s}\x1b[39m`;
	test("width ignores ANSI, counts wide characters as 2", () => {
		expect(width(red("abc"))).toBe(3);
		expect(width("日本")).toBe(4);
	});
	test("truncate and fit keep exact column counts", () => {
		expect(strip(truncate(red("abcdef"), 4))).toBe("abc…");
		expect(width(truncate("日本語テキスト", 5))).toBeLessThanOrEqual(5);
		for (const w of [1, 5, 9]) expect(width(fit(red("hello world"), w))).toBe(w);
	});
});

test("fuzzy: every term must match; substrings and word starts rank first", () => {
	const items = ["claude topdown Port the launcher", "pi xfin Refactor auth", "hermes topdown chat", "claude xfin authorization notes"];
	// Substring matches first; a scattered match (a·u·t·h across words) still counts, ranked last.
	const auth = rank(items, "auth", s => s);
	expect(new Set(auth.slice(0, 2))).toEqual(new Set(["pi xfin Refactor auth", "claude xfin authorization notes"]));
	expect(auth.at(-1)).toBe("claude topdown Port the launcher");
	expect(auth).not.toContain("hermes topdown chat");
	expect(rank(items, "top cl", s => s)).toEqual(["claude topdown Port the launcher"]);
	expect(rank(items, "", s => s)).toEqual(items);
});

test("input edits around the cursor", () => {
	const i = new Input();
	for (const k of parseKeys("helo")) i.handle(k);
	i.handle({ name: "left" });
	i.handle({ name: "l", text: "l" });
	expect(i.getValue()).toBe("hello");
	i.handle({ name: "end" });
	i.handle({ name: "paste", text: " big\nworld" });
	i.handle({ name: "ctrl+w" });
	expect(i.getValue()).toBe("hello big ");
	expect(i.handle({ name: "up" })).toBe(false);
});

test("render diff writes only changed rows, inside synchronized output", () => {
	expect(diff(["a", "b", "c"], ["a", "b", "c"])).toBe("");
	const out = diff(["a", "b", "c"], ["a", "X", "c"]);
	expect(out.startsWith("\x1b[?2026h") && out.endsWith("\x1b[?2026l")).toBe(true);
	expect(out).toContain("\x1b[2;1H");
	expect(out).not.toContain("\x1b[1;1H");
	expect(out).not.toContain("\x1b[3;1H");
});

test("hub frame: every line is exactly the screen width, joints line up", () => {
	const entries: SidebarEntry[] = [
		{ id: "apps", kind: "apps", label: "Apps", annotation: "4" },
		{ id: "sep", kind: "separator", label: "" },
		{ id: "claude", kind: "agent", label: "claude", annotation: "22%" },
	];
	const frame: HubFrame = new HubFrame(
		"topdown",
		{ min: 16, max: 26 },
		(w, rows) => frame.sidebar(entries, w, rows, { id: "apps", focused: true }, () => ({ icon: "◆", annotation: "" })),
		() => ["body line", "日本語 wide"],
	);
	for (const W of [40, 80, 133]) {
		const lines = frame.render(W, 12, entries, "⏎ open · esc close").map(strip);
		expect(lines).toHaveLength(12);
		for (const l of lines) expect(width(l)).toBe(W);
		const joint = lines[0]!.indexOf("┬");
		expect(lines[1]!.indexOf("│", 1)).toBe(joint);
		expect(lines.at(-3)!.indexOf("┴")).toBe(joint);
	}
});
