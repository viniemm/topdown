package ui

// The logo, after omp's header: a block mark with a diagonal gradient, the
// name in a half-block face, and the version in dim text. The mark is a bar
// over a downward wedge: things seen from the top down.
//
//	▀▀▀▀▀▀▀▀▀    █               █
//	▀███████▀   ▀█▀ ▄▀▀▄ █▀▀▄ ▄▀▀█ ▄▀▀▄ █ █ █ █▀▀▄
//	 ▀█████▀     █▄ ▀▄▄▀ █▄▄▀ ▀▄▄█ ▀▄▄▀ ▀▄▀▄▀ █  █
//	   ▀█▀               █      v0.8.0

import (
	"math"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/viniemm/topdown/internal/theme"
)

var (
	logoMark = []string{
		"▀▀▀▀▀▀▀▀▀",
		"▀███████▀",
		" ▀█████▀ ",
		"   ▀█▀   ",
	}
	// Letters are 4 rows: ascender, two x-height rows of half blocks, descender.
	logoWord = joinGlyphs([][4]string{
		{" █ ", "▀█▀", " █▄", "   "},         // t
		{"    ", "▄▀▀▄", "▀▄▄▀", "    "},     // o
		{"    ", "█▀▀▄", "█▄▄▀", "█   "},     // p
		{"   █", "▄▀▀█", "▀▄▄█", "    "},     // d
		{"    ", "▄▀▀▄", "▀▄▄▀", "    "},     // o
		{"     ", "█ █ █", "▀▄▀▄▀", "     "}, // w
		{"    ", "█▀▀▄", "█  █", "    "},     // n
	})
)

// joinGlyphs sets glyphs side by side, one column apart.
func joinGlyphs(glyphs [][4]string) []string {
	rows := make([]string, 4)
	for i, g := range glyphs {
		for r := range rows {
			if i > 0 {
				rows[r] += " "
			}
			rows[r] += g[r]
		}
	}
	return rows
}

const (
	logoGap   = "   "
	logoWidth = 9 + 3 + 34 // mark + gap + word
	// versionCol is where the version starts on the last row: past the p's descender.
	versionCol = 9 + 3 + 3 + 1 + 4 + 1 + 4 + 3
)

// logoFrame is one moment of the logo's animation.
type logoFrame struct {
	// reveal: 0..1, how much has been drawn, left to right (the opening sweep).
	reveal float64
	// shimmer: -1 for none, else the position (0..1) of a bright band crossing it.
	shimmer float64
}

// logo renders the three logo rows, then the version beside the descender row.
func logo(f logoFrame, version string) []string {
	if !theme.Rich() {
		out := make([]string, len(logoMark))
		for r := range logoMark {
			out[r] = theme.Fg(theme.Accent, logoMark[r]) + logoGap + theme.Bold(logoWord[r])
		}
		last := len(out) - 1
		out[last] = ansi.Truncate(out[last], versionCol, "") + dim(version)
		return out
	}
	stops := theme.LogoStops()
	fg := theme.FgColor()
	rows := len(logoMark)
	out := make([]string, rows)
	for r := range rows {
		var b strings.Builder
		line := []rune(logoMark[r] + logoGap + logoWord[r])
		markW := len([]rune(logoMark[r]))
		for x, ch := range line {
			// Diagonal gradient: each row starts a little further along, like omp's.
			t := (float64(x) + float64(r)*3) / float64(logoWidth+rows*3)
			if ch == ' ' || t > f.reveal*1.15 {
				b.WriteByte(' ')
				continue
			}
			c := theme.Grad(stops, t)
			if x >= markW { // the name: the same gradient, softened toward the text colour
				c = theme.Mix(c, fg, 0.35)
			}
			if f.shimmer >= 0 {
				if d := math.Abs(t - f.shimmer); d < 0.09 {
					c = theme.Mix(c, theme.Mix(fg, theme.RGB{255, 255, 255}, 0.5), (1-d/0.09)*0.75)
				}
			}
			s := string(ch)
			if x >= markW {
				s = theme.Bold(s)
			}
			b.WriteString(theme.FgRGB(c, s))
		}
		out[r] = b.String()
	}
	if f.reveal >= 1 && version != "" {
		last := len(out) - 1
		out[last] = ansi.Truncate(out[last], versionCol, "") + dim(version)
	}
	return out
}

// compactLogo is the mark and plain name, for narrow windows.
func compactLogo(f logoFrame) []string {
	full := logo(f, "")
	out := make([]string, len(full))
	for r := range full {
		out[r] = ansi.Truncate(full[r], 9, "")
	}
	out[2] += "  " + theme.Bold("topdown")
	return out
}
