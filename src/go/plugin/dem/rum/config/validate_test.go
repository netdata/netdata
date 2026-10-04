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
		"dns":                {"https://collector.example:4317", "https://api.example:8443", true},
		"localhost":          {"localhost:4317", "http://localhost", true},
		"absolute DNS":       {"collector.example.:4317", "https://api.example", true},
		"ipv4":               {"127.0.0.1:4317", "http://127.0.0.1:8080", true},
		"ipv6 TLS":           {"https://[::1]:4317", "https://[2001:db8::1]:8443", true},
		"ipv6 plaintext":     {"[2001:db8::2]:4317", "http://[::1]", true},
		"port endpoints":     {"localhost:1", "https://api.example:65535", true},
		"export zero":        {destination: "collector.example:0"},
		"export overflow":    {destination: "collector.example:65536"},
		"origin zero":        {origin: "https://api.example:0"},
		"origin overflow":    {origin: "https://api.example:99999"},
		"leading dot":        {destination: ".example.org:4317"},
		"empty label":        {destination: "a..b:4317"},
		"missing port":       {destination: "collector.example"},
		"empty port":         {origin: "https://api.example:"},
		"export HTTP scheme": {destination: "http://collector.example:4317"},
		"userinfo":           {destination: "https://user@collector.example:4317"},
		"path":               {origin: "https://api.example/path"},
		"query":              {origin: "https://api.example?"},
		"fragment":           {origin: "https://api.example#part"},
		"wildcard":           {origin: "https://*.example.org"},
		"invalid IPv6":       {destination: "https://[gg::1]:4317"},
		"unbracketed IPv6":   {destination: "::1:4317"},
	} {
		t.Run(name, func(t *testing.T) {
			tracing := &Tracing{
				ExportTo: tc.destination,
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
