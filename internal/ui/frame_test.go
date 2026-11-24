package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/viniemm/topdown/internal/theme"
)

func TestFrameGeometry(t *testing.T) {
	for _, rich := range []bool{false, true} {
		if rich {
			theme.Init(&theme.Palette{Fg: theme.RGB{220, 220, 220}, Bg: theme.RGB{20, 20, 20}, Accent: theme.RGB{97, 175, 239},
				Magenta: theme.RGB{193, 98, 222}, Cyan: theme.RGB{66, 179, 194}, Green: theme.RGB{140, 194, 101}})
		} else {
			theme.Init(nil)
		}
		entries := []entry{{id: "apps", kind: "apps", label: "Apps", annotation: "4"}, {id: "sep", kind: "separator"}, {id: "claude", kind: "agent", label: "claude"}}
		f := &frame{title: "topdown", minW: 16, maxW: 26}
		side := func(w, rows int) []string {
			return f.sidebar(entries, w, rows, "apps", true, func(entry) entryStyle { return entryStyle{icon: "◆"} })
		}
		body := func(w, rows int) []string { return []string{"body", "日本語 wide"} }
		header := func(w int) []string { return logo(logoFrame{reveal: 1, shimmer: 0.4}, "  v0.8.0") }
		for _, W := range []int{60, 80, 133} {
			for _, hd := range []func(int) []string{nil, header} {
				lines := strings.Split(f.render(W, 24, entries, side, body, "⏎ open · esc close", hd), "\n")
				if len(lines) != 24 {
					t.Fatalf("W=%d header=%v: %d lines", W, hd != nil, len(lines))
				}
				for i, l := range lines {
					if got := ansi.StringWidth(l); got != W {
						t.Fatalf("W=%d header=%v line %d is %d wide: %q", W, hd != nil, i, got, ansi.Strip(l))
					}
				}
			}
		}
	}
}

func TestLogoRowsLineUp(t *testing.T) {
	for r := range logoMark {
		if len(logoWord) != len(logoMark) || len([]rune(logoMark[r])) != 9 || len([]rune(logoWord[r])) != 34 {
			t.Fatalf("row %d: mark %d, word %d", r, len([]rune(logoMark[r])), len([]rune(logoWord[r])))
		}
	}
}
