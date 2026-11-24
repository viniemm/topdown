package ui

// The hub layout, after omp's model picker:
//
//	╭─ title ──────┬────────────────────────╮
//	│ ▸ ▦ Apps   4 │ body                   │
//	│ ──────────── │                        │
//	├──────────────┴────────────────────────┤
//	│ footer key hints                      │
//	╰───────────────────────────────────────╯

import (
	"strings"

	"github.com/viniemm/topdown/internal/theme"
)

type entry struct {
	id         string
	kind       string // "apps", "recent", "agent", "separator"
	label      string
	annotation string
}

type entryStyle struct {
	icon       string
	annotation string
}

func border(s string) string { return theme.Fg(theme.Border, s) }

type frame struct {
	title  string
	minW   int
	maxW   int
	scroll int
}

func (f *frame) sidebarWidth(entries []entry) int {
	longest := 0
	for _, e := range entries {
		longest = max(longest, width(e.label)+width(e.annotation)+6)
	}
	return max(f.minW, min(f.maxW, longest))
}

// sidebar rows: cursor, icon, label, right-aligned annotation; the active row in bold accent.
func (f *frame) sidebar(entries []entry, w, rows int, active string, focused bool, style func(entry) entryStyle) []string {
	ai := 0
	for i, e := range entries {
		if e.id == active {
			ai = i
		}
	}
	if ai < f.scroll {
		f.scroll = ai
	}
	if ai >= f.scroll+rows {
		f.scroll = ai - rows + 1
	}
	f.scroll = max(0, min(f.scroll, max(0, len(entries)-rows)))
	var out []string
	for _, e := range entries[f.scroll:min(len(entries), f.scroll+rows)] {
		if e.kind == "separator" {
			out = append(out, border(strings.Repeat("─", w)))
			continue
		}
		isActive := e.id == active
		s := style(e)
		cursor := " "
		if isActive && focused {
			cursor = theme.Fg(theme.Accent, "▸")
		}
		label := e.label
		if isActive {
			label = theme.Bold(theme.Fg(theme.Accent, e.label))
		}
		line := spread(cursor+" "+s.icon+" "+label, s.annotation, w)
		if isActive && focused {
			line = theme.Selected(line)
		}
		out = append(out, line)
	}
	for len(out) < rows {
		out = append(out, strings.Repeat(" ", w))
	}
	return out
}

// render is the whole screen: height lines of exactly totalWidth columns.
func (f *frame) render(totalWidth, height int, entries []entry, sidebarFn func(w, rows int) []string,
	bodyFn func(w, rows int) []string, footer string) string {
	W := max(40, totalWidth)
	rows := max(3, height-4)
	sw := min(f.sidebarWidth(entries), max(10, (W-7)/3))
	bw := W - sw - 7
	left := sidebarFn(sw, rows)
	body := bodyFn(bw, rows)

	name := " " + f.title + " "
	var b strings.Builder
	b.WriteString(border("╭─" + truncate(name, sw) + strings.Repeat("─", max(0, sw+1-width(name))) + "┬" + strings.Repeat("─", bw+2) + "╮"))
	at := func(lines []string, i int) string {
		if i < len(lines) {
			return lines[i]
		}
		return ""
	}
	for i := 0; i < rows; i++ {
		b.WriteString("\n" + border("│") + " " + fit(at(left, i), sw) + " " + border("│") + " " + fit(at(body, i), bw) + " " + border("│"))
	}
	b.WriteString("\n" + border("├"+strings.Repeat("─", sw+2)+"┴"+strings.Repeat("─", bw+2)+"┤"))
	b.WriteString("\n" + border("│") + " " + fit(footer, W-4) + " " + border("│"))
	b.WriteString("\n" + border("╰"+strings.Repeat("─", W-2)+"╯"))
	return b.String()
}
