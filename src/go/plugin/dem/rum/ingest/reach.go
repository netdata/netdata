// SPDX-License-Identifier: GPL-3.0-or-later

package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

var log = logger.New()

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

// reachState holds what the collector learned about how browsers reach
// it: the address seen through a trusted proxy, and the latest probe.
type reachState struct {
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

func (r *reachState) rejected(origin string, at time.Time) {
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
func (s *Route) LastRejectedOrigin() (RejectedOrigin, bool) {
	s.reach.mu.Lock()
	defer s.reach.mu.Unlock()
	return s.reach.rejectedOrigin, s.reach.hasRejectedOrigin
}

// Snippet reports the latest check of this site's home page.
func (s *Route) Snippet() (SnippetCheck, bool) {
	s.reach.mu.Lock()
	defer s.reach.mu.Unlock()
	return s.reach.snippet, s.reach.hasSnippet
}

func (s *Route) recordSnippet(c SnippetCheck) {
	s.reach.mu.Lock()
	prev, seen := s.reach.snippet, s.reach.hasSnippet
	s.reach.snippet, s.reach.hasSnippet = c, true
	s.reach.mu.Unlock()
	if !seen || prev.State != c.State {
		log.Noticef("rum: site %s snippet check: %s %s", s.config.Name, c.State, c.Detail)
	}
}

func (r *reachState) observe(base string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.observed = base
}

// ObservedBase is the public address last seen through a trusted proxy.
func (s *Route) ObservedBase() string {
	s.reach.mu.Lock()
	defer s.reach.mu.Unlock()
	return s.reach.observed
}

// Reachability returns this site's latest probe, if one has completed.
func (s *Route) Reachability() (Reach, bool) {
	s.reach.mu.Lock()
	defer s.reach.mu.Unlock()
	return s.reach.result, s.reach.hasResult
}

// Probe fetches this site's bootstrap and records whether this receiver answers.
func (s *Route) Probe(ctx context.Context, client *http.Client, base string) Reach {
	r := Reach{
		State:     ReachOK,
		Base:      base,
		CheckedAt: time.Now(),
	}
	if err := fetchOwnBootstrap(ctx, client, s.config.Name, base); err != nil {
		r.State, r.Error = ReachFailed, err.Error()
	}
	s.reach.mu.Lock()
	prev, seen := s.reach.result, s.reach.hasResult
	s.reach.result, s.reach.hasResult = r, true
	s.reach.mu.Unlock()
	if !seen || prev.State != r.State || prev.Base != r.Base {
		if r.State == ReachOK {
			log.Noticef("rum: site %s public URL %s reachable", s.config.Name, base)
		} else {
			log.Warningf("rum: site %s public URL %s unreachable: %s", s.config.Name, base, r.Error)
		}
	}
	return r
}

func fetchOwnBootstrap(ctx context.Context, client *http.Client, key, base string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/rum/"+key+".js", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return err
	}
	quotedKey, _ := json.Marshal(key)
	if !strings.Contains(string(body), "faro-web-sdk") || !strings.Contains(string(body), string(quotedKey)) {
		return fmt.Errorf("answered, but not by this collector")
	}
	return nil
}

// PublicBase is the base URL for site's snippet: public_url when set,
// else the address learned through a trusted proxy once a probe confirmed
// it reaches this collector, else fallback (the listener).
func (s *Route) PublicBase(fallback string) string {
	if s.config.PublicURL != "" {
		return strings.TrimRight(s.config.PublicURL, "/")
	}
	s.reach.mu.Lock()
	defer s.reach.mu.Unlock()
	if s.reach.hasResult && s.reach.observed != "" && s.reach.result.State == ReachOK &&
		s.reach.result.Base == s.reach.observed {
		return s.reach.observed
	}
	return fallback
}

// RunReachability probes this site immediately and then every interval.
// Without a configured or learned public address, reachability stays unknown.
func (s *Route) RunReachability(ctx context.Context, every time.Duration, base func() string, client *http.Client) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		publicBase := strings.TrimRight(base(), "/")
		if publicBase == "" {
			publicBase = s.ObservedBase()
		}
		if publicBase != "" {
			s.Probe(ctx, client, publicBase)
			s.recordSnippet(CheckSnippet(ctx, client, s.config, publicBase))
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
