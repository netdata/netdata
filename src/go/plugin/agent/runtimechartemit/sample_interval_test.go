// SPDX-License-Identifier: GPL-3.0-or-later

package runtimechartemit

import (
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
)

// Each runtime chart's BEGIN interval spans every tick since its previous
// delivered sample: a rejected tick delivers nothing, and a component with a
// longer update interval skips ticks.
func TestRuntimeChartSampleIntervals(t *testing.T) {
	type tick struct {
		reject bool
		want   map[string]string // component -> BEGIN interval, "" when none
	}
	// Ticks are one second apart; "slow" has an update interval of two ticks.
	ticks := []tick{
		{want: map[string]string{"fast": ""}},
		{want: map[string]string{"fast": "1000000", "slow": ""}},
		{reject: true, want: map[string]string{}},
		{want: map[string]string{"fast": "2000000", "slow": "2000000"}},
		{want: map[string]string{"fast": "1000000"}},
	}

	synctest.Test(t, func(t *testing.T) {
		reg := newComponentRegistry()
		for name, every := range map[string]int{"fast": 1, "slow": 2} {
			store := metrix.NewRuntimeStore()
			store.Write().StatefulMeter("component").Gauge("load").Set(5)
			reg.upsert(componentSpec{
				Name:         name,
				Store:        store,
				TemplateYAML: []byte(runtimeGaugeTemplateYAML()),
				UpdateEvery:  every,
				EmitEnv: chartemit.EmitEnv{
					TypeID:      "netdata.go.d.internal." + name,
					UpdateEvery: every,
					Plugin:      "go.d",
					Module:      "internal",
					JobName:     name,
				},
			})
		}
		var out safeBuffer
		job := newRuntimeMetricsJob(&out, reg, nil)

		var got []map[string]string
		for i, tc := range ticks {
			if i > 0 {
				time.Sleep(time.Second)
			}
			job.out = &out
			if tc.reject {
				job.out = failingWriter{}
			}
			start := out.Len()
			job.runOnce(i + 1)
			got = append(got, runtimeChartIntervals(out.String()[start:]))
		}
		want := make([]map[string]string, 0, len(ticks))
		for _, tc := range ticks {
			want = append(want, tc.want)
		}
		assert.Equal(t, want, got)
	})
}

// runtimeChartIntervals maps each runtime component's BEGIN to its interval in
// microseconds, "" when the BEGIN carries none.
func runtimeChartIntervals(wire string) map[string]string {
	intervals := make(map[string]string)
	for line := range strings.SplitSeq(wire, "\n") {
		rest, ok := strings.CutPrefix(line, "BEGIN 'netdata.go.d.internal.")
		if !ok {
			continue
		}
		typeID, interval, _ := strings.Cut(rest, "' ")
		component, _, _ := strings.Cut(strings.TrimSuffix(typeID, "'"), ".")
		intervals[component] = interval
	}
	return intervals
}
