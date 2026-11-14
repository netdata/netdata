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
			err := validateWithRequired(Site{
				Tracing: tracing,
			})
			if tc.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

// validateWithRequired validates s after filling the required site fields, so a
// test exercises only the settings it sets.
func validateWithRequired(s Site) error {
	s.Name = "shop"
	s.PageGroups, s.Countries = 20, 20
	s.AllowedOrigins = []string{"https://example.org"}
	return s.Validate()
}

// The public URL prefixes the generated script URL, so a query or fragment
// delimiter, even an empty one, or an unusable port breaks the snippet.
func TestPublicURLs(t *testing.T) {
	for name, tc := range map[string]struct {
		url   string
		valid bool
	}{
		"base":           {"https://rum.example.org", true},
		"trailing slash": {"https://rum.example.org/", true},
		"path prefix":    {"http://rum.example.org:8080/edge", true},
		"ipv6":           {"https://[2001:db8::1]:8443", true},
		"no host":        {url: "http://:19938"},
		"other scheme":   {url: "ftp://rum.example.org"},
		"credentials":    {url: "https://user:pass@rum.example.org"},
		"query":          {url: "https://rum.example.org/?v=1"},
		"empty query":    {url: "https://rum.example.org/?"},
		"fragment":       {url: "https://rum.example.org/#top"},
		"empty fragment": {url: "https://rum.example.org#"},
		"empty port":     {url: "https://rum.example.org:"},
		"port zero":      {url: "https://rum.example.org:0"},
		"port overflow":  {url: "https://rum.example.org:65536"},
	} {
		t.Run(name, func(t *testing.T) {
			receiver := Receiver{
				MaxBodyBytes: 1,
				RateLimit: RateLimit{
					PerIPPerMin:   1,
					PerSitePerSec: 1,
				},
				PublicURL: tc.url,
			}
			siteErr := validateWithRequired(Site{PublicURL: tc.url})
			receiverErr := receiver.Validate()
			if tc.valid {
				assert.NoError(t, siteErr)
				assert.NoError(t, receiverErr)
			} else {
				assert.EqualError(t, siteErr, errPublicURL.Error())
				assert.EqualError(t, receiverErr, errPublicURL.Error())
			}
		})
	}
}

// Browsers send an origin without an empty or out-of-range port, query or
// fragment, so an allowed origin carrying one could never match a request.
func TestAllowedOrigins(t *testing.T) {
	for name, tc := range map[string]struct {
		origin string
		valid  bool
	}{
		"host":              {"https://example.org", true},
		"trailing slash":    {"https://example.org/", true},
		"port":              {"http://shop.example.org:8080", true},
		"port endpoints":    {"https://shop.example.org:65535", true},
		"wildcard":          {"https://*.example.org", true},
		"wildcard and port": {"https://*.example.org:8443", true},
		"ipv4":              {"http://127.0.0.1:8080", true},
		"ipv6":              {"https://[2001:db8::1]:8443", true},
		"empty port":        {origin: "https://example.org:"},
		"wildcard empty":    {origin: "https://*.example.org:"},
		"ipv6 empty port":   {origin: "https://[2001:db8::1]:"},
		"port zero":         {origin: "https://example.org:0"},
		"port overflow":     {origin: "https://example.org:65536"},
		"empty query":       {origin: "https://example.org?"},
		"empty fragment":    {origin: "https://example.org#"},
	} {
		t.Run(name, func(t *testing.T) {
			err := Site{
				Name:           "shop",
				PageGroups:     20,
				Countries:      20,
				AllowedOrigins: []string{tc.origin},
			}.Validate()
			if tc.valid {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, `allowed origin "`+tc.origin+`" must be http(s)://host[:port]`)
			}
		})
	}
}
