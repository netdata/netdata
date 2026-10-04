// SPDX-License-Identifier: GPL-3.0-or-later
package ingest

import (
	"context"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
)

type fixtureConfig struct {
	config.Receiver
	Sites              []config.Site
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
		Server:  New(&cfg.Receiver, routes, geo),
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
		s.fixture.sites[sc.Name] = NewRoute(sc, s.sink)
	}
}
func (s *fixtureServer) site(k string) *site {
	r, _, _, _ := s.fixture.Acquire(k)
	if r == nil {
		return nil
	}
	return r.policy
}
func (s *fixtureServer) route(k string) *Route { r, _, _, _ := s.fixture.Acquire(k); return r }
