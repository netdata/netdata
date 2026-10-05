// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var tracingHostRe = regexp.MustCompile(`^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)*\.?$`)

// tracingAddress validates a literal origin or gRPC destination, not a URL
// carrying credentials, a path or a resolver-specific target.
func tracingAddress(raw string, destination bool) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil ||
		u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") {
		return false
	}
	host := u.Hostname()
	if strings.HasPrefix(u.Host, "[") {
		addr, err := netip.ParseAddr(host)
		if err != nil || !addr.Is6() || addr.Zone() != "" {
			return false
		}
	} else if !tracingHostRe.MatchString(host) {
		return false
	}
	port := u.Port()
	if port == "" {
		return !destination && !strings.HasSuffix(u.Host, ":")
	}
	n, err := strconv.Atoi(port)
	return err == nil && n >= 1 && n <= 65535
}

// ValidateSiteExtras checks optional site settings,
// shared by file config and dynamic configuration.
func ValidateSiteExtras(s Site) []string {
	var errs []string
	switch s.GeolocationMode() {
	case GeolocationOff, GeolocationCountry, GeolocationCity:
	default:
		errs = append(errs, "capture.geolocation must be off, country or city")
	}
	if s.Bots != "" && s.Bots != BotsExclude && s.Bots != BotsInclude {
		errs = append(errs, "bots must be exclude or include")
	}
	if t := s.Tracing; t != nil && t.Enabled {
		for _, o := range t.PropagateTo {
			if !tracingAddress(o, false) {
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

// ValidateDestination checks syntax without reading credentials or contacting the receiver.
func ValidateDestination(d Destination) []string {
	endpoint := d.EndpointURL()
	var errs []string
	if !tracingAddress(endpoint, true) {
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
