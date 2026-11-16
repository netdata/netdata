// SPDX-License-Identifier: GPL-3.0-or-later

package ping

import (
	"context"
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/pinger"
	"github.com/stretchr/testify/require"
)

// BenchmarkPublication measures framework publication for a fixed-template
// collector with one chart group per host; Collect is untimed.
func BenchmarkPublication(b *testing.B) {
	for _, hosts := range []int{3, 100} {
		b.Run(fmt.Sprintf("hosts_%d", hosts), func(b *testing.B) {
			client := &mockClient{
				byHost: make(map[string]probeResult),
			}
			collr := New()
			collr.UpdateEvery = 1
			for i := range hosts {
				host := fmt.Sprintf("192.0.2.%d", i+1)
				collr.Hosts = append(collr.Hosts, host)
				client.byHost[host] = probeResult{
					sample: sampleForHost(host),
				}
			}
			collr.newPinger = func(pinger.Config, *logger.Logger) (pinger.Client, error) { return client, nil }
			require.NoError(b, collr.Init(context.Background()))

			collecttest.BenchmarkPublication(b, collr, func(ctx context.Context) error {
				client.mu.Lock()
				client.calls = client.calls[:0] // keep the mock's call log bounded
				client.mu.Unlock()
				return collr.Collect(ctx)
			})
		})
	}
}
