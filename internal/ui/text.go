// Package ui is the Bubble Tea hub: the omp-style frame, the list, modals and
// the launch sheet.
package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// closeStyle resets what a cut may leave open (colour, bold, italic,
// underline) but not a background, so a selected row keeps its highlight.
const closeStyle = "\x1b[39;22;23;24m"

func width(s string) int { return ansi.StringWidth(s) }

// truncate cuts s to at most w columns, ending with "…" when something was dropped.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if width(s) <= w {
		return s
	}
	return ansi.Truncate(s, w-1, "") + closeStyle + "…"
}

// fit is exactly w columns: cut, or padded with spaces.
func fit(s string, w int) string {
	c := truncate(s, w)
	if pad := w - width(c); pad > 0 {
		return c + strings.Repeat(" ", pad)
	}
	return c
}

// spread right-aligns right after left within w columns (left is cut first).
func spread(left, right string, w int) string {
	rw := width(right)
	if rw+1 >= w {
		return fit(left, w)
	}
	return fit(left, w-rw) + right
}

func pad(s string, n int) string {
	if d := n - width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}
