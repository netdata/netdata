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
		"first sample publishes legacy cumulative ratio and count totals": {
			updates: []dcstatGlobalCounters{
				{Reference: 1000, Slow: 200, Miss: 50},
			},
			want: dcstatGlobalPublish{Ratio: 95, Reference: 1000, Slow: 200, Miss: 50},
		},
		"ratio uses attach-lifetime counters while counts use this interval": {
			updates: []dcstatGlobalCounters{
				{Reference: 1000, Slow: 200, Miss: 50},
				// cumulative: 1100 reference, 60 miss => 94% hits;
				// interval: reference +100, slow +60, miss +10.
				{Reference: 1100, Slow: 260, Miss: 60},
			},
			want: dcstatGlobalPublish{Ratio: 94, Reference: 100, Slow: 60, Miss: 10},
		},
		"idle interval retains cumulative ratio": {
			updates: []dcstatGlobalCounters{
				{Reference: 1000, Slow: 200, Miss: 50},
				{Reference: 1000, Slow: 200, Miss: 50},
			},
			want: dcstatGlobalPublish{Ratio: 95},
		},
		"cumulative ratio does not use the larger interval miss fraction": {
			updates: []dcstatGlobalCounters{
				{Reference: 1000, Slow: 200, Miss: 50},
				{Reference: 1010, Slow: 260, Miss: 90},
			},
			want: dcstatGlobalPublish{Ratio: 91, Reference: 10, Slow: 60, Miss: 40},
		},
		"cumulative misses above references clamp the ratio to zero": {
			updates: []dcstatGlobalCounters{
				{Reference: 10, Miss: 20},
			},
			want: dcstatGlobalPublish{Reference: 10, Miss: 20},
		},
		"counter reset is clamped instead of going negative": {
			updates: []dcstatGlobalCounters{
				{Reference: 1000, Slow: 200, Miss: 50},
				{Reference: 10, Slow: 2, Miss: 1},
			},
			want: dcstatGlobalPublish{Ratio: 90},
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
	wantRatioHeader := "HOST ''\n\nCHART 'filesystem.dc_hit_ratio' '' 'Percentage of directory lookups resolved by the cache' '%' 'directory_cache' 'filesystem.dc_hit_ratio' 'line' '21200' '7' '' 'ebpf-go.plugin' 'dcstat'\n"
	if !strings.Contains(ratio, wantRatioHeader) {
		t.Fatalf("ratio chart = %q, want substring %q", ratio, wantRatioHeader)
	}
	if !strings.Contains(ratio, "DIMENSION 'ratio' 'ratio' 'absolute' '1' '1' ''\n") {
		t.Fatalf("ratio chart is missing its dimension: %q", ratio)
	}

	reference := formatDCStatGlobalChart(dcstatGlobalCharts[1], updateEvery)
	if !strings.Contains(reference, "'files'") {
		t.Fatalf("dc_reference chart = %q, want unit 'files'", reference)
	}
	for _, dim := range []string{"reference", "slow", "miss"} {
		want := "DIMENSION '" + dim + "' '" + dim + "' 'absolute' '1' '1' ''\n"
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

// TestDCStatGlobalStateUpdateIndependentRegression pins that diffCounters clamps
// each counter independently: one field regressing (a map reset for that key)
// must not suppress the deltas of the others.
func TestDCStatGlobalStateUpdateIndependentRegression(t *testing.T) {
	state := &dcstatGlobalState{}
	state.Update(dcstatGlobalCounters{Reference: 1000, Slow: 200, Miss: 50})

	// Reference regresses, Miss advances: count deltas remain independently
	// clamped, while the cumulative ratio follows the current raw counters.
	got, _ := state.Update(dcstatGlobalCounters{Reference: 500, Slow: 300, Miss: 60})
	if got.Ratio != 88 {
		t.Fatalf("ratio = %d, want 88 from current cumulative counters", got.Ratio)
	}
	if got.Reference != 0 || got.Slow != 100 || got.Miss != 10 {
		t.Fatalf("counters = %+v, want independently clamped interval deltas", got)
	}
}

// TestDCStatGlobalStateUpdateHugeCounters pins the clamp at the uint64->int64
// boundary: diffCounters must not produce a negative delta that would invert the
// ratio.
func TestDCStatGlobalStateUpdateHugeCounters(t *testing.T) {
	state := &dcstatGlobalState{}
	got, _ := state.Update(dcstatGlobalCounters{Reference: 1 << 62, Slow: 1 << 62, Miss: 0})
	if got.Ratio != 100 {
		t.Fatalf("ratio = %d, want 100 (no misses on a huge reference delta)", got.Ratio)
	}
}
