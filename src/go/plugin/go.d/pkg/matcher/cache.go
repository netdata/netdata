// SPDX-License-Identifier: GPL-3.0-or-later

package matcher

import "sync"

type (
	cachedMatcher struct {
		matcher Matcher

		mux   sync.RWMutex
		cache map[string]bool
	}
)

// WithCache adds cache to the matcher.
func WithCache(m Matcher) Matcher {
	switch m {
	case TRUE(), FALSE():
		return m
	default:
		return &cachedMatcher{matcher: m, cache: make(map[string]bool)}
	}
}

func (m *cachedMatcher) Match(b []byte) bool {
	s := string(b)
	if result, ok := m.fetch(s); ok {
		return result
	}
	result := m.matcher.Match(b)
	m.put(s, result)
	return result
}

func (m *cachedMatcher) MatchString(s string) bool {
	if result, ok := m.fetch(s); ok {
		return result
	}
	result := m.matcher.MatchString(s)
	m.put(s, result)
	return result
}

func (m *cachedMatcher) fetch(key string) (result bool, ok bool) {
	m.mux.RLock()
	result, ok = m.cache[key]
	m.mux.RUnlock()
	return
}

func (m *cachedMatcher) put(key string, result bool) {
	m.mux.Lock()
	m.cache[key] = result
	m.mux.Unlock()
}
