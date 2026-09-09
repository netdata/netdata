// SPDX-License-Identifier: GPL-3.0-or-later

package pinger

import (
	"sync"
	"time"
)

type hostState struct {
	ewma float64
	sma  []float64
}

type stateStore struct {
	mu     sync.Mutex
	byHost map[string]*hostState
}

func newStateStore() *stateStore {
	return &stateStore{
		byHost: make(map[string]*hostState),
	}
}

func (s *stateStore) update(host string, current time.Duration, cfg AnalysisConfig) (time.Duration, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.byHost[host]
	if !ok {
		st = &hostState{}
		s.byHost[host] = st
	}

	curr := float64(current)
	alpha := 1.0 / float64(cfg.JitterEWMASamples)
	st.ewma = alpha*curr + (1-alpha)*st.ewma

	st.sma = append(st.sma, curr)
	if len(st.sma) > cfg.JitterSMAWindow {
		st.sma = st.sma[1:]
	}

	var sum float64
	for _, v := range st.sma {
		sum += v
	}

	return time.Duration(st.ewma), time.Duration(sum / float64(len(st.sma)))
}
