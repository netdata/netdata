// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
)

// CountryResolver maps a client IP to an ISO country code plus city-level
// location in one lookup. city/lat/lon are zero when hasGeo is false.
type CountryResolver interface {
	Lookup(ip string) (country, city string, lat, lon float64, hasGeo bool)
}

// Routes supplies one exact runtime registration for the whole request.
// Its context is cancelled when the site retires; release joins that admission.
type Routes interface {
	Acquire(key string) (*Route, context.Context, func(), bool)
}

type Server struct {
	snap     *snapshot
	routes   Routes
	geo      CountryResolver
	now      func() time.Time
	ips      *limiter
	siteRate float64
}

func New(cfg *config.Receiver, routes Routes, geo CountryResolver) *Server {
	return &Server{
		snap:     compile(cfg),
		routes:   routes,
		geo:      geo,
		now:      time.Now,
		siteRate: float64(cfg.RateLimit.PerSitePerSec),
		ips:      newLimiter(float64(cfg.RateLimit.PerIPPerMin)/60, float64(cfg.RateLimit.PerIPPerMin), 10000),
	}
}

// acquire fences policy, processor and diagnostics to one site runtime. Closing an
// admitted body on retirement also interrupts a slow producer inside ReadAll.
func (s *Server) acquire(w http.ResponseWriter, r *http.Request, key string) (*Route, func(), bool) {
	route, ctx, release, ok := s.routes.Acquire(key)
	if !ok {
		http.NotFound(w, r)
		return nil, nil, false
	}
	interrupted := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(interrupted)
		// Closing net/http's body waits for its current read. Interrupt the
		// socket read first so retirement can join a stalled upload.
		_ = http.NewResponseController(w).SetReadDeadline(time.Now())
		_ = r.Body.Close()
	})
	return route, func() {
		if !stop() {
			<-interrupted
		}
		release()
	}, true
}

// Handler returns the routed handler (also used by tests).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rum/demo", s.demo)
	mux.HandleFunc("GET /rum/ping", ping)
	mux.HandleFunc("GET /rum/{file}", s.bootstrap)
	mux.HandleFunc("POST /rum/{key}/collect", s.collect)
	mux.HandleFunc("OPTIONS /rum/{key}/collect", s.preflight)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	return mux
}
