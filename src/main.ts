// topdown spike: prove pi-tui's HubFrame renders, takes keys, and compiles.
import { ProcessTerminal, TUI, type Component } from "@oh-my-pi/pi-tui";
import { HubFrame, type SidebarEntry } from "@oh-my-pi/pi-tui/overlays/hub-frame";
import { matchesKey } from "@oh-my-pi/pi-tui/keys";
import { initThemeSync, theme } from "@oh-my-pi/pi-tui/theme/theme";
import { truncateToWidth, visibleWidth } from "@oh-my-pi/pi-tui/utils";

type Kind = "agent" | "recent" | "separator";

class SpikeHub implements Component {
	#tui: TUI;
	#done: () => void;
	#entries: SidebarEntry<Kind>[] = [
		{ id: "recent", kind: "recent", label: "Recent", annotation: "12" },
		{ id: "sep", kind: "separator", label: "" },
		{ id: "claude", kind: "agent", label: "claude", annotation: "8" },
		{ id: "hermes", kind: "agent", label: "hermes", annotation: "3" },
		{ id: "pi", kind: "agent", label: "pi", annotation: "1" },
	];
	#active = "recent";
	#focus: "scope" | "body" = "body";
	#cursor = 0;
	#rows = [
		["● 21:40", "claude", "topdown", "Omarchy agent launcher"],
		["  Yest 14:02", "hermes", "Work", "Lightest Linux distro for agent VM"],
		["  Mon 09:10", "pi", "xfin", "Refactor auth module"],
	];
	#frame = new HubFrame(
		"topdown",
		{ min: 16, max: 24 },
		(width, rows) =>
			this.#frame.renderSidebar(
				this.#entries,
				width,
				rows,
				{ id: this.#active, focused: this.#focus === "scope", follow: true, clamp: false },
				entry => ({ icon: entry.kind === "agent" ? theme.fg("accent", "◆") : "◇", annotation: theme.fg("dim", entry.annotation ?? "") }),
			),
		(width, height) => {
			const out = [theme.bold(" Recent conversations"), theme.fg("dim", " > ")];
			this.#rows.forEach((r, i) => {
				const line = ` ${theme.fg("dim", r[0].padEnd(12))} ${theme.fg("accent", r[1].padEnd(8))} ${theme.fg("muted", r[2].padEnd(10))} ${r[3]}`;
				const sel = i === this.#cursor && this.#focus === "body";
				out.push(sel ? theme.bg("selectedBg", truncateToWidth(line, width) + " ".repeat(Math.max(0, width - visibleWidth(line)))) : truncateToWidth(line, width));
			});
			const rows = height ?? 10;
			while (out.length < rows - 1) out.push("");
			out.push(theme.fg("dim", ` ${this.#rows[this.#cursor]?.[3]} · Enter resumes · Alt-Enter new window`));
			return out.slice(0, rows);
		},
	);

	constructor(tui: TUI, done: () => void) {
		this.#tui = tui;
		this.#done = done;
	}

	handleInput(data: string): void {
		if (matchesKey(data, "escape") || matchesKey(data, "ctrl+c")) return this.#done();
		if (matchesKey(data, "tab")) this.#focus = this.#focus === "body" ? "scope" : "body";
		if (matchesKey(data, "down")) this.#cursor = Math.min(this.#rows.length - 1, this.#cursor + 1);
		if (matchesKey(data, "up")) this.#cursor = Math.max(0, this.#cursor - 1);
		this.#frame.invalidate();
		this.#tui.requestRender();
	}

	invalidate(): void {
		this.#frame.invalidate();
	}

	render(width: number): readonly string[] {
		const rows = Math.max(16, this.#tui.terminal?.rows || process.stdout.rows || 40);
		return this.#frame.render(width, rows, this.#entries, "⏎ resume · Tab panes · ↑/↓ move · type to search · esc close");
	}
}

initThemeSync();
const tui = new TUI(new ProcessTerminal());
const hub = new SpikeHub(tui, () => {
	tui.stop();
	process.exit(0);
});
tui.addChild(hub);
tui.setFocus(hub);
tui.start();
