// Small render helpers: bars, padding, the usage panel.
import { theme } from "../tui/theme";
import { fit } from "../tui/text";
import type { UsageRecord } from "../integrations/omarchy";
import { ago, epoch, now, oneline, span, tokens } from "../core/util";

export { fit };

/** A meter whose colour runs green → yellow → red as it fills. */
export function bar(p: number, width: number): string {
	const v = Math.max(0, Math.min(1, p));
	const filled = Math.round(v * width);
	return theme.level(v, "█".repeat(filled)) + theme.fg("dim", "░".repeat(width - filled));
}

export function usageLines(rec: UsageRecord | undefined, width: number): string[] {
	if (!rec) return [theme.fg("dim", "No plan limits to track (a harness or a plain TUI).")];
	const out: string[] = [];
	const limits = rec.limits ?? [];
	const tier = rec.tierLabel ? `  ${theme.fg("accent", rec.tierLabel)}` : "";
	out.push(theme.bold("Usage") + tier);
	if (!limits.length && !rec.balance) {
		const status = rec.usageStatusText || "";
		const help = rec.authHelpText || "";
		if (status || help) {
			out.push(theme.fg("warning", status || "No limits yet"));
			if (help) out.push(theme.fg("dim", oneline(help, width * 2)));
		} else out.push(theme.fg("dim", "No plan limits; billed through whatever provider it's pointed at."));
	}
	const bw = Math.max(10, Math.min(28, width - 30));
	for (const l of limits) {
		const p = Number(l.percent) || 0;
		const label = (l.title || (l.label || "Limit").split(" (")[0] || "Limit").padEnd(10).slice(0, 10);
		const reset = epoch(l.resetsAt);
		const left = !reset ? "" : reset > now() ? `resets ${span(reset - now())}` : "reset due";
		out.push(`${label} ${bar(p, bw)} ${String(Math.round(p * 100)).padStart(3)}%  ${theme.fg("dim", left)}`);
	}
	if (rec.balance) {
		const rem = rec.balance.remaining ?? 0;
		const funded = rec.balance.funded ?? 0;
		out.push(
			`${"Balance".padEnd(10)} ${bar(funded ? 1 - rem / funded : 0, bw)} ${rem.toFixed(2)} ${rec.balance.currency ?? "USD"} left of ${funded.toFixed(2)}`,
		);
	}
	if (limits.length && rec.limitsStale)
		out.push(`${theme.fg("warning", "⚠ limits not current")} ${theme.fg("dim", oneline(rec.authHelpText || rec.usageStatusText || "", width))}`);
	if (rec.hasLocalStats || rec.todayPrompts)
		out.push(
			`${theme.fg("dim", "Today")} ${rec.todayPrompts ?? 0} prompts · ${rec.todaySessions ?? 0} sessions · ${tokens(rec.todayTotalTokens)} tokens  ${theme.fg("dim", "All time")} ${rec.totalPrompts ?? 0}`,
		);
	out.push(theme.fg("dim", `updated ${ago(epoch(rec.updatedAt))} · Ctrl-R refresh`));
	return out;
}
