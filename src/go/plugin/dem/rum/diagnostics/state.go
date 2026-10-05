// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
)

// Reachability states for a site's public snippet URL.
const (
	ReachUnknown = "unknown" // no public URL configured or learned yet
	ReachOK      = "reachable"
	ReachFailed  = "unreachable"
)

// Reach is the latest self-check of one site's public snippet URL.
type Reach struct {
	State     string
	Base      string
	Error     string
	CheckedAt time.Time
}

// State holds what the collector learned about how browsers reach
// it: the address seen through a trusted proxy, and the latest probe.
type State struct {
	config            config.Site
	mu                sync.Mutex
	observed          string
	result            Reach
	hasResult         bool
	snippet           SnippetCheck
	hasSnippet        bool
	rejectedOrigin    RejectedOrigin
	hasRejectedOrigin bool
}

// RejectedOrigin is the last origin whose beacons a site refused.
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

// Snippet reports the latest check of this site's home page.
func (s *State) Snippet() (SnippetCheck, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snippet, s.hasSnippet
}

func (s *State) recordSnippet(c SnippetCheck) {
	s.mu.Lock()
	prev, seen := s.snippet, s.hasSnippet
	s.snippet, s.hasSnippet = c, true
	s.mu.Unlock()
	if !seen || prev.State != c.State {
		log.Noticef("rum: site %s snippet check: %s %s", s.config.Name, c.State, c.Detail)
	}
}

func (r *State) ObserveBase(base string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.observed = base
}

// ObservedBase is the public address last seen through a trusted proxy.
func (s *State) ObservedBase() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.observed
}

// Reachability returns this site's latest probe, if one has completed.
func (s *State) Reachability() (Reach, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.result, s.hasResult
}

// New creates the diagnostics owned by one admitted site generation.
func New(cfg config.Site) *State {
	return &State{
		config: cfg,
	}
}
