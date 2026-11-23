// topdown: one TUI to launch, resume and manage coding agents.
//
//   topdown [pick]            the hub in this terminal
//   topdown open              the hub in a new terminal window (Super+Shift+A)
//   topdown default           launch the default agent; none yet -> the hub chooses (Super+A)
//   topdown launch <app-id>   launch one app from the list
//   topdown resume <agent> <session-id>
//   topdown list [agent…]     recent conversations, tab-separated (for scripts)
import { agent as findAgent } from "./agents";
import { getSession, recent } from "./core/catalog";
import { getDefault, loadApps } from "./core/config";
import { appSpec, defaultSpec, execHere, execWindow, resumeSpec, type Where } from "./core/launch";
import type { ExecSpec } from "./core/types";
import { oneline, stamp } from "./core/util";
import { focusWindow } from "./integrations/hyprland";
import { isOmarchy, WINDOW_CLASS } from "./integrations/omarchy";
import { App as Ui } from "./tui/app";
import { detectPalette } from "./tui/palette";
import { initTheme } from "./tui/theme";
import { Hub, type HubResult } from "./ui/hub";

/** How to run topdown itself: the wrapper sets TOPDOWN_SELF; otherwise this runtime + script. */
function selfArgv(): string[] {
	if (process.env.TOPDOWN_SELF) return [process.env.TOPDOWN_SELF];
	return Bun.main.endsWith(".ts") ? [process.execPath, Bun.main] : [process.execPath];
}

async function run(spec: ExecSpec, where: Where, title: string): Promise<never> {
	if (where === "window") {
		execWindow(spec);
		process.exit(0);
	}
	return execHere(spec, title);
}

async function pick(chooseDefault: boolean): Promise<never> {
	initTheme(await detectPalette());
	const ui = new Ui();
	const result = await new Promise<HubResult>(resolve => ui.start(new Hub(ui, resolve, { chooseDefault })));
	ui.stop();
	switch (result.action) {
		case "quit":
			process.exit(0);
		case "focus":
			focusWindow(result.address);
			process.exit(0);
		case "launch":
			return run(appSpec(result.app, result.where, result.extra), result.where, result.app.label);
		case "resume":
			return run(resumeSpec(result.s, result.where, result.extra), result.where, `${result.s.agent} — ${oneline(result.s.title, 60)}`);
	}
}

function openWindow(args: string[]): never {
	const argv = ["xdg-terminal-exec", `--app-id=${WINDOW_CLASS}`, "--title=topdown", "-e", ...selfArgv(), "pick", ...args];
	const full = Bun.which("uwsm-app") ? ["uwsm-app", "--", ...argv] : argv;
	Bun.spawnSync(["setsid", "-f", ...full], { stdin: "ignore", stdout: "ignore", stderr: "ignore" });
	process.exit(0);
}

async function main(): Promise<void> {
	const [cmd = "pick", ...args] = process.argv.slice(2);
	switch (cmd) {
		case "pick":
			return pick(args.includes("--choose-default"));
		case "open":
			return openWindow(args);
		case "default": {
			const id = getDefault();
			if (!id) return openWindow(["--choose-default"]);
			execWindow(defaultSpec(id, "window"));
			process.exit(0);
		}
		case "launch": {
			const app = loadApps().find(a => a.id === args[0]);
			if (!app) throw new Error(`No app named '${args[0] ?? ""}'`);
			const where: Where = process.env.TOPDOWN_WINDOW ? "window" : "here";
			return run(appSpec(app, where), where, app.label);
		}
		case "resume": {
			const s = args[0] && args[1] ? getSession(args[0], args[1]) : undefined;
			if (!s) throw new Error("usage: topdown resume <agent> <session-id> (session not found)");
			return run(resumeSpec(s, "here"), "here", s.title);
		}
		case "list": {
			const agents = args.length ? args : [...new Set(loadApps().map(a => a.agent).filter((a): a is string => !!findAgent(a)?.sessions))];
			for (const s of recent(agents, 50).sessions) console.log([stamp(s.when), s.agent, s.id, s.cwd, s.title].join("\t"));
			return;
		}
		case "-h":
		case "--help":
		case "help":
			console.log(
				"topdown [pick|open|default|launch <app>|resume <agent> <id>|list [agent…]]" + (isOmarchy() ? "  (Omarchy integration on)" : ""),
			);
			return;
		default:
			throw new Error(`unknown command: ${cmd}`);
	}
}

main().catch(e => {
	console.error(`topdown: ${e instanceof Error ? e.message : e}`);
	process.exit(1);
});
