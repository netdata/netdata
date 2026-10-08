// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"sync"
	"time"
)

// State holds the bounded last rejected-origin observation for one admitted site generation.
type State struct {
	mu                sync.Mutex
	rejectedOrigin    RejectedOrigin
	hasRejectedOrigin bool
}

// RejectedOrigin is the last origin refused by a site, including an OPTIONS preflight.
type RejectedOrigin struct {
	Origin string
	At     time.Time
}

func (r *State) RejectOrigin(origin string, at time.Time) {
	if origin == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rejectedOrigin = RejectedOrigin{
		Origin: origin,
		At:     at,
	}
	r.hasRejectedOrigin = true
}

// LastRejectedOrigin is the last origin refused by this site, if any.
func (s *State) LastRejectedOrigin() (RejectedOrigin, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rejectedOrigin, s.hasRejectedOrigin
}

// New creates the diagnostics owned by one admitted site generation.
func New() *State { return &State{} }
