// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/faro"
)

// bootstrap serves GET /rum/<key>.js: the pinned Faro SDK loader pointed
// at this collector. It is a public asset (the key is already in the
// page's HTML), so no origin check applies here.
func (s *Server) bootstrap(w http.ResponseWriter, r *http.Request) {
	file := r.PathValue("file")
	key, ok := strings.CutSuffix(file, ".js")
	if !ok {
		http.NotFound(w, r)
		return
	}
	route, release, ok := s.acquire(w, r, key)
	if !ok {
		return
	}
	defer release()
	st := route.policy
	base := strings.TrimRight(route.config.PublicURL, "/")
	if base == "" {
		base = s.snap.publicURL
	}
	if base == "" {
		base = s.baseURL(r)
	}
	if remote := parseAddr(r.RemoteAddr); remote.IsValid() && s.snap.isTrusted(remote) {
		route.diagnostics.ObserveBase(base)
	}
	// The snippet carries the site's settings, so it is revalidated on every
	// page load (ETag, usually a bodyless 304) instead of cached.
	// "private" keeps CDNs out: Cloudflare rewrites a plain no-cache or a
	// short max-age to its own browser TTL (4 h by default).
	js := faro.Bootstrap(key, base, faro.BootstrapOptions{
		MeasureRate:        st.measureRate,
		IncludeBots:        st.includeBots,
		EventLogs:          route.config.EventLogsOn(),
		FrustrationSignals: route.config.FrustrationSignalsOn(),
		ConsoleLogs:        route.config.ConsoleLogsOn(),
		Tracing:            st.tracing,
		Propagate:          st.propagate,
	})
	sum := sha256.Sum256([]byte(js))
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = io.WriteString(w, js)
}

// baseURL is scheme+host as the browser sees this collector. Behind a
// trusted proxy X-Forwarded-Proto/Host are honored.
func (s *Server) baseURL(r *http.Request) string {
	snap := s.snap
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	if remote := parseAddr(r.RemoteAddr); remote.IsValid() && snap.isTrusted(remote) {
		if p := strings.ToLower(r.Header.Get("X-Forwarded-Proto")); p == "https" || p == "http" {
			scheme = p
		}
		if h := r.Header.Get("X-Forwarded-Host"); h != "" && len(h) < 256 && !strings.ContainsAny(h, " /\\\"'<>") {
			host = h
		}
	}
	return scheme + "://" + host
}
