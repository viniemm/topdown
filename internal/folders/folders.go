// Package folders suggests where to start a new agent: zoxide's frecent
// directories first (when it's installed), then the folders the agent's
// conversations live in, and plain path completion for typed paths.
package folders

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/viniemm/topdown/internal/fuzzy"
	"github.com/viniemm/topdown/internal/util"
)

// Folder is one suggestion.
type Folder struct {
	Path  string
	Rank  float64 // zoxide's score (0: not in zoxide)
	Convs int     // conversations the agent has there
}

// Candidates are the suggestions for an agent, best first. convs counts the
// agent's conversations by folder.
func Candidates(convs map[string]int) []Folder {
	byPath := map[string]*Folder{}
	var out []*Folder
	get := func(p string) *Folder {
		if f, ok := byPath[p]; ok {
			return f
		}
		f := &Folder{Path: p}
		byPath[p] = f
		out = append(out, f)
		return f
	}
	for _, z := range zoxide() {
		get(z.Path).Rank = z.Rank
	}
	for p, n := range convs {
		if p != "" && isDir(p) {
			get(p).Convs = n
		}
	}
	home := util.Home()
	res := make([]Folder, 0, len(out))
	for _, f := range out {
		if f.Path != home && f.Path != "/" {
			res = append(res, *f)
		}
	}
	// zoxide's rank leads; conversations count as a few visits each.
	weight := func(f Folder) float64 { return f.Rank + 2*float64(f.Convs) }
	sort.SliceStable(res, func(i, j int) bool { return weight(res[i]) > weight(res[j]) })
	return res
}

func zoxide() []Folder {
	if util.Which("zoxide") == "" {
		return nil
	}
	out, err := exec.Command("zoxide", "query", "--list", "--score").Output()
	if err != nil {
		return nil
	}
	var res []Folder
	for _, line := range strings.Split(string(out), "\n") {
		score, path, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		r, err := strconv.ParseFloat(score, 64)
		if err != nil {
			continue
		}
		res = append(res, Folder{Path: strings.TrimSpace(path), Rank: r})
	}
	return res
}

// Added tells zoxide about a folder an agent was started in, so it ranks in `z` too.
func Added(path string) {
	if util.Which("zoxide") != "" {
		exec.Command("zoxide", "add", "--", path).Run()
	}
}

// IsPath: the query is a path to complete (/, ~ or .), not words to match.
func IsPath(q string) bool {
	return strings.HasPrefix(q, "/") || strings.HasPrefix(q, "~") || strings.HasPrefix(q, ".")
}

// Expand resolves ~ and relative paths (against base).
func Expand(q, base string) string {
	switch {
	case q == "~":
		return util.Home()
	case strings.HasPrefix(q, "~/"):
		return filepath.Join(util.Home(), q[2:])
	case !filepath.IsAbs(q):
		return filepath.Join(base, q)
	}
	return filepath.Clean(q)
}

// Match narrows the candidates to a query: fuzzy over the folders, or, for a
// typed path, the directories it could complete to.
func Match(cands []Folder, q, base string) []Folder {
	q = strings.TrimSpace(q)
	if !IsPath(q) {
		return fuzzy.Rank(cands, q, func(f Folder) string { return util.ShortPath(f.Path) })
	}
	full := Expand(q, base)
	dir, prefix := full, ""
	if !strings.HasSuffix(q, "/") {
		dir, prefix = filepath.Dir(full), filepath.Base(full)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	known := map[string]Folder{}
	for _, c := range cands {
		known[c.Path] = c
	}
	var res []Folder
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() || !strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) ||
			(strings.HasPrefix(name, ".") && !strings.HasPrefix(prefix, ".")) {
			continue
		}
		p := filepath.Join(dir, name)
		f, ok := known[p]
		if !ok {
			f = Folder{Path: p}
		}
		res = append(res, f)
	}
	sort.SliceStable(res, func(i, j int) bool { return res[i].Rank+float64(res[i].Convs) > res[j].Rank+float64(res[j].Convs) })
	return res
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// Exists reports whether p is a directory.
func Exists(p string) bool { return isDir(p) }
