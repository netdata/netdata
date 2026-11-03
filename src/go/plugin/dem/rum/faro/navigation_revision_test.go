// SPDX-License-Identifier: GPL-3.0-or-later

package faro_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/faro"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNavigationRevisionIgnoresEmptyTimingReports(t *testing.T) {
	for name, timings := range map[string]string{
		"metadata only":   "",
		"invalid timings": `,"pageLoadTime":"-1","domContentLoadHandlerTime":"NaN"`,
	} {
		for _, separate := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/separate=%t", name, separate), func(t *testing.T) {
				a := aggregate.New(time.Hour, aggregate.SiteCfg{Name: "shop"})
				valid := `{"name":"faro.performance.navigation","attributes":{"observation_id":"nav1","observation_sequence":"1","pageLoadTime":"123","domContentLoadHandlerTime":"10"}}`
				empty := `{"name":"faro.performance.navigation","attributes":{"observation_id":"nav2","observation_sequence":"2"` + timings + `}}`
				batches := []string{valid + "," + empty}
				if separate {
					batches = []string{valid, empty}
				}
				for i, events := range batches {
					raw := `{"meta":{"page":{"id":"document","url":"https://shop.example/entry"}},"events":[` + events + `]}`
					b, err := faro.Decode([]byte(raw), faro.Options{Site: "shop", Now: time.Now()})
					require.NoError(t, err)
					if separate && i == 1 {
						assert.Nil(t, b.Navigation)
					}
					a.Ingest(b)
				}
				snapshot := a.Snapshot()
				assert.EqualValues(t, 1, snapshot.Load.N)
				assert.Equal(t, float64(123), snapshot.Load.P75)
				assert.EqualValues(t, 1, snapshot.DCL.N)
				assert.Equal(t, float64(10), snapshot.DCL.P75)

				// A valid partial revision still replaces the previous report,
				// including removing its now-absent load measurement.
				raw := `{"meta":{"page":{"id":"document","url":"https://shop.example/entry"}},"events":[{"name":"faro.performance.navigation","attributes":{"observation_id":"nav3","observation_sequence":"3","domContentLoadHandlerTime":"0"}}]}`
				b, err := faro.Decode([]byte(raw), faro.Options{Site: "shop", Now: time.Now()})
				require.NoError(t, err)
				a.Ingest(b)
				snapshot = a.Snapshot()
				assert.Zero(t, snapshot.Load.N)
				assert.EqualValues(t, 1, snapshot.DCL.N)
				assert.Zero(t, snapshot.DCL.P75)
			})
		}
	}
}
