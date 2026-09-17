// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/testutil"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/stretchr/testify/require"
)

func TestDecodedCollectorPreservesDistinctEnrichmentReadings(t *testing.T) {
	const b = "/redfish/v1/"
	for _, route := range []string{"linked collection", "link array"} {
		for _, provenance := range []bool{false, true} {
			name := route + "/without provenance"
			if provenance {
				name = route + "/distinct provenance"
			}
			t.Run(name, func(t *testing.T) {
				docs := graphExcerptDocuments()
				first, second := b+"Chassis/C/EnvironmentMetrics/One", b+"Chassis/C/EnvironmentMetrics/Two"
				if route == "link array" {
					docs[b+"Chassis/C"]["EnvironmentMetrics"] = []any{testutil.Link(first), testutil.Link(second)}
				} else {
					docs[b+"Chassis/C/EnvironmentMetrics"] = testutil.Collection(b+"Chassis/C/EnvironmentMetrics", "EnvironmentMetrics", first, second)
				}
				for i, uri := range []string{first, second} {
					excerpt := map[string]any{"Reading": 10 * (i + 1), "Status": map[string]any{"Health": "OK"}}
					if provenance {
						excerpt["DataSourceUri"] = uri + "#/PowerWatts/Reading"
					}
					docs[uri] = testutil.Resource(
						uri,
						"EnvironmentMetrics",
						uri,
						map[string]any{"PowerWatts": excerpt},
					)
				}
				collector := sourceTestDecodedCollector(t, testutil.ServeDocuments(t, docs))
				sourceTestCollectCycle(t, collector)
				var values []float64
				var keys []string
				collector.MetricStore().
					Read(metrix.ReadFlatten()).
					ForEachByName("reading_power_value", func(labels metrix.LabelView, value metrix.SampleValue) {
						values = append(values, value)
						key, present := labels.Get("reading_key")
						require.True(t, present)
						require.NotEmpty(t, key)
						keys = append(keys, key)
					})
				require.ElementsMatch(t, []float64{10, 20}, values)
				require.Len(t, keys, 2)
				require.NotEqual(t, keys[0], keys[1])
			})
		}
	}
}
