// Package theme decides colours. They come from, best first:
//  1. Omarchy's current theme (~/.local/state/omarchy/current/theme/colors.toml):
//     its semantic palette, so topdown follows `omarchy theme set`.
//  2. The terminal itself: OSC 10/11 (fg/bg) and OSC 4 (the 16 colours) queries,
//     so any themed terminal works without Omarchy.
//  3. Nothing known: the 16 ANSI colour codes.
package theme

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/viniemm/topdown/internal/util"
)

type RGB [3]uint8

type Palette struct {
	Fg, Bg, Accent, Red, Green, Yellow, Blue, Magenta, Cyan, Orange RGB
}

func Hex(s string) (RGB, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return RGB{}, false
	}
	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return RGB{}, false
	}
	return RGB{uint8(n >> 16), uint8(n >> 8), uint8(n)}, true
}

// Mix blends a toward b by t (0..1).
func Mix(a, b RGB, t float64) RGB {
	var out RGB
	for i := range out {
		out[i] = uint8(float64(a[i]) + (float64(b[i])-float64(a[i]))*t + 0.5)
	}
	return out
}

// ---- 1. Omarchy theme ----

var tomlLine = regexp.MustCompile(`^\s*([a-z_0-9]+)\s*=\s*"(#[0-9a-fA-F]{6})"`)

func readOmarchy() *Palette {
	fh, err := os.Open(filepath.Join(util.StateHome(), "omarchy/current/theme/colors.toml"))
	if err != nil {
		return nil
	}
	defer fh.Close()
	kv := map[string]RGB{}
	sc := bufio.NewScanner(fh)
	for sc.Scan() {
		if m := tomlLine.FindStringSubmatch(sc.Text()); m != nil {
			kv[m[1]], _ = Hex(m[2])
		}
	}
	pick := func(names ...string) (RGB, bool) {
		for _, n := range names {
			if c, ok := kv[n]; ok {
				return c, true
			}
		}
		return RGB{}, false
	}
	fg, okF := pick("fg", "foreground")
	bg, okB := pick("bg", "background")
	if !okF || !okB {
		return nil
	}
	c := func(name string, n int) RGB {
		if v, ok := pick(name, "color"+strconv.Itoa(n)); ok {
			return v
		}
		return fg
	}
	p := &Palette{Fg: fg, Bg: bg,
		Red: c("red", 1), Green: c("green", 2), Yellow: c("yellow", 3), Blue: c("blue", 4), Magenta: c("magenta", 5), Cyan: c("cyan", 6)}
	p.Accent, _ = pick("accent")
	if _, ok := kv["accent"]; !ok {
		p.Accent = p.Blue
	}
	if o, ok := kv["orange"]; ok {
		p.Orange = o
	} else {
		p.Orange = p.Yellow
	}
	return p
}

// ---- 2. Ask the terminal ----

var oscReply = regexp.MustCompile(`(?i)\x1b\](\d+);(?:(\d+);)?rgb:([0-9a-f]{1,4})/([0-9a-f]{1,4})/([0-9a-f]{1,4})(?:\x07|\x1b\\)`)
var da1Reply = regexp.MustCompile(`\x1b\[\?[\d;]*c`)

func channel(h string) uint8 {
	n, _ := strconv.ParseUint(h, 16, 32)
	maxv := uint64(1)<<(4*len(h)) - 1
	return uint8(float64(n)/float64(maxv)*255 + 0.5)
}

// queryTerminal asks for fg, bg and the 16 colours. A DA1 request goes last:
// every terminal answers it, so its reply means all the colour replies that
// are coming have arrived (no fixed wait). Gives up after the timeout.
func queryTerminal(timeout time.Duration) *Palette {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil
	}
	defer tty.Close()
	old, err := term.MakeRaw(int(tty.Fd()))
	if err != nil {
		return nil
	}
	defer term.Restore(int(tty.Fd()), old)

	var q strings.Builder
	q.WriteString("\x1b]10;?\x1b\\\x1b]11;?\x1b\\")
	for n := 0; n < 16; n++ {
		q.WriteString("\x1b]4;" + strconv.Itoa(n) + ";?\x1b\\")
	}
	q.WriteString("\x1b[c")
	tty.WriteString(q.String())

	deadline := time.Now().Add(timeout)
	tty.SetReadDeadline(deadline)
	var buf []byte
	chunk := make([]byte, 1024)
	for time.Now().Before(deadline) && !da1Reply.Match(buf) {
		n, err := tty.Read(chunk)
		buf = append(buf, chunk[:n]...)
		if err != nil {
			break
		}
	}
	var fg, bg *RGB
	ansi := map[int]RGB{}
	for _, m := range oscReply.FindAllStringSubmatch(string(buf), -1) {
		c := RGB{channel(m[3]), channel(m[4]), channel(m[5])}
		switch m[1] {
		case "10":
			fg = &c
		case "11":
			bg = &c
		case "4":
			if n, err := strconv.Atoi(m[2]); err == nil {
				ansi[n] = c
			}
		}
	}
	if fg == nil || bg == nil || len(ansi) < 8 {
		return nil
	}
	a := func(n int) RGB {
		if c, ok := ansi[n]; ok {
			return c
		}
		return *fg
	}
	return &Palette{Fg: *fg, Bg: *bg, Accent: a(4), Red: a(1), Green: a(2), Yellow: a(3),
		Blue: a(4), Magenta: a(5), Cyan: a(6), Orange: Mix(a(1), a(3), 0.5)}
}

// Detect finds the palette, or nil when neither source answers.
func Detect() *Palette {
	if p := readOmarchy(); p != nil {
		return p
	}
	return queryTerminal(300 * time.Millisecond)
}
