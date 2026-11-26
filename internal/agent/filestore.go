package agent

// Sessions kept one file per conversation (Claude, Pi, omp): cheap discovery by
// mtime, parse only what is shown, cache parses by path + mtime + size.

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/viniemm/topdown/internal/util"
)

// Parsed is what a parser extracts from one session file.
type Parsed struct {
	Title   string   `json:"title"`
	Cwd     string   `json:"cwd"`
	Prompts []string `json:"prompts"`
	Context int      `json:"context,omitempty"`
}

const cacheVersion = 5 // separate from the TypeScript build's cache format

type cacheEntry struct {
	Parsed
	Mtime int64 `json:"mtime"`
	Size  int64 `json:"size"`
}

type parseCache struct {
	mu    sync.Mutex
	data  map[string]cacheEntry
	dirty bool
}

var (
	cacheOnce sync.Once
	cache     *parseCache
)

func cacheFile() string { return filepath.Join(util.CacheDir(), "sessions.json") }

func sharedCache() *parseCache {
	cacheOnce.Do(func() {
		cache = &parseCache{data: map[string]cacheEntry{}}
		var raw struct {
			Version  int                   `json:"version"`
			Sessions map[string]cacheEntry `json:"sessions"`
		}
		if b, err := os.ReadFile(cacheFile()); err == nil && json.Unmarshal(b, &raw) == nil && raw.Version == cacheVersion && raw.Sessions != nil {
			cache.data = raw.Sessions
		}
	})
	return cache
}

// ResetCache drops the in-memory cache (tests switch $HOME between cases).
func ResetCache() { cacheOnce = sync.Once{} }

func (c *parseCache) lookup(file string, st os.FileInfo, parse func(string) Parsed) cacheEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if hit, ok := c.data[file]; ok && hit.Mtime == st.ModTime().UnixNano() && hit.Size == st.Size() {
		return hit
	}
	e := cacheEntry{Parsed: parse(file), Mtime: st.ModTime().UnixNano(), Size: st.Size()}
	c.data[file] = e
	c.dirty = true
	return e
}

// SaveCache writes the parse cache if anything changed.
func SaveCache() {
	c := sharedCache()
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.dirty {
		return
	}
	b, err := json.Marshal(map[string]any{"version": cacheVersion, "sessions": c.data})
	if err == nil && util.WriteFileAtomic(cacheFile(), b, 0o644) == nil {
		c.dirty = false
	}
}

// fileStore describes one agent's session files.
type fileStore struct {
	agent string
	// pattern is a glob under $HOME.
	pattern string
	parse   func(file string) Parsed
	// byPath: the session id is the full path (else the file stem).
	byPath bool
}

func (f fileStore) files() []string {
	m, _ := filepath.Glob(filepath.Join(util.Home(), f.pattern))
	return m
}

func (f fileStore) session(file string, st os.FileInfo) (Session, bool) {
	if st == nil {
		var err error
		if st, err = os.Stat(file); err != nil {
			return Session{}, false
		}
	}
	info := sharedCache().lookup(file, st, f.parse)
	if info.Title == "" {
		return Session{}, false
	}
	id := file
	if !f.byPath {
		id = strings.TrimSuffix(filepath.Base(file), ".jsonl")
	}
	return Session{Agent: f.agent, ID: id, Cwd: info.Cwd, Title: info.Title,
		When: float64(st.ModTime().UnixMilli()) / 1000, Prompts: info.Prompts, Path: file, Context: info.Context}, true
}

// List returns up to limit sessions, newest first; only shown files are parsed.
func (f fileStore) List(limit int) []Session {
	type cand struct {
		file string
		st   os.FileInfo
	}
	var cands []cand
	for _, file := range f.files() {
		if st, err := os.Stat(file); err == nil {
			cands = append(cands, cand{file, st})
		}
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].st.ModTime().After(cands[j].st.ModTime()) })
	var out []Session
	for _, c := range cands {
		if len(out) >= limit {
			break
		}
		if s, ok := f.session(c.file, c.st); ok {
			out = append(out, s)
		}
	}
	return out
}

func (f fileStore) fileFor(id string) string {
	if f.byPath {
		if _, err := os.Stat(id); err == nil {
			return id
		}
		return ""
	}
	m, _ := filepath.Glob(filepath.Join(util.Home(), filepath.Dir(f.pattern), escapeGlob(id)+".jsonl"))
	if len(m) == 0 {
		return ""
	}
	return m[0]
}

func (f fileStore) Get(id string) (Session, bool) {
	if file := f.fileFor(id); file != "" {
		return f.session(file, nil)
	}
	return Session{}, false
}

// fileOf is a session's file: the one it was listed with, else found by id ("" when gone).
func (f fileStore) fileOf(s Session) string {
	if s.Path != "" {
		return s.Path
	}
	return f.fileFor(s.ID)
}

// first is the first of ss, or "" (a title's last fallback: the first prompt).
func first(ss []string) string {
	if len(ss) > 0 {
		return ss[0]
	}
	return ""
}

func escapeGlob(s string) string {
	r := strings.NewReplacer(`\`, `\\`, "*", `\*`, "?", `\?`, "[", `\[`)
	return r.Replace(s)
}

// eachLine calls fn with each line of a JSONL file; fn returns false to stop.
func eachLine(file string, fn func(line string, n int) bool) {
	fh, err := os.Open(file)
	if err != nil {
		return
	}
	defer fh.Close()
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20) // transcripts can hold very long lines
	for n := 0; sc.Scan(); n++ {
		if !fn(sc.Text(), n) {
			return
		}
	}
}

// usageAt decodes the JSON object that follows key in line (e.g. `"usage":`)
// into v, without decoding the rest of a possibly very long line.
func usageAt(line, key string, v any) bool {
	i := strings.LastIndex(line, key)
	if i == -1 {
		return false
	}
	return json.NewDecoder(strings.NewReader(line[i+len(key):])).Decode(v) == nil
}

// appendJSONL appends one compact JSON entry, keys in the given order.
func appendJSONL(file string, entry []kv) error {
	var b strings.Builder
	b.WriteByte('{')
	for i, e := range entry {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(e.k)
		v, _ := json.Marshal(e.v)
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteString("}\n")
	fh, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer fh.Close()
	_, err = fh.WriteString(b.String())
	return err
}

type kv struct {
	k string
	v any
}
