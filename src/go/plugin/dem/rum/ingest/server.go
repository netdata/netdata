// SPDX-License-Identifier: GPL-3.0-or-later

// Package ingest is the RUM beacon collector that runs inside
// dem.plugin: a small HTTP(S) server that hands
// out the per-site Faro bootstrap, accepts Faro payloads under a strict
// origin allow-list and rate limits, normalizes them and forwards them
// to the aggregator / OTLP exporter through a beacon.Sink.
package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

// FaroVersion is the pinned Faro web SDK served by the bootstrap.
const FaroVersion = "2.11.0"

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

func New(cfg *config.RumCfg, routes Routes, geo CountryResolver) *Server {
	return &Server{
		snap:     compile(cfg),
		routes:   routes,
		geo:      geo,
		now:      time.Now,
		siteRate: float64(cfg.RateLimit.PerSitePerSec),
		ips:      newLimiter(float64(cfg.RateLimit.PerIPPerMin)/60, float64(cfg.RateLimit.PerIPPerMin), 10000),
	}
}

// acquire fences policy, sink and diagnostics to one site runtime. Closing an
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
	route.sink.Reject(route.policy.key, beacon.RejectOrigin)
	route.reach.rejected(route.policy.key, r.Header.Get("Origin"), s.now())
}

func (s *Server) collect(w http.ResponseWriter, r *http.Request) {
	route, release, ok := s.acquire(w, r, r.PathValue("key"))
	if !ok {
		return
	}
	defer release()
	st := route.policy
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
		route.sink.Reject(st.key, beacon.RejectRate)
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	if r.ContentLength > snap.maxBody {
		route.sink.Reject(st.key, beacon.RejectSize)
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}
	bot := r.URL.Query().Get("bot") == "1" || beacon.IsBot(r.Header.Get("User-Agent"))
	if bot && !st.includeBots {
		route.sink.Reject(st.key, beacon.RejectBot)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, snap.maxBody))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			route.sink.Reject(st.key, beacon.RejectSize)
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		route.sink.Reject(st.key, beacon.RejectInvalid)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var country, city string
	var lat, lon float64
	var hasGeo bool
	if s.geo != nil && ip.IsValid() && !ip.IsLoopback() && !ip.IsPrivate() {
		country, city, lat, lon, hasGeo = s.geo.Lookup(ipKey)
	}
	b, err := parseFaro(
		st.key,
		body,
		country,
		city,
		lat,
		lon,
		hasGeo,
		parseOpts{
			consoleLogs: route.config.CollectConsoleLogs,
			tracing:     st.tracing,
			pathRules:   st.pathRules,
		},
		now,
	)
	if err != nil {
		route.sink.Reject(st.key, beacon.RejectInvalid)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if bot {
		b.Device = beacon.DeviceBot
	}
	b.TraceExportTo = st.exportTo
	route.sink.Ingest(b)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = io.WriteString(w, `{"accepted":true}`)
}

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
		route.reach.observe(key, base)
	}
	// The snippet carries the site's settings, so it is revalidated on every
	// page load (ETag, usually a bodyless 304) instead of cached.
	// "private" keeps CDNs out: Cloudflare rewrites a plain no-cache or a
	// short max-age to its own browser TTL (4 h by default).
	js := bootstrapJS(key, base, st)
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

// bootstrapJS renders the loader; every value is JSON-encoded so a hostile
// Host header cannot break out of the script.
func bootstrapJS(key, base string, st *site) string {
	k, _ := json.Marshal(key)
	b, _ := json.Marshal(base)
	opt := map[string]any{
		"sampling":    st.measureRate,
		"bots":        beacon.BotPatternJS(),
		"includeBots": st.includeBots,
		"tracing":     nil,
	}
	if st.tracing {
		opt["tracing"] = append([]string{}, st.propagate...)
	}
	o, _ := json.Marshal(opt)
	return strings.NewReplacer("__KEY__", string(k), "__BASE__", string(b), "__OPT__", string(o)).Replace(faroBootstrap)
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

func (s *Server) demo(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		key = "demo"
	}
	_, release, ok := s.acquire(w, r, key)
	if !ok {
		return
	}
	defer release()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = rumDemoPage.Execute(w, map[string]string{"Key": key})
}

const faroBootstrap = `(function (k, base, opt) {
  // data-version/data-env on our OWN <script> tag become
  // Faro app.version/app.environment; currentScript must be read
  // synchronously, before anything yields, or it stops pointing here.
  var cs = document.currentScript;
  var ver = (cs && cs.getAttribute('data-version')) || '';
  var env = (cs && cs.getAttribute('data-env')) || '';
  // Crawlers, headless browsers and automation: send nothing unless
  // the site keeps bots, then flag them to the collector.
  // netdataRum.setUser({id}) links sessions to the site's own user id
  //; only the id is passed on, and it works before Faro has loaded.
  var user = null;
  var api = window.netdataRum = window.netdataRum || {};
  api.setUser = function (u) {
    user = u && u.id != null && u.id !== '' ? { id: String(u.id) } : null;
    applyUser();
  };
  function faroApi() { return window.GrafanaFaroWebSdk && GrafanaFaroWebSdk.faro && GrafanaFaroWebSdk.faro.api; }
  function applyUser() {
    var f = faroApi();
    if (!f) { return; }
    try { if (user) { f.setUser(user); } else { f.resetUser(); } } catch (e) {}
  }
  var bot = false;
  try { bot = navigator.webdriver === true || new RegExp(opt.bots, 'i').test(navigator.userAgent || ''); } catch (e) {}
  if (bot && !opt.includeBots) { return; }
  // Pinned Faro web SDK and tracing add-on (Apache-2.0), from the public CDN.
  var cdn = 'https://cdn.jsdelivr.net/npm/@grafana/';
  function load(name, next, failed) {
    var s = document.createElement('script');
    s.async = true;
    s.src = cdn + name + '@` + FaroVersion + `/dist/bundle/' + name + '.iife.js';
    s.onload = next;
    if (failed) { s.onerror = failed; }
    document.head.appendChild(s);
  }
  function start() {
    try {
      var app = { name: document.location.hostname };
      if (ver) { app.version = ver; }
      if (env) { app.environment = env; }
      var inst = GrafanaFaroWebSdk.getWebInstrumentations();
      if (opt.tracing && window.GrafanaFaroWebTracing) {
        // Same-origin requests always get traceparent; other origins only
        // when listed, since their CORS must allow the header.
        inst.push(new GrafanaFaroWebTracing.TracingInstrumentation({
          instrumentationOptions: {
            propagateTraceHeaderCorsUrls: opt.tracing.map(function (p) { return new RegExp(p); })
          }
        }));
      }
      var cfg = { url: base + '/rum/' + k + '/collect' + (bot ? '?bot=1' : ''), app: app, instrumentations: inst };
      if (opt.sampling > 0 && opt.sampling < 1) { cfg.sessionTracking = { samplingRate: opt.sampling }; }
      GrafanaFaroWebSdk.initializeFaro(cfg);
      if (user) { applyUser(); }
      watchFrustration();
    } catch (e) { /* never break the host page */ }
  }
  // Frustration signals: rage clicks (3 within 1s on one spot), dead
  // clicks (a link or button that changes nothing within 1s) and error
  // clicks (a JavaScript error within 1s). Only a short CSS selector of the
  // element is sent, never its text; ordinary clicks send nothing.
  function selector(el) {
    var parts = [];
    for (var i = 0; el && el.nodeType === 1 && i < 3; i++, el = el.parentElement) {
      var tag = el.tagName.toLowerCase();
      if (el.id) { parts.unshift(tag + '#' + el.id); break; }
      var cls = (el.getAttribute('class') || '').trim().split(/\s+/).slice(0, 2).join('.');
      parts.unshift(cls ? tag + '.' + cls : tag);
    }
    return parts.join(' > ').slice(0, 200);
  }
  function signal(name, target) {
    var f = faroApi();
    if (f) { try { f.pushEvent(name, { target: target }); } catch (e) {} }
  }
  function watchFrustration() {
    var burst = [], last = null;
    document.addEventListener('click', function (ev) {
      var t = ev.target;
      if (!t || t.nodeType !== 1) { return; }
      var now = Date.now(), sel = selector(t);
      burst = burst.filter(function (c) { return now - c.t < 1000 && Math.abs(c.x - ev.clientX) < 30 && Math.abs(c.y - ev.clientY) < 30; });
      burst.push({ t: now, x: ev.clientX, y: ev.clientY });
      if (burst.length === 3) { signal('rage_click', sel); }
      last = { t: now, sel: sel };
      var control = t.closest && t.closest('a[href],button,[role=button]');
      if (!control || !window.MutationObserver || control.target === '_blank' || control.hasAttribute('download') || /^(mailto|tel):/.test(control.getAttribute('href') || '')) { return; }
      var changed = false, href = location.href;
      var requests = performance.getEntriesByType ? performance.getEntriesByType('resource').length : 0;
      var mo = new MutationObserver(function () { changed = true; });
      mo.observe(document.documentElement, { childList: true, subtree: true, attributes: true, characterData: true });
      setTimeout(function () {
        mo.disconnect();
        var now2 = performance.getEntriesByType ? performance.getEntriesByType('resource').length : 0;
        if (!changed && location.href === href && now2 === requests && !document.hidden) { signal('dead_click', sel); }
      }, 1000);
    }, true);
    function onError() {
      if (last && Date.now() - last.t < 1000) { signal('error_click', last.sel); last = null; }
    }
    window.addEventListener('error', onError);
    window.addEventListener('unhandledrejection', onError);
  }
  load('faro-web-sdk', function () {
    if (opt.tracing) { load('faro-web-tracing', start, start); } else { start(); }
  });
})(__KEY__, __BASE__, __OPT__);
`

var rumDemoPage = template.Must(template.New("demo").Parse(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>Netdata RUM demo</title>
<script async src="/rum/{{.Key}}.js"></script></head>
<body>
<h1>RUM demo page</h1>
<p>This page carries the Netdata RUM bootstrap for site
<code>{{.Key}}</code>. Open it in a real browser and the Faro web SDK reports
web vitals (LCP, FCP, CLS, TTFB, INP) and errors to the collector.</p>
<ul><li><a href="#" id="link" onclick="return false">A dummy link to interact with (INP)</a></li>
<li><a href="#" id="boom" onclick="throw new Error('demo error'); return false">Throw a JS error</a></li></ul>
</body></html>
`))
