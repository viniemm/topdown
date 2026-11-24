package ui

// Animation: Bubble Tea ticks drive a few short effects, and the ticker only
// runs while something is moving.
//
//	intro     the logo sweeps in when the hub opens (~0.9 s)
//	shimmer   a bright band crosses the logo every few seconds
//	fade      the selection bar fades in when the cursor moves
//	pulse     open conversations' ● markers breathe
//	spinner   while usage refreshes in the background
//	scenery   the header's landscape: stars, clouds, water (4 fps)
//
// All of it is skipped without true colour or with reduced motion
// ("animations": false in theme.json, or TOPDOWN_REDUCED_MOTION=1), except
// the spinner, which is plain text.

import (
	"math"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/viniemm/topdown/internal/theme"
)

const (
	fps           = 30
	introDur      = 900 * time.Millisecond
	shimmerDur    = 900 * time.Millisecond
	shimmerEvery  = 7 * time.Second
	fadeDur       = 160 * time.Millisecond
	pulsePeriod   = 1800 * time.Millisecond
	pulseInterval = 100 * time.Millisecond
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// tickMsg carries its schedule generation: a newer schedule supersedes older
// pending ticks, so there is never more than one live chain.
type tickMsg struct {
	at  time.Time
	gen int
}

type animState struct {
	started      time.Time
	now          time.Time
	shimmerStart time.Time
	nextShimmer  time.Time
	selAt        time.Time
	gen          int
	pending      time.Time // when the pending tick fires (zero: none)
	pulsing      bool      // an open-conversation marker is on screen (keeps frames coming)
	bodyPulsing  bool      // …in the body, which the header-only repaint can't refresh
	scenery      bool      // the animated landscape is on screen
}

func (a *animState) init(now time.Time) {
	a.started, a.now = now, now
	a.nextShimmer = now.Add(introDur + shimmerEvery/2)
}

// ease-out cubic.
func ease(t float64) float64 {
	t = min(1, max(0, t))
	return 1 - math.Pow(1-t, 3)
}

func (h *Hub) progress(start time.Time, d time.Duration) float64 {
	return float64(h.anim.now.Sub(start)) / float64(d)
}

func (h *Hub) logoFrame() logoFrame {
	if !theme.Animated() {
		return logoFrame{reveal: 1, shimmer: -1}
	}
	f := logoFrame{reveal: ease(h.progress(h.anim.started, introDur)), shimmer: -1}
	if p := h.progress(h.anim.shimmerStart, shimmerDur); !h.anim.shimmerStart.IsZero() && p < 1 {
		f.shimmer = -0.15 + 1.3*ease(p)
	}
	return f
}

// fade is how far the selection bar has faded in (1: fully).
func (h *Hub) fade() float64 {
	if !theme.Animated() {
		return 1
	}
	return ease(h.progress(h.anim.selAt, fadeDur))
}

// pulse is 0..1 on a slow sine, for breathing markers.
func (h *Hub) pulse() float64 {
	ph := float64(h.anim.now.Sub(h.anim.started)%pulsePeriod) / float64(pulsePeriod)
	return (1 - math.Cos(2*math.Pi*ph)) / 2
}

func (h *Hub) spinner() string {
	i := int(h.anim.now.Sub(h.anim.started)/(80*time.Millisecond)) % len(spinnerFrames)
	return spinnerFrames[i]
}

// nextFrame is when the next frame is needed (zero: nothing moving soon).
func (h *Hub) nextFrame() time.Time {
	now := h.anim.now
	fast := now.Add(time.Second / fps)
	if len(h.refreshing) > 0 {
		return fast
	}
	if !theme.Animated() {
		return time.Time{}
	}
	if now.Sub(h.anim.started) < introDur || now.Sub(h.anim.selAt) < fadeDur ||
		(!h.anim.shimmerStart.IsZero() && now.Sub(h.anim.shimmerStart) < shimmerDur) {
		return fast
	}
	next := h.anim.nextShimmer
	if h.anim.pulsing {
		next = now.Add(pulseInterval)
	}
	if s := now.Add(sceneryInterval); h.anim.scenery && s.Before(next) {
		next = s
	}
	return next
}

// schedule makes sure a tick is pending soon enough; it returns nil when the
// pending one already is.
func (h *Hub) schedule() tea.Cmd {
	at := h.nextFrame()
	if at.IsZero() || (!h.anim.pending.IsZero() && !h.anim.pending.After(at)) {
		return nil
	}
	h.anim.gen++
	h.anim.pending = at
	gen := h.anim.gen
	return tea.Tick(time.Until(at), func(t time.Time) tea.Msg { return tickMsg{at: t, gen: gen} })
}

func (h *Hub) onTick(m tickMsg) tea.Cmd {
	if m.gen != h.anim.gen {
		return nil // superseded
	}
	h.anim.pending = time.Time{}
	h.anim.now = m.at
	if theme.Animated() && !m.at.Before(h.anim.nextShimmer) {
		h.anim.shimmerStart = m.at
		h.anim.nextShimmer = m.at.Add(shimmerEvery)
	}
	return h.schedule()
}

// touch records activity (a key) at the current time and keeps frames coming.
func (h *Hub) touch() tea.Cmd {
	h.anim.now = time.Now()
	return h.schedule()
}
