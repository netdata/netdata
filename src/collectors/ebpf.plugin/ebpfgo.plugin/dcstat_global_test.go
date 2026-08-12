package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
)

func TestDCStatGlobalStateUpdate(t *testing.T) {
	tests := map[string]struct {
		updates []dcstatGlobalCounters
		want    dcstatGlobalPublish
	}{
		"first sample publishes raw counters with no ratio": {
			// No previous sample means no interval to derive a ratio from; the
			// counters themselves are still published because the chart uses the
			// incremental algorithm.
			updates: []dcstatGlobalCounters{
				{Reference: 1000, Slow: 200, Miss: 50},
			},
			want: dcstatGlobalPublish{Ratio: 0, Reference: 1000, Slow: 200, Miss: 50},
		},
		"ratio uses this interval only": {
			updates: []dcstatGlobalCounters{
				{Reference: 1000, Slow: 200, Miss: 50},
				// interval: reference +100, miss +10 => 90% hits
				{Reference: 1100, Slow: 260, Miss: 60},
			},
			want: dcstatGlobalPublish{Ratio: 90, Reference: 1100, Slow: 260, Miss: 60},
		},
		"idle interval reports ratio 0": {
			// C dcstat convention: no lookups this interval => ratio 0 (cachestat
			// deliberately reports 100 in its idle case).
			updates: []dcstatGlobalCounters{
				{Reference: 1000, Slow: 200, Miss: 50},
				{Reference: 1000, Slow: 200, Miss: 50},
			},
			want: dcstatGlobalPublish{Ratio: 0, Reference: 1000, Slow: 200, Miss: 50},
		},
		"misses above references clamp the ratio to zero": {
			updates: []dcstatGlobalCounters{
				{Reference: 1000, Slow: 200, Miss: 50},
				{Reference: 1010, Slow: 260, Miss: 90},
			},
			want: dcstatGlobalPublish{Ratio: 0, Reference: 1010, Slow: 260, Miss: 90},
		},
		"counter reset is clamped instead of going negative": {
			updates: []dcstatGlobalCounters{
				{Reference: 1000, Slow: 200, Miss: 50},
				{Reference: 10, Slow: 2, Miss: 1},
			},
			want: dcstatGlobalPublish{Ratio: 0, Reference: 10, Slow: 2, Miss: 1},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			state := &dcstatGlobalState{}
			var got dcstatGlobalPublish
			var ok bool
			for _, update := range tc.updates {
				got, ok = state.Update(update)
			}
			if !ok {
				t.Fatal("Update reported an invalid publish")
			}
			if got != tc.want {
				t.Fatalf("Update = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestFormatDCStatGlobalCharts(t *testing.T) {
	updateEvery := 7

	ratio := formatDCStatGlobalChart(dcstatGlobalCharts[0], updateEvery)
	wantRatioHeader := "HOST ''\n\nCHART 'filesystem.dc_hit_ratio' '' 'Percentage of files inside directory cache' '%' 'directory_cache' 'filesystem.dc_hit_ratio' 'line' '21200' '7' '' 'ebpf-go.plugin' 'dcstat'\n"
	if !strings.Contains(ratio, wantRatioHeader) {
		t.Fatalf("ratio chart = %q, want substring %q", ratio, wantRatioHeader)
	}
	if !strings.Contains(ratio, "DIMENSION 'ratio' 'ratio' 'absolute' '1' '1' ''\n") {
		t.Fatalf("ratio chart is missing its dimension: %q", ratio)
	}

	reference := formatDCStatGlobalChart(dcstatGlobalCharts[1], updateEvery)
	// The counters are published cumulatively with the incremental algorithm, so
	// the unit must name a rate.
	if !strings.Contains(reference, "'files/s'") {
		t.Fatalf("dc_reference chart = %q, want unit 'files/s'", reference)
	}
	for _, dim := range []string{"reference", "slow", "miss"} {
		want := "DIMENSION '" + dim + "' '" + dim + "' 'incremental' '1' '1' ''\n"
		if !strings.Contains(reference, want) {
			t.Fatalf("dc_reference chart = %q, want substring %q", reference, want)
		}
	}
}

func formatDCStatGlobalChart(chart dcstatGlobalChart, updateEvery int) string {
	var buf bytes.Buffer
	api := netdataapi.New(&buf)

	api.HOST("")
	emitDCStatGlobalChart(api, chart, updateEvery)

	return buf.String()
}
