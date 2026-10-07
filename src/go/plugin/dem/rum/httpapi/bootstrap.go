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
	w.Header().Set("Cache-Control", "no-store")
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
	// The snippet carries the site's settings, so it is revalidated on every
	// page load (ETag, usually a bodyless 304) instead of cached.
	// Shared caches must not retain site policy; browsers revalidate it.
	js := faro.Bootstrap(key, faro.BootstrapOptions{
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
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = io.WriteString(w, js)
}
