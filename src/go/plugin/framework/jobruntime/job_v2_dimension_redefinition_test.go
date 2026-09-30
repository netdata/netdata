// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/hostoutput"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJobV2RetainedDimensionAfterDescriptorEviction(t *testing.T) {
	for name, tc := range map[string]struct {
		first, next      func(metrix.SnapshotMeter)
		algorithm        string
		initial, changed string
	}{
		"gauge to counter": {
			first:   func(m metrix.SnapshotMeter) { m.Gauge("changing").Observe(7) },
			next:    func(m metrix.SnapshotMeter) { m.Counter("changing").ObserveTotal(100) },
			initial: "'absolute' '1' '1' 'type=int'",
			changed: "'incremental' '1' '1' 'type=int'",
		},
		"counter to gauge": {
			first:   func(m metrix.SnapshotMeter) { m.Counter("changing").ObserveTotal(7) },
			next:    func(m metrix.SnapshotMeter) { m.Gauge("changing").Observe(100) },
			initial: "'incremental' '1' '1' 'type=int'",
			changed: "'absolute' '1' '1' 'type=int'",
		},
		"float to int": {
			first:   func(m metrix.SnapshotMeter) { m.Gauge("changing", metrix.WithFloat(true)).Observe(7.5) },
			next:    func(m metrix.SnapshotMeter) { m.Gauge("changing").Observe(100) },
			initial: "'absolute' '1' '1' 'type=float'",
			changed: "'absolute' '1' '1' 'type=int'",
		},
		"explicit algorithm stays authoritative": {
			first:     func(m metrix.SnapshotMeter) { m.Gauge("changing").Observe(7) },
			next:      func(m metrix.SnapshotMeter) { m.Counter("changing").ObserveTotal(100) },
			algorithm: "algorithm: absolute",
			initial:   "'absolute' '1' '1' 'type=int'",
		},
	} {
		t.Run(name, func(t *testing.T) {
			store := metrix.NewCollectorStore()
			set, err := chartengine.NewTemplateSetYAML([]byte(fmt.Sprintf(`
version: v1
context_namespace: test
groups:
  - family: Test
    metrics: [changing, steady]
    charts:
      - id: together
        title: Two measurements
        context: together
        units: value
        %s
        dimensions:
          - selector: changing
            name: changing
          - selector: steady
            name: steady
`, tc.algorithm)))
			require.NoError(t, err)
			base := &mockModuleV2{store: store}
			mod := &nativeModuleV2{CollectorV2: base, set: set}
			out := &selectiveNativeOutput{}
			job := NewJobV2(JobV2Config{
				PluginName:  pluginName,
				Name:        jobName,
				ModuleName:  modName,
				FullName:    modName + "_" + jobName,
				Module:      mod,
				Out:         out,
				UpdateEvery: 1,
				Publication: hostoutput.New(),
			})
			require.NoError(t, job.AutoDetectionManaged(context.Background()))
			collect := func(observe func(metrix.SnapshotMeter)) error {
				out.Reset()
				base.collectFunc = func(context.Context) error {
					meter := store.Write().SnapshotMeter("")
					meter.Gauge("steady").Observe(1)
					if observe != nil {
						observe(meter)
					}
					return nil
				}
				return job.finishPreparedEmission(collectNativeFrame(t, job))
			}
			require.NoError(t, collect(tc.first))
			require.Contains(t, out.String(), "DIMENSION 'changing' 'changing' "+tc.initial)
			window := store.(metrix.DescriptorRetention).DescriptorRetentionWindow()
			for i := uint64(0); i <= window; i++ {
				require.NoError(t, collect(nil))
				require.NotContains(t, out.String(), "obsolete")
			}
			require.Greater(t, store.Read(metrix.ReadRaw()).CollectMeta().EvictedDescriptors, uint64(0))
			_, present := store.Read(metrix.ReadRaw()).Value("changing", nil)
			require.False(t, present)

			// Reject after collection commits but before its output is accepted.
			out.reject = "SET 'changing'"
			require.Error(t, collect(tc.next))
			require.Empty(t, out.String())
			value, present := store.Read(metrix.ReadRaw()).Value("changing", nil)
			require.True(t, present)
			require.Equal(t, float64(100), value)

			out.reject = ""
			require.NoError(t, collect(tc.next))
			wire := out.String()
			assert.NotContains(t, wire, "DIMENSION 'steady'")
			assert.NotContains(t, wire, "obsolete")
			if tc.changed != "" {
				definition := "DIMENSION 'changing' 'changing' " + tc.changed
				require.Contains(t, wire, definition)
				assert.Equal(t, 1, strings.Count(wire, "DIMENSION 'changing'"))
				assert.Less(t, strings.Index(wire, definition), strings.Index(wire, "BEGIN 'module_job.together'"))
			} else {
				assert.NotContains(t, wire, "DIMENSION")
			}
			assert.Contains(t, wire, "SET 'changing' = 100")
			require.NoError(t, collect(tc.next))
			assert.NotContains(t, out.String(), "DIMENSION", "unchanged definitions must not churn")
		})
	}
}
