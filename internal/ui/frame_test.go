package ui

import (
	"strings"
	"testing"
	"time"

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

// The header band is exactly the band's width at every width and keeps the
// logo whole.
func TestHeaderWidth(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, v := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(v, "")
	}
	for _, rich := range []bool{false, true} {
		if rich {
			theme.Init(&theme.Palette{Fg: theme.RGB{220, 220, 220}, Bg: theme.RGB{20, 20, 20}, Accent: theme.RGB{97, 175, 239},
				Blue: theme.RGB{97, 175, 239}, Magenta: theme.RGB{193, 98, 222}, Cyan: theme.RGB{66, 179, 194},
				Green: theme.RGB{140, 194, 101}, Yellow: theme.RGB{229, 192, 123}, Orange: theme.RGB{209, 154, 102}, Red: theme.RGB{224, 108, 117}})
		} else {
			theme.Init(nil)
		}
		for _, W := range []int{50, 64, 70, 79, 80, 90, 100, 120, 140, 200, 320} {
			h := &Hub{width: W, height: 30, live: map[string]int{"x": 1}, opts: Options{Version: "0.9.0"},
				frame: frame{title: "topdown", minW: 16, maxW: 26}}
			h.anim.init(time.Now().Add(-3 * time.Second))
			h.anim.now = time.Now()
			w := W - 4
			band := h.header()(w)
			want := logo(h.logoFrame(), "  v0.9.0")
			if w < logoWidth+30 {
				want = compactLogo(h.logoFrame())
			}
			for i, l := range band {
				if got := ansi.StringWidth(l); got != w {
					t.Fatalf("rich=%v W=%d: band line %d is %d wide, want %d: %q", rich, W, i, got, w, ansi.Strip(l))
				}
				if logoRow := ansi.Strip(want[i]); !strings.HasPrefix(ansi.Strip(l), " "+logoRow) {
					t.Fatalf("rich=%v W=%d: logo row %d cut: %q", rich, W, i, ansi.Strip(l))
				}
			}
			// And the whole frame stays rectangular.
			for i, l := range strings.Split(h.frame.render(W, 30, nil, func(int, int) []string { return nil },
				func(int, int) []string { return nil }, "", h.header()), "\n") {
				if got := ansi.StringWidth(l); got != max(40, W) {
					t.Fatalf("rich=%v W=%d: frame line %d is %d wide", rich, W, i, got)
				}
			}
		}
	}
}
