// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"errors"
	"io"
	"net/http"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/faro"
)

// cors applies the fail-closed origin policy: the exact matched request
// origin is echoed (never "*") with Vary: Origin. ok=false means 403.
func (s *Server) cors(w http.ResponseWriter, r *http.Request, st *site) bool {
	origin, ok := st.matchOrigin(requestOrigin(r))
	if !ok {
		return false
	}
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", origin)
	h.Add("Vary", "Origin")
	h.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	reqHdr := r.Header.Get("Access-Control-Request-Headers")
	if reqHdr == "" || len(reqHdr) > 256 {
		reqHdr = "Content-Type"
	}
	h.Set("Access-Control-Allow-Headers", reqHdr)
	h.Set("Access-Control-Max-Age", "86400")
	return true
}

func (s *Server) preflight(w http.ResponseWriter, r *http.Request) {
	route, release, ok := s.acquire(w, r, r.PathValue("key"))
	if !ok {
		return
	}
	defer release()
	st := route.policy
	if !s.cors(w, r, st) {
		s.rejectOrigin(route, r)
		w.WriteHeader(http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ping answers the Connect panel's reachability check, run from the
// viewer's browser on another origin; it reveals nothing about the sites.
func ping(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) rejectOrigin(route *Route, r *http.Request) {
	route.processor.Reject(beacon.RejectOrigin)
	route.rejectOrigin(r.Header.Get("Origin"), s.now())
}

func (s *Server) collect(w http.ResponseWriter, r *http.Request) {
	route, release, ok := s.acquire(w, r, r.PathValue("key"))
	if !ok {
		return
	}
	defer release()
	st := route.policy
	cfg := &route.config
	snap := s.snap
	w.Header().Set("Cache-Control", "no-store")
	if !s.cors(w, r, st) {
		s.rejectOrigin(route, r)
		w.WriteHeader(http.StatusForbidden)
		return
	}
	now := s.now()
	ip := snap.clientIP(r)
	ipKey := ip.String()
	if !s.ips.allow(ipKey, now) || !route.allowSite(s.siteRate, now) {
		route.processor.Reject(beacon.RejectRate)
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	if r.ContentLength > snap.maxBody {
		route.processor.Reject(beacon.RejectSize)
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}
	bot := r.URL.Query().Get("bot") == "1" || faro.IsBot(r.Header.Get("User-Agent"))
	if bot && !cfg.IncludesBots() {
		route.processor.Reject(beacon.RejectBot)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// MaxBytesReader needs net/http's concrete writer to signal HTTP/1 close
	// on oversized bodies; it does not follow middleware Unwrap methods.
	bodyWriter := w
	for {
		unwrapper, ok := bodyWriter.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			break
		}
		bodyWriter = unwrapper.Unwrap()
	}
	body, err := io.ReadAll(http.MaxBytesReader(bodyWriter, r.Body, snap.maxBody))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			route.processor.Reject(beacon.RejectSize)
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		route.processor.Reject(beacon.RejectInvalid)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// Preserve admission and body-size checks for stale pages and custom
	// producers, but intentional exclusion is neither acceptance nor loss.
	if cfg.MeasureRate() == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var location beacon.Location
	mode := cfg.GeolocationMode()
	if mode != config.GeolocationOff && s.geo != nil && ip.IsValid() && !ip.IsLoopback() && !ip.IsPrivate() {
		location = s.geo.Lookup(ipKey)
	}
	if mode != config.GeolocationCity {
		location = beacon.Location{
			Country: location.Country,
		}
	}
	b, err := faro.Decode(body, faro.Options{
		Site:               cfg.Name,
		Now:                now,
		Location:           location,
		EventLogs:          cfg.EventLogsOn(),
		FrustrationSignals: cfg.FrustrationSignalsOn(),
		Redactor:           route.redactor,
		ConsoleLogs:        cfg.ConsoleLogsOn(),
		Tracing:            cfg.TracingOn(),
		PathRules:          st.pathRules,
	})
	if err != nil {
		route.processor.Reject(beacon.RejectInvalid)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if bot {
		b.Device = beacon.DeviceBot
	}
	route.processor.Ingest(b)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = io.WriteString(w, `{"accepted":true}`)
}
