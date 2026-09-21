// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJobV2StoreFirst(t *testing.T) {
	const template = `
version: v1
engine:
  autogen:
    enabled: true
groups:
  - family: Workers
    metrics: [apache.workers_busy, apache.workers_idle]
    charts:
      - id: workers
        title: Workers
        context: workers
        units: workers
        dimensions:
          - selector: apache.workers_busy
            name: busy
          - selector: apache.workers_idle
            name: idle
`
	for _, provider := range []string{"yaml", "native"} {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/enabled=%t", provider, enabled), func(t *testing.T) {
				store := metrix.NewCollectorStore()
				cycle := 0
				base := &mockModuleV2{
					store:    store,
					template: template,
				}
				base.collectFunc = func(context.Context) error {
					cycle++
					for _, scope := range []metrix.HostScope{{}, {ScopeKey: "node", GUID: "node-guid", Hostname: "node-host"}} {
						meter := store.Write().SnapshotMeter("apache").WithHostScope(scope)
						meter.Gauge("workers_busy").Observe(7)
						meter.Gauge("queue").Observe(2) // unmatched: automatic chart
						if cycle > 1 {
							meter.Gauge("workers_idle").Observe(3)
						}
					}
					return nil
				}
				var mod collectorapi.CollectorV2 = base
				if provider == "native" {
					spec, err := charttpl.DecodeYAML([]byte(template))
					require.NoError(t, err)
					set, err := chartengine.NewTemplateSet(chartengine.TemplateSetSpec{
						Entries: []chartengine.TemplateEntry{{ID: "workers", Groups: spec.Groups}},
						Policy: chartengine.EnginePolicy{
							Autogen: &chartengine.AutogenPolicy{
								Enabled: true,
							},
						},
					})
					require.NoError(t, err)
					mod = &nativeModuleV2{
						CollectorV2: base,
						set:         set,
					}
				}
				var out bytes.Buffer
				job := NewJobV2(JobV2Config{
					PluginName:  pluginName,
					Name:        jobName,
					ModuleName:  modName,
					FullName:    modName + "_" + jobName,
					Module:      mod,
					Out:         &out,
					UpdateEvery: 1,
					StoreFirst:  enabled,
					Labels:      map[string]string{"instance": "localhost"},
				})
				require.NoError(t, job.AutoDetectionManaged(context.Background()))
				t.Cleanup(job.Cleanup)

				job.runOnce()
				assert.Contains(t, out.String(), "HOST 'node-guid'")
				assert.Contains(t, out.String(), "SET 'busy' = 7")
				assert.NotContains(t, out.String(), "DIMENSION 'idle'")
				assert.Equal(t, 4, assertStoreFirstWire(t, out.String(), enabled, false))

				out.Reset()
				job.runOnce()
				assert.Contains(t, out.String(), "DIMENSION 'idle'")
				assert.Contains(t, out.String(), "SET 'idle' = 3")
				assert.Equal(t, 2, assertStoreFirstWire(t, out.String(), enabled, false))

				out.Reset()
				job.Cleanup()
				assert.Contains(t, out.String(), "HOST 'node-guid'")
				assert.Equal(t, 4, assertStoreFirstWire(t, out.String(), enabled, true))
			})
		}
	}
}

func assertStoreFirstWire(t *testing.T, wire string, enabled, obsolete bool) int {
	t.Helper()
	count := 0
	collectorChart := false
	for _, line := range strings.Split(wire, "\n") {
		if strings.HasPrefix(line, "HOST ") {
			collectorChart = false
		}
		if strings.HasPrefix(line, "CHART ") {
			collectorChart = strings.HasPrefix(line, "CHART 'module_job.")
			if !collectorChart {
				assert.NotContains(t, line, "store_first", "framework self-metrics keep their defaults")
				continue
			}
			count++
			options := ""
			if obsolete {
				options = "obsolete"
			}
			if enabled {
				options = strings.TrimSpace(options + " store_first")
			}
			assert.Contains(t, line, " '1' '"+options+"' 'plugin' 'module'")
			if !enabled {
				assert.NotContains(t, line, "store_first")
			}
		}
		if strings.HasPrefix(line, "DIMENSION 'busy'") || strings.HasPrefix(line, "DIMENSION 'idle'") {
			assert.True(t, collectorChart, "collector dimensions require a preceding chart definition")
		}
	}
	return count
}
