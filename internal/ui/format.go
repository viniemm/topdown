package ui

import (
	"fmt"
	"strings"

	"github.com/viniemm/topdown/internal/omarchy"
	"github.com/viniemm/topdown/internal/theme"
	"github.com/viniemm/topdown/internal/util"
)

// bar is a meter: each filled cell takes its colour from its place on a
// green → yellow → red gradient, so a fuller meter reaches further into red.
func bar(p float64, w int) string {
	v := min(1, max(0, p))
	filled := int(v*float64(w) + 0.5)
	if !theme.Rich() {
		return theme.Level(v, strings.Repeat("█", filled)) + theme.Fg(theme.Dim, strings.Repeat("░", w-filled))
	}
	var b strings.Builder
	for i := range filled {
		b.WriteString(theme.Level(float64(i)/float64(max(1, w-1)), "█"))
	}
	return b.String() + theme.Fg(theme.Dim, strings.Repeat("░", w-filled))
}

func usageLines(u *omarchy.Usage, w int) []string {
	dim := func(s string) string { return theme.Fg(theme.Dim, s) }
	if u == nil {
		return []string{dim("No plan limits to track (a harness or a plain TUI).")}
	}
	head := theme.Bold("Usage")
	if u.TierLabel != "" {
		head += "  " + theme.Fg(theme.Accent, u.TierLabel)
	}
	out := []string{head}
	if len(u.Limits) == 0 && u.Balance == nil {
		if u.UsageStatusText != "" || u.AuthHelpText != "" {
			status := u.UsageStatusText
			if status == "" {
				status = "No limits yet"
			}
			out = append(out, theme.Fg(theme.Warning, status))
			if u.AuthHelpText != "" {
				out = append(out, dim(util.Oneline(u.AuthHelpText, w*2)))
			}
		} else {
			out = append(out, dim("No plan limits; billed through whatever provider it's pointed at."))
		}
	}
	bw := max(10, min(28, w-30))
	for _, l := range u.Limits {
		label := l.Title
		if label == "" {
			label = strings.SplitN(l.Label, " (", 2)[0]
		}
		if label == "" {
			label = "Limit"
		}
		left := ""
		if reset := util.Epoch(l.ResetsAt); reset > 0 {
			if reset > util.Now() {
				left = "resets " + util.Span(reset-util.Now())
			} else {
				left = "reset due"
			}
		}
		out = append(out, fmt.Sprintf("%s %s %3.0f%%  %s", pad(cut(label, 10), 10), bar(l.Percent, bw), l.Percent*100, dim(left)))
	}
	if b := u.Balance; b != nil {
		spent := 0.0
		if b.Funded > 0 {
			spent = 1 - b.Remaining/b.Funded
		}
		cur := b.Currency
		if cur == "" {
			cur = "USD"
		}
		out = append(out, fmt.Sprintf("%s %s %.2f %s left of %.2f", pad("Balance", 10), bar(spent, bw), b.Remaining, cur, b.Funded))
	}
	if len(u.Limits) > 0 && u.LimitsStale {
		why := u.AuthHelpText
		if why == "" {
			why = u.UsageStatusText
		}
		out = append(out, theme.Fg(theme.Warning, "⚠ limits not current")+" "+dim(util.Oneline(why, w)))
	}
	if u.HasLocalStats || u.TodayPrompts > 0 {
		out = append(out, fmt.Sprintf("%s %.0f prompts · %.0f sessions · %s tokens  %s %.0f",
			dim("Today"), u.TodayPrompts, u.TodaySessions, util.Tokens(u.TodayTotalTokens), dim("All time"), u.TotalPrompts))
	}
	return append(out, dim("updated "+util.Ago(util.Epoch(u.UpdatedAt))+" · Ctrl-R refresh"))
}
