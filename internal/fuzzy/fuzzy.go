// Package fuzzy ranks items: every space-separated term must match as a
// subsequence (like fzf's extended mode); matches at word starts and in a row
// score higher, as do earlier ones; a plain substring beats any scattered
// match. Case-insensitive.
package fuzzy

import (
	"sort"
	"strings"
)

func boundary(r rune) bool { return strings.ContainsRune(" \t/_.-:", r) }

// score of term in text, or -1 when it doesn't match.
func score(term, text string) int {
	t := []rune(strings.ToLower(text))
	q := []rune(strings.ToLower(term))
	if len(q) == 0 {
		return 0
	}
	if at := strings.Index(string(t), string(q)); at != -1 {
		at = len([]rune(string(t)[:at]))
		s := 1000 - at
		if at == 0 || boundary(t[at-1]) {
			s += 200
		}
		return s
	}
	s, ti, prev := 0, 0, -2
	for _, ch := range q {
		found := -1
		for i := ti; i < len(t); i++ {
			if t[i] == ch {
				found = i
				break
			}
		}
		if found == -1 {
			return -1
		}
		s += 10
		if found == prev+1 {
			s += 15
		}
		if found == 0 || boundary(t[found-1]) {
			s += 20
		}
		s -= min(found-ti, 10)
		prev, ti = found, found+1
	}
	return s
}

// Rank returns the items matching every term of query, best first (ties keep their order).
func Rank[T any](items []T, query string, text func(T) string) []T {
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return append([]T(nil), items...)
	}
	type scored struct {
		item  T
		score int
	}
	var out []scored
outer:
	for _, it := range items {
		t := text(it)
		total := 0
		for _, term := range terms {
			s := score(term, t)
			if s < 0 {
				continue outer
			}
			total += s
		}
		out = append(out, scored{it, total})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].score > out[j].score })
	res := make([]T, len(out))
	for i, s := range out {
		res[i] = s.item
	}
	return res
}
