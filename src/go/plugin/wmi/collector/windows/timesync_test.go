// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"context"
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

func TestReadTimeService(t *testing.T) {
	for name, tc := range map[string]struct {
		rows       queryResult
		wantValues [len(timeServiceCounters)]sample
		wantUsable bool
	}{
		// A host without a responding time source still reports its counters.
		"WinRM text, a valid zero and a malformed value": {
			rows: fixtureRows(map[string]any{
				"ComputedTimeOffset":       "59",
				"NTPClientTimeSourceCount": "0",
				"NTPRoundtripDelay":        "malformed",
			}),
			wantValues: timeServiceValues(map[string]sample{
				"ComputedTimeOffset":       {value: 59, valid: true},
				"NTPClientTimeSourceCount": {value: 0, valid: true},
			}),
			wantUsable: true,
		},
		"no usable counter": {
			rows: fixtureRows(map[string]any{"ComputedTimeOffset": "malformed"}),
		},
		"more than one row": {
			rows: fixtureRows(timeServiceRow(), timeServiceRow()),
		},
		"no row": {
			rows: fixtureRows(),
		},
	} {
		t.Run(name, func(t *testing.T) {
			var o observations
			assert.Equal(t, tc.wantUsable, o.readTimeService(tc.rows.rows))
			assert.Equal(t, tc.wantValues, o.timeService)
		})
	}
}

// TestCollector_TimeServiceCountersReachCharts checks that each Windows Time
// Service counter reaches one chart dimension, in that chart's units. The
// oracle is Microsoft's documentation of the counter set: the computed offset
// and the NTP round trip are in microseconds, the source count is a number of
// time sources.
func TestCollector_TimeServiceCountersReachCharts(t *testing.T) {
	counterUnits := map[string]string{
		"ComputedTimeOffset":       "microseconds",
		"NTPRoundtripDelay":        "microseconds",
		"NTPClientTimeSourceCount": "sources",
	}
	// chartPerCounterUnit converts a counter's unit to a chart's units.
	chartPerCounterUnit := map[[2]string]float64{
		{"microseconds", "milliseconds"}: 0.001,
		{"sources", "sources"}:           1,
	}
	// Every value of timeServiceRow is distinct, so a value names its counter.
	row := timeServiceRow()
	properties := make(map[float64]string, len(row))
	for property, value := range row {
		properties[fixtureNumber(t, value)] = property
	}

	synctest.Test(t, func(t *testing.T) {
		c := configuredCollector()
		c.Vnode = "fixture"
		c.newSession = func(context.Context) (querySession, error) {
			return &fakeSession{
				results: timedQueryResults(),
			}, nil
		}
		collect := startJob(t, c, vnodes.VirtualNode{
			Name:     "fixture",
			Hostname: "fixture",
			GUID:     "245dea48-ae2a-4697-a22e-050e7e358afb",
		})
		output := collect(c.UpdateEvery)
		samples := parseSamples(t, output)

		reached := make(map[string][]string)
		for id, chart := range parseCharts(t, output) {
			if !strings.Contains(chart.context, ".clock_sync_") {
				continue
			}
			for _, dimension := range chart.dimensions {
				where := chart.context + " " + dimension.name
				value, ok := samples[id][dimension.name]
				require.True(t, ok, "%s has no value", where)
				property, ok := properties[value]
				require.True(t, ok, "%s sends %v, no Windows Time Service counter's value", where, value)
				reached[property] = append(reached[property], where)

				scale, ok := chartPerCounterUnit[[2]string{counterUnits[property], chart.units}]
				require.True(t, ok, "%s: %s in %s", where, counterUnits[property], chart.units)
				shown := value * float64(dimension.multiplier) / float64(dimension.divisor)
				assert.InDelta(t, value*scale, shown, 1e-9, where)
				assert.Equal(t, "absolute", dimension.algorithm, where)
			}
		}
		for property := range row {
			assert.Len(t, reached[property], 1, "%s reaches %v", property, reached[property])
		}
	})
}

// timeServiceValues places samples, by property, where readTimeService stores
// them.
func timeServiceValues(values map[string]sample) [len(timeServiceCounters)]sample {
	var samples [len(timeServiceCounters)]sample
	for property, value := range values {
		index := slices.IndexFunc(timeServiceCounters[:], func(c rawCounter) bool { return c.property == property })
		samples[index] = value
	}
	return samples
}
