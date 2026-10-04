// SPDX-License-Identifier: GPL-3.0-or-later

package rum

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/receiver"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/runtimehub"
	"github.com/netdata/netdata/go/plugins/plugin/dem/store"
	"github.com/stretchr/testify/require"
)

// Collection covers fixed instruments and a bounded set of active breakdowns.
// Cost follows the site's active series; timings are local trends, not CI gates.
func BenchmarkCollect(b *testing.B) {
	for _, measured := range []bool{false, true} {
		name := "quiet"
		if measured {
			name = "measured"
		}
		b.Run(name, func(b *testing.B) {
			ctx := context.Background()
			db, err := store.Open(ctx, "")
			require.NoError(b, err)
			defer db.Close()
			hub := runtimehub.New()
			revoke := hub.PublishReceiver(runtimehub.Availability{
				Serving: true,
			})
			defer revoke()
			c := New(Dependencies{
				Hub:     hub,
				History: db,
			})
			require.NoError(
				b,
				json.Unmarshal(
					[]byte(
						`{"name":"shop","display_name":"Shop","allowed_origins":["https://example.org"],"otlp":{"enabled":"no"}}`,
					),
					&c.Config,
				),
			)
			require.NoError(b, c.Init(ctx))
			defer c.Cleanup(ctx)
			if measured {
				c.aggregator.Ingest(&beacon.Beacon{
					Site:      "shop",
					SessionID: "browser",
					PageGroup: "/products",
					Path:      "/products",
					Browser:   "Chrome",
					Device:    "desktop",
					Country:   "GR",
					Received:  time.Now(),
					Vitals:    []beacon.Vital{{Name: beacon.LCP, Value: 1200}, {Name: beacon.CLS, Value: 0}},
				})
			}
			managed, ok := metrix.AsCycleManagedStore(c.MetricStore())
			require.True(b, ok)
			cycle := managed.CycleController()
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				cycle.BeginCycle()
				if err := c.Collect(ctx); err != nil {
					b.Fatal(err)
				}
				if err := cycle.CommitCycleSuccess(); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
		})
	}
}

func BenchmarkReceiverCollect(b *testing.B) {
	c := receiver.New(runtimehub.New())
	managed, ok := metrix.AsCycleManagedStore(c.MetricStore())
	require.True(b, ok)
	cycle := managed.CycleController()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		cycle.BeginCycle()
		if err := c.Collect(context.Background()); err != nil {
			b.Fatal(err)
		}
		if err := cycle.CommitCycleSuccess(); err != nil {
			b.Fatal(err)
		}
	}
}
