// Package util holds paths, time and text helpers shared by every package.
package util

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Home is $HOME at call time (tests point it at a fixture home).
func Home() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return h
}

func xdg(name, fallback string) string {
	if v := os.Getenv("XDG_" + name + "_HOME"); v != "" {
		return v
	}
	return filepath.Join(Home(), fallback)
}

func ConfigHome() string { return xdg("CONFIG", ".config") }
func StateHome() string  { return xdg("STATE", ".local/state") }
func CacheHome() string  { return xdg("CACHE", ".cache") }
func DataHome() string   { return xdg("DATA", ".local/share") }

func ConfigDir() string { return filepath.Join(ConfigHome(), "topdown") }
func StateDir() string  { return filepath.Join(StateHome(), "topdown") }
func CacheDir() string  { return filepath.Join(CacheHome(), "topdown") }

// Now is seconds since the epoch.
func Now() float64 { return float64(time.Now().UnixMilli()) / 1000 }

// Epoch converts seconds, milliseconds, numeric strings or ISO times to
// seconds since the epoch; 0 if unusable.
func Epoch(v any) float64 {
	norm := func(n float64) float64 {
		if n > 1e11 {
			return n / 1000
		}
		return n
	}
	switch x := v.(type) {
	case nil:
		return 0
	case float64:
		return norm(x)
	case int64:
		return norm(float64(x))
	case int:
		return norm(float64(x))
	case json.Number:
		f, _ := x.Float64()
		return norm(f)
	case []byte:
		return Epoch(string(x))
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return 0
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return norm(f)
		}
		s = strings.Replace(s, " ", "T", 1)
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05"} {
			if t, err := time.Parse(layout, s); err == nil {
				return float64(t.UnixMilli()) / 1000
			}
		}
	}
	return 0
}

// Span formats a duration in seconds compactly: 45s, 12m, 3h 05m, 2d 4h.
func Span(seconds float64) string {
	s := int(seconds)
	if s < 0 {
		s = -s
	}
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	m := s / 60
	if m < 60 {
		return fmt.Sprintf("%dm", m)
	}
	h := m / 60
	if h < 24 {
		if m%60 != 0 {
			return fmt.Sprintf("%dh %02dm", h, m%60)
		}
		return fmt.Sprintf("%dh", h)
	}
	d := h / 24
	if h%24 != 0 {
		return fmt.Sprintf("%dd %dh", d, h%24)
	}
	return fmt.Sprintf("%dd", d)
}

func Ago(ts float64) string {
	if ts == 0 {
		return "?"
	}
	return Span(Now()-ts) + " ago"
}

// Stamp is a compact local time: 21:40 today, "Yest 14:02", "Mon 09:10" this
// week, "Nov 18" older (with the year when it isn't this year).
func Stamp(ts float64) string {
	if ts == 0 {
		return ""
	}
	t := time.UnixMilli(int64(ts * 1000)).Local()
	now := time.Now()
	day := func(x time.Time) time.Time { return time.Date(x.Year(), x.Month(), x.Day(), 0, 0, 0, 0, time.Local) }
	days := int(day(now).Sub(day(t)).Hours()/24 + 0.5)
	switch {
	case days == 0:
		return t.Format("15:04")
	case days == 1:
		return "Yest " + t.Format("15:04")
	case days > 0 && days < 7:
		return t.Format("Mon 15:04")
	case t.Year() == now.Year():
		return t.Format("Jan 02")
	}
	return t.Format("Jan 02 2006")
}

func ShortPath(p string) string {
	if h := Home(); p != "" && strings.HasPrefix(p, h) {
		return "~" + p[len(h):]
	}
	return p
}

// Project is the last path component of a working directory ("~" for $HOME).
func Project(p string) string {
	s := strings.TrimRight(p, "/")
	if s == Home() {
		return "~"
	}
	if b := filepath.Base(s); b != "." && b != "/" {
		return b
	}
	return s
}

// Oneline collapses whitespace and cuts to n runes with an ellipsis.
func Oneline(s string, n int) string {
	t := strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(t) <= n {
		return t
	}
	r := []rune(t)
	return string(r[:max(0, n-1)]) + "…"
}

// TextOf returns a message's text: a string, or a list of {type: "text", text} parts.
func TextOf(content any) string {
	switch c := content.(type) {
	case string:
		return c
	case []any:
		var parts []string
		for _, p := range c {
			if m, ok := p.(map[string]any); ok && (m["type"] == "text" || m["type"] == "input_text") {
				if s, ok := m["text"].(string); ok {
					parts = append(parts, s)
				}
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}

// IsUserText: something the user typed, not a harness insert (<command-…>
// tags, Claude's "Caveat:" notes).
func IsUserText(t string) bool {
	s := strings.TrimLeft(t, " \t\r\n")
	return s != "" && !strings.HasPrefix(s, "<") && !strings.HasPrefix(s, "Caveat:")
}

func Tokens(n float64) string {
	v := int64(n)
	for _, u := range []struct {
		unit string
		size float64
	}{{"B", 1e9}, {"M", 1e6}, {"k", 1e3}} {
		if float64(v) >= u.size {
			return fmt.Sprintf("%.1f%s", float64(v)/u.size, u.unit)
		}
	}
	return strconv.FormatInt(v, 10)
}

// Which is the first executable named name on $PATH, or "".
func Which(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

// UserBin is like omarchy-agent: a user install in ~/.local/bin wins over PATH.
func UserBin(name string) string {
	local := filepath.Join(Home(), ".local/bin", name)
	if st, err := os.Stat(local); err == nil && st.Mode()&0o111 != 0 {
		return local
	}
	if p := Which(name); p != "" {
		return p
	}
	return name
}

// WriteFileAtomic writes via a temporary file and a rename.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
