package fuzzy

import (
	"slices"
	"testing"
)

func TestRank(t *testing.T) {
	items := []string{"claude topdown Port the launcher", "pi xfin Refactor auth", "hermes topdown chat", "claude xfin authorization notes"}
	id := func(s string) string { return s }
	// Substring matches first; a scattered match (a·u·t·h across words) still counts, ranked last.
	auth := Rank(items, "auth", id)
	if len(auth) != 3 || auth[2] != items[0] || slices.Contains(auth, items[2]) {
		t.Fatalf("auth: %q", auth)
	}
	if !slices.Contains(auth[:2], items[1]) || !slices.Contains(auth[:2], items[3]) {
		t.Fatalf("auth top two: %q", auth[:2])
	}
	if got := Rank(items, "top cl", id); !slices.Equal(got, items[:1]) {
		t.Fatalf("top cl: %q", got)
	}
	if got := Rank(items, "", id); !slices.Equal(got, items) {
		t.Fatalf("empty: %q", got)
	}
}
