package observation

import (
	"sort"
	"sync"
	"time"
)

const LiveLimit = 8
const DemotionDelay = 30 * time.Second

type Candidate struct {
	RunID                                             string
	PendingInteraction, Recovery, ActiveTurn, Visible bool
	Recent                                            time.Time
}

func (c Candidate) priority() int {
	if c.PendingInteraction || c.Recovery || c.ActiveTurn {
		return 2
	}
	if c.Visible {
		return 1
	}
	return 0
}

// Window chooses logical subscriptions over a shared provider. Safety priority
// preempts immediately; ordinary replacement waits 30 seconds after admission.
type Window struct {
	mu   sync.Mutex
	live map[string]time.Time
}

func (w *Window) Select(now time.Time, candidates []Candidate) map[string]string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.live == nil {
		w.live = make(map[string]time.Time)
	}
	unique := make(map[string]Candidate)
	for _, c := range candidates {
		if c.RunID != "" {
			unique[c.RunID] = c
		}
	}
	ordered := make([]Candidate, 0, len(unique))
	for _, c := range unique {
		ordered = append(ordered, c)
	}
	sort.Slice(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if a.priority() != b.priority() {
			return a.priority() > b.priority()
		}
		if !a.Recent.Equal(b.Recent) {
			return a.Recent.After(b.Recent)
		}
		return a.RunID < b.RunID
	})
	selected := make(map[string]time.Time)
	// Admit safety candidates first, then retain ordinary streams during hysteresis.
	for _, c := range ordered {
		if c.priority() == 2 && len(selected) < LiveLimit {
			selected[c.RunID] = now
		}
	}
	for _, c := range ordered {
		since, live := w.live[c.RunID]
		if live && now.Sub(since) < DemotionDelay && len(selected) < LiveLimit {
			selected[c.RunID] = since
		}
	}
	for _, c := range ordered {
		if _, ok := selected[c.RunID]; !ok && len(selected) < LiveLimit {
			selected[c.RunID] = now
		}
	}
	modes := make(map[string]string, len(unique))
	for id := range unique {
		modes[id] = "polling"
		if _, ok := selected[id]; ok {
			modes[id] = "live"
			if since, old := w.live[id]; old {
				selected[id] = since
			}
		}
	}
	w.live = selected
	return modes
}
