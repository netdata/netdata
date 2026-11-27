// SPDX-License-Identifier: GPL-3.0-or-later

package runtimechartemit

import (
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
)

// Each runtime chart sample ends with its tick's time (`END <seconds> <microseconds> 1`)
// and its BEGIN carries no interval: a rejected tick delivers nothing, and a
// component with a longer update interval samples every other tick.
func TestRuntimeChartSampleTimes(t *testing.T) {
	type tick struct {
		reject bool
		want   []string // components with a sample this tick
	}
	// Ticks are one second apart; "slow" has an update interval of two ticks.
	ticks := []tick{
		{want: []string{"fast"}},
		{want: []string{"fast", "slow"}},
		{reject: true},
		{want: []string{"fast", "slow"}},
		{want: []string{"fast"}},
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

		var got, want []map[string]string
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
			got = append(got, runtimeChartTimes(out.String()[start:]))
			us := time.Now().UnixMicro()
			end := fmt.Sprintf("%d %d 1", us/1e6, us%1e6)
			w := map[string]string{}
			for _, name := range tc.want {
				w[name] = end
			}
			want = append(want, w)
		}
		assert.Equal(t, want, got)
	})
}

// runtimeChartTimes maps each runtime component's sample to the time its END
// carries; a BEGIN with an interval is reported as "BEGIN ...".
func runtimeChartTimes(wire string) map[string]string {
	times := make(map[string]string)
	component := ""
	for line := range strings.SplitSeq(wire, "\n") {
		if rest, ok := strings.CutPrefix(line, "BEGIN 'netdata.go.d.internal."); ok {
			id, extra, _ := strings.Cut(rest, "'")
			component, _, _ = strings.Cut(id, ".")
			if extra != "" {
				times[component] = "BEGIN" + extra
				component = ""
			}
			continue
		}
		if component != "" && (line == "END" || strings.HasPrefix(line, "END ")) {
			times[component] = strings.TrimPrefix(strings.TrimPrefix(line, "END"), " ")
			component = ""
		}
	}
	return times
}
