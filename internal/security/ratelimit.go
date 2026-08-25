package security

import (
	"sync"
	"time"
)

type Limiter struct {
	mu     sync.Mutex
	events map[string][]time.Time
	limit  int
	window time.Duration
	lastGC time.Time
}

func NewLimiter(limit int, window time.Duration) *Limiter {
	return &Limiter{events: make(map[string][]time.Time), limit: limit, window: window, lastGC: time.Now()}
}

func (l *Limiter) Allow(key string) bool {
	now := time.Now()
	cutoff := now.Add(-l.window)
	l.mu.Lock()
	defer l.mu.Unlock()

	if now.Sub(l.lastGC) > 5*l.window {
		for k, ts := range l.events {
			kept := ts[:0]
			for _, t := range ts {
				if t.After(cutoff) {
					kept = append(kept, t)
				}
			}
			if len(kept) == 0 {
				delete(l.events, k)
			} else {
				l.events[k] = kept
			}
		}
		l.lastGC = now
	}

	ts := l.events[key]
	kept := ts[:0]
	for _, t := range ts {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.limit {
		l.events[key] = kept
		return false
	}
	l.events[key] = append(kept, now)
	return true
}
