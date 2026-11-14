// SPDX-License-Identifier: GPL-3.0-or-later
package rum_test

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"

	rumhistory "github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"

	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/rum"
	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitRejectsURLsWithoutHostname(t *testing.T) {
	db, err := demjournal.Open(context.Background(), "")
	require.NoError(t, err)
	defer db.Close()
	for _, field := range []string{"public_url", "allowed_origins"} {
		t.Run(field, func(t *testing.T) {
			c := rum.New(rum.Dependencies{
				Registry: rumregistry.New(),
				History:  rumhistory.NewStore(db),
			})
			c.Name = "shop"
			c.AllowedOrigins = []string{"https://example.org"}
			if field == "public_url" {
				c.PublicURL = "http://:19938"
			} else {
				c.AllowedOrigins = []string{"https://:443"}
			}
			require.Error(t, c.Init(context.Background()))
		})
	}
}

func TestCancelledPlaintextPreparationAndRun(t *testing.T) {
	db, err := demjournal.Open(context.Background(), "")
	require.NoError(t, err)
	defer db.Close()
	c := rum.New(rum.Dependencies{
		Registry: rumregistry.New(),
		History:  rumhistory.NewStore(db),
	})
	c.Name = "shop"
	c.AllowedOrigins = []string{"https://example.org"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, c.Init(ctx), context.Canceled)
	require.NoError(t, c.Init(context.Background()))
	ready := false
	require.ErrorIs(t, c.Run(ctx, func() { ready = true }), context.Canceled)
	assert.False(t, ready)
}

func TestInitSamplingRates(t *testing.T) {
	db, err := demjournal.Open(context.Background(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	for _, rate := range []float64{0, .25, 1, -.1, 1.1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		for _, field := range []string{"measure", "investigate"} {
			t.Run(fmt.Sprintf("%s/%v", field, rate), func(t *testing.T) {
				c := rum.New(rum.Dependencies{
					Registry: rumregistry.New(),
					History:  rumhistory.NewStore(db),
				})
				c.Name = "shop"
				c.AllowedOrigins = []string{"https://shop.example.org"}
				if field == "measure" {
					c.MeasureSampleRate = &rate
				} else {
					c.Investigate.SampleRate = &rate
				}
				err := c.Init(context.Background())
				if !math.IsNaN(rate) && rate >= 0 && rate <= 1 {
					require.NoError(t, err)
				} else {
					require.ErrorContains(t, err, "sample_rate must be between 0 and 1")
				}
			})
		}
	}
}

// Init accepts or rejects each site setting with a stable message; the checks run
// in this order, so the first failing rule names the error.
func TestInitValidationContract(t *testing.T) {
	db, err := demjournal.Open(context.Background(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	text := func(s string) *string { return &s }
	tests := map[string]struct {
		mutate   func(c *rum.Collector)
		want     string // exact error, "" for success
		contains string // used instead of want for errors carrying I/O detail
	}{
		"valid defaults": {mutate: func(*rum.Collector) {}},
		"valid wildcard, root path and base URL with path": {mutate: func(c *rum.Collector) {
			c.AllowedOrigins = []string{"https://*.example.org", "http://shop.example.org:8080/"}
			c.PublicURL = "https://cdn.example.org/netdata"
		}},
		"name with space": {
			mutate: func(c *rum.Collector) { c.Name = "my shop" },
			want:   "name must be a nonempty URL-safe site key: letters, digits, underscores or hyphens",
		},
		"empty name": {
			mutate: func(c *rum.Collector) { c.Name = "" },
			want:   "name must be a nonempty URL-safe site key: letters, digits, underscores or hyphens",
		},
		"window too short": {
			mutate: func(c *rum.Collector) { c.Window = confopt.Duration(30 * time.Second) },
			want:   "window must be between 1m and 30m",
		},
		"window too long": {
			mutate: func(c *rum.Collector) { c.Window = confopt.Duration(31 * time.Minute) },
			want:   "window must be between 1m and 30m",
		},
		"zero page groups": {
			mutate: func(c *rum.Collector) { c.PageGroups = 0 },
			want:   "page_groups and countries must be between 1 and 100",
		},
		"too many countries": {
			mutate: func(c *rum.Collector) { c.Countries = 101 },
			want:   "page_groups and countries must be between 1 and 100",
		},
		"no origins": {
			mutate: func(c *rum.Collector) { c.AllowedOrigins = nil },
			want:   "allowed_origins is required",
		},
		"origin with path": {
			mutate: func(c *rum.Collector) { c.AllowedOrigins = []string{"https://shop.example.org/cart"} },
			want:   `allowed origin "https://shop.example.org/cart" must be http(s)://host[:port]`,
		},
		"origin with other scheme": {
			mutate: func(c *rum.Collector) { c.AllowedOrigins = []string{"ftp://shop.example.org"} },
			want:   `allowed origin "ftp://shop.example.org" must be http(s)://host[:port]`,
		},
		"origin with credentials": {
			mutate: func(c *rum.Collector) { c.AllowedOrigins = []string{"https://user@shop.example.org"} },
			want:   `allowed origin "https://user@shop.example.org" must be http(s)://host[:port]`,
		},
		"inner wildcard": {
			mutate: func(c *rum.Collector) { c.AllowedOrigins = []string{"https://shop.*.example.org"} },
			want:   "origin wildcard is only allowed as a leading *.",
		},
		"bare wildcard": {
			mutate: func(c *rum.Collector) { c.AllowedOrigins = []string{"https://*."} },
			want:   "origin wildcard is only allowed as a leading *.",
		},
		"geolocation and bots together": {
			mutate: func(c *rum.Collector) {
				c.Capture.Geolocation = text("precise")
				c.Bots = "maybe"
			},
			want: "capture.geolocation must be off, country or city; bots must be exclude or include",
		},
		"propagation target with path": {
			mutate: func(c *rum.Collector) {
				c.Tracing.Enabled = true
				c.Tracing.PropagateTo = []string{"https://api.example.org/v1"}
			},
			want: "tracing.propagate_to entry https://api.example.org/v1 must be http(s)://host[:port] with port 1-65535",
		},
		"event log destination without port": {
			mutate: func(c *rum.Collector) {
				c.EventLogs.Enabled = true
				c.EventLogs.Destination.Endpoint = text("http://collector.example.org")
			},
			want: "event_logs.destination.endpoint must be http(s)://host:port with port 1-65535",
		},
		"trace destination with TLS over http": {
			mutate: func(c *rum.Collector) {
				c.Tracing.Enabled = true
				c.Tracing.Destination.TLSCA = "ca.pem"
			},
			want: "tracing.destination.TLS options require an https endpoint",
		},
		"invalid redaction pattern": {
			mutate: func(c *rum.Collector) {
				c.RedactPaths = []config.PathRule{{
					Pattern: "(",
					Replace: "x",
				}}
			},
			want: `redact_paths pattern "(" is not a valid regular expression`,
		},
		"measure rate above one": {
			mutate: func(c *rum.Collector) { c.MeasureSampleRate = new(1.5) },
			want:   "measure_sample_rate must be between 0 and 1",
		},
		"negative investigate rate": {
			mutate: func(c *rum.Collector) { c.Investigate.SampleRate = new(-0.5) },
			want:   "investigate sample_rate must be between 0 and 1",
		},
		"unknown always_keep": {
			mutate: func(c *rum.Collector) { c.Investigate.AlwaysKeep = []string{"slow"} },
			want:   "unknown investigate always_keep condition",
		},
		"public_url with query": {
			mutate: func(c *rum.Collector) { c.PublicURL = "https://cdn.example.org/?v=1" },
			want:   "public_url must be an http(s) base URL without credentials, query or fragment",
		},
		"public_url with credentials": {
			mutate: func(c *rum.Collector) { c.PublicURL = "https://user:pass@cdn.example.org" },
			want:   "public_url must be an http(s) base URL without credentials, query or fragment",
		},
		"unreadable trace CA": {
			mutate: func(c *rum.Collector) {
				c.Tracing.Enabled = true
				c.Tracing.Destination.Endpoint = text("https://collector.example.org:4317")
				c.Tracing.Destination.TLSCA = filepath.Join(t.TempDir(), "missing.pem")
			},
			contains: "tracing.destination: ",
		},
		"name reported before origins": {
			mutate: func(c *rum.Collector) {
				c.Name = "my shop"
				c.AllowedOrigins = nil
			},
			want: "name must be a nonempty URL-safe site key: letters, digits, underscores or hyphens",
		},
		"origins reported before rates": {
			mutate: func(c *rum.Collector) {
				c.AllowedOrigins = nil
				c.MeasureSampleRate = new(2.0)
			},
			want: "allowed_origins is required",
		},
		"extras reported before rates": {
			mutate: func(c *rum.Collector) {
				c.Bots = "maybe"
				c.MeasureSampleRate = new(2.0)
			},
			want: "bots must be exclude or include",
		},
		"site rules reported before window": {
			mutate: func(c *rum.Collector) {
				c.PageGroups = 0
				c.Window = confopt.Duration(30 * time.Second)
			},
			want: "page_groups and countries must be between 1 and 100",
		},
		"rates reported before public_url": {
			mutate: func(c *rum.Collector) {
				c.MeasureSampleRate = new(2.0)
				c.PublicURL = "ftp://cdn.example.org"
			},
			want: "measure_sample_rate must be between 0 and 1",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := rum.New(rum.Dependencies{
				Registry: rumregistry.New(),
				History:  rumhistory.NewStore(db),
			})
			c.Name = "shop"
			c.AllowedOrigins = []string{"https://shop.example.org"}
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
