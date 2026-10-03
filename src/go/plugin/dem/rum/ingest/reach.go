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
	"github.com/netdata/netdata/go/plugins/plugin/dem/config"
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
	mu       sync.Mutex
	observed map[string]string
	results  map[string]Reach
	snippets map[string]SnippetCheck
	rejects  map[string]RejectedOrigin
}

// RejectedOrigin is the last origin whose beacons a site refused.
type RejectedOrigin struct {
	Origin string
	At     time.Time
}

func (r *reachState) rejected(key, origin string, at time.Time) {
	if origin == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rejects == nil {
		r.rejects = map[string]RejectedOrigin{}
	}
	r.rejects[key] = RejectedOrigin{
		Origin: origin,
		At:     at,
	}
}

// LastRejectedOrigin is the last origin refused for site key, if any.
func (s *Route) LastRejectedOrigin(key string) (RejectedOrigin, bool) {
	s.reach.mu.Lock()
	defer s.reach.mu.Unlock()
	r, ok := s.reach.rejects[key]
	return r, ok
}

// Snippet reports the latest check of site key's home page.
func (s *Route) Snippet(key string) (SnippetCheck, bool) {
	s.reach.mu.Lock()
	defer s.reach.mu.Unlock()
	c, ok := s.reach.snippets[key]
	return c, ok
}

func (s *Route) recordSnippet(key string, c SnippetCheck) {
	s.reach.mu.Lock()
	if s.reach.snippets == nil {
		s.reach.snippets = map[string]SnippetCheck{}
	}
	prev, seen := s.reach.snippets[key]
	s.reach.snippets[key] = c
	s.reach.mu.Unlock()
	if !seen || prev.State != c.State {
		log.Noticef("rum: site %s snippet check: %s %s", key, c.State, c.Detail)
	}
}

func (r *reachState) observe(key, base string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.observed == nil {
		r.observed = map[string]string{}
	}
	r.observed[key] = base
}

// ObservedBase is the public address of site key as last seen on a
// request through a trusted proxy ("" when none).
func (s *Route) ObservedBase(key string) string {
	s.reach.mu.Lock()
	defer s.reach.mu.Unlock()
	return s.reach.observed[key]
}

// Reachability returns the latest probe of site key.
func (s *Route) Reachability(key string) (Reach, bool) {
	s.reach.mu.Lock()
	defer s.reach.mu.Unlock()
	r, ok := s.reach.results[key]
	return r, ok
}

// Probe fetches base/rum/<key>.js the way a visitor's browser would and
// records whether this collector answered it.
func (s *Route) Probe(ctx context.Context, client *http.Client, key, base string) Reach {
	r := Reach{
		State:     ReachOK,
		Base:      base,
		CheckedAt: time.Now(),
	}
	if err := fetchOwnBootstrap(ctx, client, key, base); err != nil {
		r.State, r.Error = ReachFailed, err.Error()
	}
	s.reach.mu.Lock()
	if s.reach.results == nil {
		s.reach.results = map[string]Reach{}
	}
	prev, seen := s.reach.results[key]
	s.reach.results[key] = r
	s.reach.mu.Unlock()
	if !seen || prev.State != r.State || prev.Base != r.Base {
		if r.State == ReachOK {
			log.Noticef("rum: site %s public URL %s reachable", key, base)
		} else {
			log.Warningf("rum: site %s public URL %s unreachable: %s", key, base, r.Error)
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
func (s *Route) PublicBase(site config.RumSite, fallback string) string {
	if site.PublicURL != "" {
		return strings.TrimRight(site.PublicURL, "/")
	}
	observed := s.ObservedBase(site.Key)
	if r, ok := s.Reachability(site.Key); ok && observed != "" && r.State == ReachOK && r.Base == observed {
		return observed
	}
	return fallback
}

// RunReachability probes every site now and then every interval, until
// ctx ends. Sites with neither public_url nor a learned address stay
// unknown.
func (s *Route) RunReachability(ctx context.Context, every time.Duration, base func() string, client *http.Client) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		publicBase := strings.TrimRight(base(), "/")
		if publicBase == "" {
			publicBase = s.ObservedBase(s.config.Key)
		}
		if publicBase != "" {
			s.Probe(ctx, client, s.config.Key, publicBase)
			s.recordSnippet(s.config.Key, CheckSnippet(ctx, client, s.config, publicBase))
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
