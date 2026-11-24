package ui

// The scenery: a small landscape in the header band, between the logo and
// the stats. It is drawn procedurally for the space it has (no art is copied
// from anywhere): seeded peaks and sine hills, a sun or moon, a lake.
//
//	night  a moon, twinkling stars, blue ridges, its reflection on the lake
//	dawn   a low sun on the left, rising through a gap in the hills
//	day    a high sun, snow on the tallest peaks, green hills, clouds drifting
//	dusk   the sun setting on the right behind magenta ridges
//
// The scene follows the local hour (TOPDOWN_SCENE=night|dawn|day|dusk forces
// one) and the terrain is reseeded daily. Rows 0..2 are half-block pixels
// (two per cell: 6 tall, about square); row 3 is the water, in text. It moves
// at a few frames a second when animated, sits still with reduced motion, and
// uses the ANSI roles without true colour.

import (
	"math"
	"math/rand/v2"
	"os"
	"strings"
	"time"

	"github.com/viniemm/topdown/internal/theme"
)

const (
	sceneRows       = 4
	skyPx           = 6 // pixel rows above the water
	minSceneW       = 14
	sceneGap        = 3 // columns kept clear on each side
	sceneryInterval = 250 * time.Millisecond
)

type sceneKind int

const (
	night sceneKind = iota
	dawn
	day
	dusk
)

var sceneNames = []string{"night", "dawn", "day", "dusk"}

func sceneAt(t time.Time) sceneKind {
	if s := os.Getenv("TOPDOWN_SCENE"); s != "" {
		for i, n := range sceneNames {
			if n == s {
				return sceneKind(i)
			}
		}
	}
	switch hr := t.Hour(); {
	case hr >= 5 && hr < 8:
		return dawn
	case hr >= 8 && hr < 17:
		return day
	case hr >= 17 && hr < 20:
		return dusk
	}
	return night
}

type star struct {
	x, row       int
	phase, speed float64
}

type cloud struct {
	x, speed float64
	y, n     int // top pixel row, length in columns
}

// scene is the fixed part of a landscape: built once per width, kind and day.
type scene struct {
	kind           sceneKind
	w, day         int
	far, mid, near []float64 // ridge heights in pixels, back to front
	bx, by, br     float64   // sun or moon: centre (columns, pixels) and radius
	stars          []star
	clouds         []cloud
}

// envelope tapers things toward the scene's edges so it ends softly, not as a box.
func (s *scene) envelope(x int) float64 {
	k := min(6.0, float64(s.w)/5)
	ss := func(v float64) float64 { v = min(1, max(0, v/k)); return v * v * (3 - 2*v) }
	return ss(float64(x)+0.5) * ss(float64(s.w-x)-0.5)
}

// ridge is a line of peaks: n of them, h0..h0+dh tall, sloping 45° or steeper.
func ridge(r *rand.Rand, w, n int, h0, dh float64) []float64 {
	type peak struct{ x, h, slope float64 }
	ps := make([]peak, n)
	for i := range ps {
		ps[i] = peak{(float64(i) + 0.15 + 0.7*r.Float64()) / float64(n) * float64(w), h0 + dh*r.Float64(), 0.7 + 0.5*r.Float64()}
	}
	out := make([]float64, w)
	for x := range out {
		for _, p := range ps {
			out[x] = max(out[x], p.h-p.slope*math.Abs(float64(x)-p.x))
		}
		out[x] += 0.4 * (r.Float64() - 0.5) // a ragged crest
	}
	return out
}

func buildScene(kind sceneKind, w, dayN int) *scene {
	r := rand.New(rand.NewPCG(uint64(kind)+1, uint64(dayN)))
	s := &scene{kind: kind, w: w, day: dayN}
	fw := float64(w)

	// Sun and moon sit on whole pixels, so their discs come out round.
	at := func(lo, span float64) float64 { return math.Round(fw * (lo + span*r.Float64())) }
	switch kind {
	case night:
		s.bx, s.by, s.br = at(0.55, 0.3), 2, 2
	case dawn:
		s.bx, s.by, s.br = at(0.15, 0.15), 4, 2.6
	case day:
		s.bx, s.by, s.br = at(0.7, 0.2), 2, 2
	case dusk:
		s.bx, s.by, s.br = at(0.7, 0.15), 4, 2.6
	}

	tall := 0.0
	if kind == day {
		tall = 0.4 // room for snow
	}
	s.far = ridge(r, w, max(2, w/11), 3.6+tall, 2.4)
	s.mid = ridge(r, w, max(2, w/8), 2, 1.6)
	p1, p2 := r.Float64()*2*math.Pi, r.Float64()*2*math.Pi
	s.near = make([]float64, w)
	for x := range w {
		fx := float64(x)
		e := s.envelope(x)
		// A low sun shows through a gap in the hills; a high one or the moon
		// clears the peaks.
		gap, clear := 1.0, math.Inf(1)
		if kind == dawn || kind == dusk {
			gap = 1 - 0.75*math.Exp(-math.Pow((fx+0.5-s.bx)/(s.br+2.5), 2))
		} else {
			clear = float64(skyPx) - s.by - s.br + max(0, math.Abs(fx+0.5-s.bx)-s.br)
		}
		s.far[x] = min(s.far[x]*gap, clear) * e
		s.mid[x] = min(s.mid[x]*gap, clear) * e
		s.near[x] = min((0.9+0.7*math.Sin(fx*0.17+p1)+0.4*math.Sin(fx*0.41+p2))*gap, clear) * e
	}

	if kind == night {
		for range w / 4 {
			s.stars = append(s.stars, star{x: r.IntN(w), row: r.IntN(2), phase: r.Float64() * 2 * math.Pi, speed: 0.5 + 1.5*r.Float64()})
		}
	} else {
		for range w / 22 { // none in a narrow scene: a cloud would fill it
			s.clouds = append(s.clouds, cloud{x: r.Float64() * fw, speed: 0.25 + 0.35*r.Float64(), y: r.IntN(2), n: 4 + r.IntN(6)})
		}
	}
	return s
}

// sceneColors are a scene's colours, from the palette: further back is closer
// to the background, as in haze.
type sceneColors struct {
	far, mid, near, snow, body, body2, cloud, water, crest, star theme.RGB
}

func colorsFor(kind sceneKind) sceneColors {
	p := theme.Colors()
	bg, fg := p.Bg, p.Fg
	m := theme.Mix
	white := theme.RGB{255, 255, 255}
	switch kind {
	case dawn:
		return sceneColors{far: m(p.Magenta, bg, 0.6), mid: m(m(p.Magenta, p.Blue, 0.5), bg, 0.45), near: m(p.Blue, bg, 0.35),
			body: m(p.Yellow, fg, 0.3), body2: m(p.Orange, p.Magenta, 0.3),
			cloud: m(p.Magenta, fg, 0.5), water: m(p.Blue, bg, 0.2), crest: m(p.Magenta, fg, 0.45)}
	case day:
		return sceneColors{far: m(p.Blue, bg, 0.45), mid: m(m(p.Blue, p.Green, 0.5), bg, 0.35), near: m(p.Green, bg, 0.2), snow: m(fg, white, 0.4),
			body: p.Yellow, body2: m(p.Yellow, p.Orange, 0.4),
			cloud: m(fg, white, 0.2), water: m(p.Cyan, bg, 0.25), crest: m(p.Cyan, fg, 0.45)}
	case dusk:
		return sceneColors{far: m(p.Magenta, bg, 0.55), mid: m(m(p.Magenta, p.Red, 0.4), bg, 0.5), near: m(m(p.Magenta, p.Blue, 0.6), bg, 0.55),
			body: m(p.Yellow, p.Orange, 0.3), body2: m(p.Orange, p.Red, 0.6),
			cloud: m(p.Orange, fg, 0.4), water: m(p.Magenta, bg, 0.3), crest: m(p.Orange, fg, 0.3)}
	}
	return sceneColors{far: m(m(p.Blue, fg, 0.3), bg, 0.55), mid: m(p.Blue, bg, 0.6), near: m(m(p.Blue, p.Magenta, 0.3), bg, 0.75),
		body: m(p.Yellow, fg, 0.6), body2: m(p.Yellow, fg, 0.75),
		water: m(p.Blue, bg, 0.3), crest: m(p.Cyan, fg, 0.35), star: fg}
}

// px is one half-block pixel: a true colour, or a role without one.
type px struct {
	on   bool
	c    theme.RGB
	role theme.Role
}

// cell is one character of the finished scene.
type cell struct {
	ch     string
	styled bool
	fg, bg theme.RGB
	hasBg  bool
	role   theme.Role
}

// render draws the scene at t seconds; alpha (0..1) fades it in from the background.
func (s *scene) render(t, alpha float64) []string {
	rich := theme.Rich()
	var col sceneColors
	var bg theme.RGB
	if rich {
		col, bg = colorsFor(s.kind), theme.Bg()
	}
	w := s.w
	grid := make([][]px, skyPx)
	for y := range grid {
		grid[y] = make([]px, w)
	}
	set := func(x, y int, c theme.RGB, role theme.Role) {
		if x >= 0 && x < w && y >= 0 && y < skyPx {
			grid[y][x] = px{true, c, role}
		}
	}

	// A ridge fills each column up to its height.
	layer := func(hs []float64, c theme.RGB, role theme.Role) {
		for x := range w {
			for y := range skyPx {
				if hs[x] >= float64(skyPx-y)-0.5 {
					set(x, y, c, role)
				}
			}
		}
	}
	// Sun or moon.
	for y := range skyPx {
		for x := range w {
			if math.Hypot(float64(x)+0.5-s.bx, float64(y)+0.5-s.by) <= s.br {
				c := theme.Mix(col.body, col.body2, min(1, max(0, (float64(y)-s.by+s.br)/(2*s.br))))
				set(x, y, c, theme.Warning)
			}
		}
	}
	// Far ridges, with snow on the tallest (day).
	for x := range w {
		for y := range skyPx {
			if s.far[x] >= float64(skyPx-y)-0.5 {
				c, role := col.far, theme.Dim
				if s.kind == day && s.far[x] > 4.6 && float64(skyPx-y)-0.5 > s.far[x]-1 {
					c, role = col.snow, theme.Muted
				}
				set(x, y, c, role)
			}
		}
	}
	layer(s.mid, col.mid, theme.Border)
	// Clouds drift right and wrap round.
	for _, c := range s.clouds {
		span := float64(w + c.n + 4)
		cx := int(math.Floor(math.Mod(c.x+c.speed*t, span))) - c.n - 2
		for i := range c.n {
			x := cx + i
			if x < 0 || x >= w {
				continue
			}
			if s.envelope(x) < 0.5 {
				continue // clouds drift in and out of view, not across the edge
			}
			puff := func(y int) { set(x, y, col.cloud, theme.Muted) }
			puff(c.y + 1)
			if i > 0 && i < c.n-1 && (i+c.n)%3 != 0 {
				puff(c.y)
			}
		}
	}
	layer(s.near, col.near, theme.Success)

	rows := make([][]cell, sceneRows)
	for r := range skyPx / 2 {
		rows[r] = make([]cell, w)
		for x := range w {
			top, bot := grid[2*r][x], grid[2*r+1][x]
			var c cell
			switch {
			case !top.on && !bot.on:
				c = cell{ch: " "}
			case top.on && !bot.on:
				c = cell{ch: "▀", styled: true, fg: top.c, role: top.role}
			case !top.on:
				c = cell{ch: "▄", styled: true, fg: bot.c, role: bot.role}
			case rich && top.c == bot.c, !rich && top.role == bot.role:
				c = cell{ch: "█", styled: true, fg: top.c, role: top.role}
			case rich:
				c = cell{ch: "▀", styled: true, fg: top.c, bg: bot.c, hasBg: true}
			default: // two roles in one cell: the nearer (lower) one takes it
				c = cell{ch: "█", styled: true, role: bot.role}
			}
			rows[r][x] = c
		}
	}
	// Stars, in the empty sky.
	for _, st := range s.stars {
		if rows[st.row][st.x].styled {
			continue
		}
		b := 0.5 + 0.5*math.Sin(t*st.speed+st.phase)
		ch := "·"
		switch {
		case b > 0.92:
			ch = "*"
		case b > 0.7:
			ch = "+"
		}
		rows[st.row][st.x] = cell{ch: ch, styled: true, fg: theme.Mix(bg, col.star, (0.2+0.6*b)*s.envelope(st.x)), role: theme.Dim}
	}
	// The water: a shimmer of crests, and the sun's or moon's reflection.
	water := make([]cell, w)
	for x := range w {
		fx := float64(x)
		v := 0.5 + 0.5*math.Sin(fx*0.9+t*2.2+1.5*math.Sin(fx*0.31-t*0.7))
		ch := "─"
		if v > 0.5 {
			ch = "~"
		}
		c, role := theme.Mix(col.water, col.crest, v), theme.Accent
		reach := s.br + 0.6 + 0.7*math.Sin(t*3.1+fx*1.7)
		if s.kind != day && math.Abs(fx+0.5-s.bx) <= reach {
			c, role, ch = theme.Mix(c, col.body, 0.75), theme.Warning, "~"
		}
		if rich {
			c = theme.Mix(bg, c, s.envelope(x))
		}
		water[x] = cell{ch: ch, styled: true, fg: c, role: role}
	}
	rows[sceneRows-1] = water

	out := make([]string, sceneRows)
	for r, cells := range rows {
		out[r] = paintCells(cells, rich, bg, alpha)
	}
	return out
}

// paintCells writes a row, one escape per run of like-coloured cells.
func paintCells(cells []cell, rich bool, bg theme.RGB, alpha float64) string {
	var b, run strings.Builder
	var cur cell
	flush := func() {
		if run.Len() == 0 {
			return
		}
		s := run.String()
		switch {
		case !cur.styled:
		case !rich:
			s = theme.Fg(cur.role, s)
		case cur.hasBg:
			s = theme.BgRGB(theme.Mix(bg, cur.bg, alpha), theme.FgRGB(theme.Mix(bg, cur.fg, alpha), s))
		default:
			s = theme.FgRGB(theme.Mix(bg, cur.fg, alpha), s)
		}
		b.WriteString(s)
		run.Reset()
	}
	for i, c := range cells {
		same := c.styled == cur.styled && (!c.styled || (rich && c.fg == cur.fg && c.hasBg == cur.hasBg && c.bg == cur.bg) || (!rich && c.role == cur.role))
		if i == 0 || !same {
			flush()
			cur = c
		}
		run.WriteString(c.ch)
	}
	flush()
	return b.String()
}

// scenery is the landscape for w columns now, or nil when w is too narrow.
func (h *Hub) scenery(w int, reveal float64) []string {
	if w < minSceneW {
		return nil
	}
	now := h.anim.now
	kind, dayN := sceneAt(now), now.YearDay()+1000*now.Year()
	if s := h.scene; s == nil || s.w != w || s.kind != kind || s.day != dayN {
		h.scene = buildScene(kind, w, dayN)
	}
	t, alpha := 0.0, 1.0
	if theme.Animated() {
		t, alpha = now.Sub(h.anim.started).Seconds(), reveal
		h.anim.scenery = true
	}
	return h.scene.render(t, alpha)
}
