package as400

import (
	"sync"
	"time"
)

type latencyCache struct {
	mu     sync.RWMutex
	values map[string]int64
	last   time.Time
}

func (l *latencyCache) beginCycle(ts time.Time) {
	l.mu.Lock()
	if l.values == nil {
		l.values = make(map[string]int64)
	}
	l.last = ts
	l.mu.Unlock()
}

func (l *latencyCache) add(name string, value int64) {
	if value == 0 {
		return
	}
	l.mu.Lock()
	if l.values == nil {
		l.values = make(map[string]int64)
	}
	l.values[name] += value
	l.mu.Unlock()
}

func (l *latencyCache) snapshot() (map[string]int64, time.Time) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if len(l.values) == 0 {
		return nil, l.last
	}
	out := make(map[string]int64, len(l.values))
	for k, v := range l.values {
		out[k] = v
	}
	return out, l.last
}
