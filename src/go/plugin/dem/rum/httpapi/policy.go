// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
)

// snapshot is the immutable, compiled receiver configuration used by
// requests for one receiver generation. Site policy comes from each
// request's exact routing lease; neither is read from disk per request.
type snapshot struct {
	trusted   []netip.Prefix
	maxBody   int64
	publicURL string
}

type site struct {
	key         string
	origins     []originRule
	measureRate float64 // collection sampling for the snippet and receiver zero gate
	includeBots bool    // bots: include
	// tracing enables browser spans. propagate lists cross-origin trace
	// header patterns.
	tracing   bool
	propagate []string
	pathRules []beacon.PathRule // redact_paths
	redactor  *redact.Redactor
}

// originRule is one allowed_origins entry: scheme + host[:port], with
// wildcard true for a single leading "*." label.
type originRule struct {
	scheme   string
	host     string // lowercase, default port stripped; wildcard rules hold the suffix after "*."
	wildcard bool
}

func compile(cfg *config.Receiver) *snapshot {
	s := &snapshot{
		maxBody:   cfg.MaxBodyBytes,
		publicURL: strings.TrimRight(cfg.PublicURL, "/"),
	}
	for _, tp := range cfg.TrustedProxies {
		if p, err := netip.ParsePrefix(tp); err == nil {
			s.trusted = append(s.trusted, p)
		} else if a, err := netip.ParseAddr(tp); err == nil {
			s.trusted = append(s.trusted, netip.PrefixFrom(a, a.BitLen()))
		}
	}
	return s
}

func compileSite(sc config.Site) *site {
	st := &site{
		key:         sc.Name,
		measureRate: sc.MeasureRate(),
		includeBots: sc.IncludesBots(),
	}
	var secrets []string
	if sc.EventLogs != nil {
		secrets = append(secrets, sc.EventLogs.Destination.AuthToken)
	}
	if sc.Tracing != nil {
		secrets = append(secrets, sc.Tracing.Destination.AuthToken)
	}
	st.redactor = redact.NewRedactor(secrets...)
	if sc.TracingOn() {
		st.tracing = true
		for _, o := range sc.Tracing.PropagateTo {
			st.propagate = append(st.propagate, "^"+regexp.QuoteMeta(strings.TrimRight(o, "/"))+"(/|$)")
		}
	}
	for _, pr := range sc.RedactPaths {
		if pr.Pattern == "" {
			continue // an empty pattern would match everywhere
		}
		if re, err := regexp.Compile(pr.Pattern); err == nil {
			st.pathRules = append(st.pathRules, beacon.PathRule{
				Re:      re,
				Replace: pr.Replace,
			})
		}
	}
	for _, o := range sc.AllowedOrigins {
		if r, ok := parseOriginRule(o); ok {
			st.origins = append(st.origins, r)
		}
	}
	return st
}

func parseOriginRule(o string) (originRule, bool) {
	u, err := url.Parse(strings.TrimSpace(o))
	if err != nil || u.Host == "" {
		return originRule{}, false
	}
	r := originRule{
		scheme: strings.ToLower(u.Scheme),
	}
	host := normalizeHostPort(r.scheme, u.Host)
	if strings.HasPrefix(host, "*.") {
		r.wildcard = true
		host = host[1:] // keep the leading "." so "*.example.com" never matches "example.com"
	}
	r.host = host
	return r, true
}

// normalizeHostPort lowercases and strips the scheme's default port so
// "https://a.example.com:443" and "https://a.example.com" compare equal.
func normalizeHostPort(scheme, hostport string) string {
	hp := strings.ToLower(hostport)
	if strings.HasSuffix(hp, ":443") && scheme == "https" {
		return strings.TrimSuffix(hp, ":443")
	}
	if strings.HasSuffix(hp, ":80") && scheme == "http" {
		return strings.TrimSuffix(hp, ":80")
	}
	return hp
}

// matchOrigin returns the request origin to echo when it is allowed for
// the site. Only the exact request origin is ever echoed, never "*".
func (st *site) matchOrigin(origin string) (string, bool) {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.Scheme == "" {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	host := normalizeHostPort(scheme, u.Host)
	for _, r := range st.origins {
		if r.scheme != scheme {
			continue
		}
		if r.wildcard {
			if strings.HasSuffix(host, r.host) && len(host) > len(r.host) {
				return scheme + "://" + u.Host, true
			}
			continue
		}
		if host == r.host {
			return scheme + "://" + u.Host, true
		}
	}
	return "", false
}

// requestOrigin takes the Origin header, falling back to the Referer's
// origin (some beacon transports omit Origin).
func requestOrigin(r *http.Request) string {
	if o := r.Header.Get("Origin"); o != "" && o != "null" {
		return o
	}
	if ref := r.Header.Get("Referer"); ref != "" {
		if u, err := url.Parse(ref); err == nil && u.Scheme != "" && u.Host != "" {
			return u.Scheme + "://" + u.Host
		}
	}
	return ""
}

// clientIP uses RemoteAddr unless it is a trusted proxy. In that case,
// the right-most X-Forwarded-For entry that is not a trusted proxy
// (or X-Real-IP) identifies the client.
func (s *snapshot) clientIP(r *http.Request) netip.Addr {
	remote := parseAddr(r.RemoteAddr)
	if !remote.IsValid() || !s.isTrusted(remote) {
		return remote
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			a := parseAddr(strings.TrimSpace(parts[i]))
			if !a.IsValid() {
				return remote // malformed chain: do not trust it
			}
			if !s.isTrusted(a) {
				return a
			}
		}
		return remote // every hop trusted: the proxy itself is the client
	}
	if xr := parseAddr(strings.TrimSpace(r.Header.Get("X-Real-IP"))); xr.IsValid() {
		return xr
	}
	return remote
}

func (s *snapshot) isTrusted(a netip.Addr) bool {
	a = a.Unmap()
	for _, p := range s.trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

func parseAddr(hostport string) netip.Addr {
	if host, _, err := net.SplitHostPort(hostport); err == nil {
		hostport = host
	}
	a, err := netip.ParseAddr(strings.Trim(hostport, "[]"))
	if err != nil {
		return netip.Addr{}
	}
	return a.Unmap()
}
