// SPDX-License-Identifier: GPL-3.0-or-later
package receiver

import (
	"context"
	"path/filepath"
	"testing"

	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitValidatesListenPort(t *testing.T) {
	for _, listen := range []string{"127.0.0.1:65536", "127.0.0.1:-1", "127.0.0.1:not-a-tcp-service"} {
		t.Run(listen, func(t *testing.T) {
			c := New(Dependencies{
				Registry: rumregistry.New(),
			})
			c.Listen = listen
			t.Cleanup(func() { c.Cleanup(context.Background()) })
			require.ErrorContains(t, c.Init(context.Background()), "listen")
		})
	}
	for _, listen := range []string{"127.0.0.1:0", "127.0.0.1:65535", "127.0.0.1:http", "[::1]:19938"} {
		t.Run(listen, func(t *testing.T) {
			c := New(Dependencies{
				Registry: rumregistry.New(),
			})
			c.Listen = listen
			t.Cleanup(func() { c.Cleanup(context.Background()) })
			require.NoError(t, c.Init(context.Background()))
		})
	}
}

func TestInitRejectsPublicURLWithoutHostname(t *testing.T) {
	c := New(Dependencies{
		Registry: rumregistry.New(),
	})
	c.PublicURL = "http://:19938"
	t.Cleanup(func() { c.Cleanup(context.Background()) })
	require.ErrorContains(t, c.Init(context.Background()), "public_url")
}

func TestCancelledPlaintextPreparationAndRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := New(Dependencies{
		Registry: rumregistry.New(),
	})
	c.Listen = "127.0.0.1:0"
	t.Cleanup(func() { c.Cleanup(context.Background()) })
	assert.ErrorIs(t, c.Init(ctx), context.Canceled)
	require.NoError(t, c.Init(context.Background()))
	ready := false
	require.ErrorIs(t, c.Run(ctx, func() { ready = true }), context.Canceled)
	assert.False(t, ready)
	assert.False(t, c.registry.Availability().Serving)
}

// Init accepts or rejects each receiver setting with a stable message; the checks
// run in this order, so the first failing rule names the error.
func TestInitValidationContract(t *testing.T) {
	tests := map[string]struct {
		mutate   func(c *Collector)
		want     string // exact error, "" for success
		contains string // used instead of want for errors carrying parser or I/O detail
	}{
		"valid defaults": {mutate: func(*Collector) {}},
		"valid proxies and base URL with path": {mutate: func(c *Collector) {
			c.TrustedProxies = []string{"10.0.0.0/8", "192.0.2.10", "2001:db8::/32"}
			c.PublicURL = "https://rum.example.org/netdata"
		}},
		"missing routes": {
			mutate: func(c *Collector) { c.registry = nil },
			want:   "missing runtime routes",
		},
		"listen without port": {
			mutate:   func(c *Collector) { c.Listen = "127.0.0.1" },
			contains: "listen: ",
		},
		"zero body limit": {
			mutate: func(c *Collector) { c.MaxBodyBytes = 0 },
			want:   "body and rate limits must be positive",
		},
		"zero per-IP rate": {
			mutate: func(c *Collector) { c.RateLimit.PerIPPerMin = 0 },
			want:   "body and rate limits must be positive",
		},
		"negative per-site rate": {
			mutate: func(c *Collector) { c.RateLimit.PerSitePerSec = -1 },
			want:   "body and rate limits must be positive",
		},
		"invalid trusted proxy": {
			mutate:   func(c *Collector) { c.TrustedProxies = []string{"proxy.example.org"} },
			contains: `trusted proxy "proxy.example.org": `,
		},
		"public_url with fragment": {
			mutate: func(c *Collector) { c.PublicURL = "https://rum.example.org/#top" },
			want:   "public_url must be an http(s) base URL without credentials, query or fragment",
		},
		"public_url with other scheme": {
			mutate: func(c *Collector) { c.PublicURL = "ftp://rum.example.org" },
			want:   "public_url must be an http(s) base URL without credentials, query or fragment",
		},
		"certificate without key": {
			mutate: func(c *Collector) { c.TLSCert = "cert.pem" },
			want:   "tls_cert and tls_key must be set together",
		},
		"unreadable certificate": {
			mutate: func(c *Collector) {
				dir := t.TempDir()
				c.TLSCert, c.TLSKey = filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
			},
			contains: "listener TLS: ",
		},
		"limits reported before proxies": {
			mutate: func(c *Collector) {
				c.MaxBodyBytes = 0
				c.TrustedProxies = []string{"proxy.example.org"}
			},
			want: "body and rate limits must be positive",
		},
		"proxies reported before public_url": {
			mutate: func(c *Collector) {
				c.TrustedProxies = []string{"proxy.example.org"}
				c.PublicURL = "ftp://rum.example.org"
			},
			contains: `trusted proxy "proxy.example.org": `,
		},
		"public_url reported before TLS pairing": {
			mutate: func(c *Collector) {
				c.PublicURL = "ftp://rum.example.org"
				c.TLSCert = "cert.pem"
			},
			want: "public_url must be an http(s) base URL without credentials, query or fragment",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := New(Dependencies{
				Registry: rumregistry.New(),
			})
			c.Listen = "127.0.0.1:0"
			t.Cleanup(func() { c.Cleanup(context.Background()) })
			tc.mutate(c)
			err := c.Init(context.Background())
			switch {
			case tc.contains != "":
				require.ErrorContains(t, err, tc.contains)
			case tc.want != "":
				require.EqualError(t, err, tc.want)
			default:
				require.NoError(t, err)
			}
		})
	}
}
