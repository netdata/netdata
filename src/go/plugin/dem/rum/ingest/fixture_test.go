// SPDX-License-Identifier: GPL-3.0-or-later
package ingest

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

type fixtureConfig struct {
	config.RumCfg
	Sites              []config.RumSite
	CollectConsoleLogs bool
}
type fixtureRoutes struct {
	mu    sync.Mutex
	sites map[string]*Route
}

func (r *fixtureRoutes) Acquire(k string) (*Route, context.Context, func(), bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	route, ok := r.sites[k]
	return route, context.Background(), func() {}, ok
}

type fixtureServer struct {
	*Server
	fixture *fixtureRoutes
	sink    beacon.Sink
}

func newFixture(cfg *fixtureConfig, sink beacon.Sink, geo CountryResolver) *fixtureServer {
	routes := &fixtureRoutes{}
	s := &fixtureServer{
		Server:  New(&cfg.RumCfg, routes, geo),
		fixture: routes,
		sink:    sink,
	}
	s.Update(cfg)
	return s
}
func (s *fixtureServer) Update(cfg *fixtureConfig) {
	s.fixture.mu.Lock()
	defer s.fixture.mu.Unlock()
	s.fixture.sites = map[string]*Route{}
	for _, sc := range cfg.Sites {
		sc.CollectConsoleLogs = cfg.CollectConsoleLogs
		s.fixture.sites[sc.Key] = NewRoute(sc, s.sink)
	}
}
func (s *fixtureServer) site(k string) *site {
	r, _, _, _ := s.fixture.Acquire(k)
	if r == nil {
		return nil
	}
	return r.policy
}
func (s *fixtureServer) route(k string) *Route        { r, _, _, _ := s.fixture.Acquire(k); return r }
func (s *fixtureServer) ObservedBase(k string) string { return s.route(k).ObservedBase(k) }
func (s *fixtureServer) Probe(ctx context.Context, c *http.Client, k, b string) Reach {
	return s.route(k).Probe(ctx, c, k, b)
}
func (s *fixtureServer) Reachability(k string) (Reach, bool) { return s.route(k).Reachability(k) }
func (s *fixtureServer) LastRejectedOrigin(k string) (RejectedOrigin, bool) {
	return s.route(k).LastRejectedOrigin(k)
}
func (s *fixtureServer) PublicBase(sc config.RumSite, b string) string {
	return s.route(sc.Key).PublicBase(sc, b)
}

func (s *fixtureServer) RunReachability(
	ctx context.Context,
	every time.Duration,
	sites func() []config.RumSite,
	c *http.Client,
) {
	for _, sc := range sites() {
		r := s.route(sc.Key)
		r.config = sc
		r.RunReachability(ctx, every, func() string { return sc.PublicURL }, c)
	}
}
