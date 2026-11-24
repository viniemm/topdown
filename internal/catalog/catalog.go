// Package catalog merges conversations across agents, newest first, a page at a time.
package catalog

import (
	"sort"
	"sync"

	"github.com/viniemm/topdown/internal/agent"
)

type Page struct {
	Sessions []agent.Session
	// More: older conversations exist beyond the limit.
	More bool
}

// Recent lists up to limit conversations of the given agents. Stores are read
// concurrently; a store that fails is skipped.
func Recent(agentIDs []string, limit int) Page {
	var (
		mu  sync.Mutex
		all []agent.Session
		wg  sync.WaitGroup
	)
	for _, id := range agentIDs {
		a := agent.Get(id)
		if a == nil || a.Sessions == nil {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { recover() }() // a store changing shape must never break the picker
			got := a.Sessions.List(limit + 1)
			mu.Lock()
			all = append(all, got...)
			mu.Unlock()
		}()
	}
	wg.Wait()
	agent.SaveCache()
	sort.SliceStable(all, func(i, j int) bool { return all[i].When > all[j].When })
	if len(all) > limit {
		return Page{Sessions: all[:limit], More: true}
	}
	return Page{Sessions: all}
}

// Get looks one conversation up by agent and id.
func Get(agentID, id string) (agent.Session, bool) {
	if a := agent.Get(agentID); a != nil && a.Sessions != nil {
		return a.Sessions.Get(id)
	}
	return agent.Session{}, false
}
