// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"context"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/diagnostics"
)

type fixtureConfig struct {
	config.Receiver
	Sites []config.Site
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
	sink    Processor
}

func newFixture(cfg *fixtureConfig, sink Processor, geo CountryResolver) *fixtureServer {
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
		s.fixture.sites[sc.Name] = NewRoute(sc, s.sink, diagnostics.New(sc))
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
