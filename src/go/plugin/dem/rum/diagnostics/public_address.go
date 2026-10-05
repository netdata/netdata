// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"net"
	"os"
	"strings"
)

// PublicBase is the base URL for site's snippet: public_url when set,
// else the address learned through a trusted proxy once a probe confirmed
// it reaches this collector, else fallback (the listener).
func (s *State) PublicBase(fallback string) string {
	if s.config.PublicURL != "" {
		return strings.TrimRight(s.config.PublicURL, "/")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hasResult && s.observed != "" && s.result.State == ReachOK &&
		s.result.Base == s.observed {
		return s.observed
	}
	return fallback
}

// ListenerBase derives the advertised address when no public URL is configured.
// Bind-all hosts use the machine hostname so the result is not a wildcard address.
func ListenerBase(listen string, tls bool) string {
	scheme := "http://"
	if tls {
		scheme = "https://" // the listener only speaks TLS when a cert is configured
	}
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return scheme + listen // defensive; validate.Config already rejects this
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		if h, herr := os.Hostname(); herr == nil && h != "" {
			host = h
		}
	}
	return scheme + net.JoinHostPort(host, port)
}
