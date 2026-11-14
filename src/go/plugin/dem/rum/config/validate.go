// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"errors"
	"fmt"
	"math"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	siteKeyRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
	hostRe    = regexp.MustCompile(`^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)*\.?$`)

	errPublicURL = errors.New("public_url must be an http(s) base URL without credentials, query or fragment")
)

// Validate checks the site's syntax without reading files or contacting
// destinations. Rules run in a fixed order and the first failure is reported;
// optional settings report all of their problems together.
func (s Site) Validate() error {
	if !siteKeyRe.MatchString(s.Name) {
		return errors.New("name must be a nonempty URL-safe site key: letters, digits, underscores or hyphens")
	}
	if s.PageGroups < 1 || s.PageGroups > 100 || s.Countries < 1 || s.Countries > 100 {
		return errors.New("page_groups and countries must be between 1 and 100")
	}
	if len(s.AllowedOrigins) == 0 {
		return errors.New("allowed_origins is required")
	}
	for _, origin := range s.AllowedOrigins {
		u, err := url.Parse(origin)
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil ||
			(u.Path != "" && u.Path != "/") || hasQueryOrFragment(u, origin) || !validPort(u, false) {
			return fmt.Errorf("allowed origin %q must be http(s)://host[:port]", origin)
		}
		host := u.Hostname()
		if strings.Contains(host, "*") &&
			(!strings.HasPrefix(host, "*.") || strings.Contains(host[2:], "*") || len(host) == 2) {
			return errors.New("origin wildcard is only allowed as a leading *.")
		}
	}
	if errs := s.optionalErrors(); len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	if rate := s.MeasureRate(); math.IsNaN(rate) || rate < 0 || rate > 1 {
		return errors.New("measure_sample_rate must be between 0 and 1")
	}
	if s.Investigate != nil {
		if rate := s.InvestigateRate(); math.IsNaN(rate) || rate < 0 || rate > 1 {
			return errors.New("investigate sample_rate must be between 0 and 1")
		}
		for _, keep := range s.Investigate.AlwaysKeep {
			if keep != KeepErrors && keep != KeepPoorVitals {
				return errors.New("unknown investigate always_keep condition")
			}
		}
	}
	if s.PublicURL != "" && !validPublicURL(s.PublicURL) {
		return errPublicURL
	}
	return nil
}

// optionalErrors checks capture, bots, enabled exports and path redactions.
// Settings of disabled exports are retained without validation.
func (s Site) optionalErrors() []string {
	var errs []string
	switch s.GeolocationMode() {
	case GeolocationOff, GeolocationCountry, GeolocationCity:
	default:
		errs = append(errs, "capture.geolocation must be off, country or city")
	}
	if s.Bots != "" && s.Bots != BotsExclude && s.Bots != BotsInclude {
		errs = append(errs, "bots must be exclude or include")
	}
	if s.TracingOn() {
		for _, o := range s.Tracing.PropagateTo {
			if !validAddress(o, false) {
				errs = append(errs, "tracing.propagate_to entry "+o+" must be http(s)://host[:port] with port 1-65535")
			}
		}
	}
	s = s.Effective()
	for _, feature := range []struct {
		path        string
		enabled     bool
		destination Destination
	}{
		{"event_logs.destination", s.EventLogs.Enabled, s.EventLogs.Destination},
		{"tracing.destination", s.Tracing.Enabled, s.Tracing.Destination},
	} {
		if feature.enabled {
			for _, err := range ValidateDestination(feature.destination) {
				errs = append(errs, feature.path+"."+err)
			}
		}
	}
	for _, r := range s.RedactPaths {
		if r.Pattern == "" {
			continue // an empty row from the form: ignored, never matched
		}
		if _, err := regexp.Compile(r.Pattern); err != nil {
			errs = append(errs, "redact_paths pattern "+strconv.Quote(r.Pattern)+" is not a valid regular expression")
		}
	}
	return errs
}

// Validate checks the receiver's limits, proxies, public URL and TLS pairing
// without binding or reading certificate files. The receiver job checks Listen
// first, in Init, because resolving a named port can read the services database.
func (r Receiver) Validate() error {
	if r.MaxBodyBytes <= 0 || r.RateLimit.PerIPPerMin <= 0 || r.RateLimit.PerSitePerSec <= 0 {
		return errors.New("body and rate limits must be positive")
	}
	for _, p := range r.TrustedProxies {
		if _, err := netip.ParsePrefix(p); err != nil {
			if _, err := netip.ParseAddr(p); err != nil {
				return fmt.Errorf("trusted proxy %q: %w", p, err)
			}
		}
	}
	if r.PublicURL != "" && !validPublicURL(r.PublicURL) {
		return errPublicURL
	}
	if (r.TLSCert == "") != (r.TLSKey == "") {
		return errors.New("tls_cert and tls_key must be set together")
	}
	return nil
}

// validPublicURL accepts an absolute http(s) base URL; a path prefix is allowed.
func validPublicURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Hostname() != "" && (u.Scheme == "http" || u.Scheme == "https") && u.User == nil &&
		!hasQueryOrFragment(u, raw) && validPort(u, false)
}

// validAddress validates a literal origin or gRPC destination, not a URL
// carrying credentials, a path or a resolver-specific target. A destination
// requires a port.
func validAddress(raw string, destination bool) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Path != "" ||
		hasQueryOrFragment(u, raw) {
		return false
	}
	host := u.Hostname()
	if strings.HasPrefix(u.Host, "[") {
		addr, err := netip.ParseAddr(host)
		if err != nil || !addr.Is6() || addr.Zone() != "" {
			return false
		}
	} else if !hostRe.MatchString(host) {
		return false
	}
	return validPort(u, destination)
}

// hasQueryOrFragment reports a query or fragment delimiter, even an empty one.
// An empty fragment leaves no trace in url.URL, so the raw text is checked.
func hasQueryOrFragment(u *url.URL, raw string) bool {
	return u.RawQuery != "" || u.ForceQuery || strings.Contains(raw, "#")
}

// validPort accepts a port from 1 to 65535, or no port unless one is required;
// a bare ":" is never accepted.
func validPort(u *url.URL, required bool) bool {
	port := u.Port()
	if port == "" {
		return !required && !strings.HasSuffix(u.Host, ":")
	}
	n, err := strconv.Atoi(port)
	return err == nil && n >= 1 && n <= 65535
}

// ValidateDestination checks syntax without reading credentials or contacting the receiver.
func ValidateDestination(d Destination) []string {
	endpoint := d.EndpointURL()
	var errs []string
	if !validAddress(endpoint, true) {
		errs = append(errs, "endpoint must be http(s)://host:port with port 1-65535")
	}
	if (d.TLSCert == "") != (d.TLSKey == "") {
		errs = append(errs, "tls_cert and tls_key must be set together")
	}
	if !strings.HasPrefix(endpoint, "https://") && (d.TLSCA != "" || d.TLSCert != "" || d.TLSKey != "") {
		errs = append(errs, "TLS options require an https endpoint")
	}
	return errs
}
