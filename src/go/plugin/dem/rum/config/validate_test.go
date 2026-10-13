// SPDX-License-Identifier: GPL-3.0-or-later
package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTracingAddresses(t *testing.T) {
	for name, tc := range map[string]struct {
		destination, origin string
		valid               bool
	}{
		"dns":              {"https://collector.example:4317", "https://api.example:8443", true},
		"localhost":        {"http://localhost:4317", "http://localhost", true},
		"absolute DNS":     {"http://collector.example.:4317", "https://api.example", true},
		"ipv4":             {"http://127.0.0.1:4317", "http://127.0.0.1:8080", true},
		"ipv6 TLS":         {"https://[::1]:4317", "https://[2001:db8::1]:8443", true},
		"ipv6 plaintext":   {"http://[2001:db8::2]:4317", "http://[::1]", true},
		"port endpoints":   {"http://localhost:1", "https://api.example:65535", true},
		"export zero":      {destination: "http://collector.example:0"},
		"export overflow":  {destination: "http://collector.example:65536"},
		"origin zero":      {origin: "https://api.example:0"},
		"origin overflow":  {origin: "https://api.example:99999"},
		"leading dot":      {destination: "http://.example.org:4317"},
		"empty label":      {destination: "http://a..b:4317"},
		"missing port":     {destination: "http://collector.example"},
		"empty port":       {origin: "https://api.example:"},
		"bare target":      {destination: "collector.example:4317"},
		"userinfo":         {destination: "https://user@collector.example:4317"},
		"path":             {origin: "https://api.example/path"},
		"query":            {origin: "https://api.example?"},
		"fragment":         {origin: "https://api.example#part"},
		"wildcard":         {origin: "https://*.example.org"},
		"invalid IPv6":     {destination: "https://[gg::1]:4317"},
		"unbracketed IPv6": {destination: "http://::1:4317"},
	} {
		t.Run(name, func(t *testing.T) {
			tracing := &Tracing{
				Enabled: true,
			}
			if tc.destination != "" {
				tracing.Destination.Endpoint = &tc.destination
			}
			if tc.origin != "" {
				tracing.PropagateTo = []string{tc.origin}
			}
			errs := ValidateSiteExtras(Site{
				Tracing: tracing,
			})
			if tc.valid {
				assert.Empty(t, errs)
			} else {
				assert.NotEmpty(t, errs)
			}
		})
	}
}
