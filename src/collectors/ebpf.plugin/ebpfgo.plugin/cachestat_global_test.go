package main

import (
	"strings"
	"testing"
)

func TestCachestatGlobalStateUpdate(t *testing.T) {
	tests := map[string]struct {
		updates   []cachestatGlobalCounters
		want      cachestatGlobalPublish
		wantValid bool
	}{
		"initializes on first sample": {
			updates: []cachestatGlobalCounters{
				{
					MarkPageAccessed:   100,
					MarkBufferDirty:    20,
					AddToPageCacheLru:  80,
					AccountPageDirtied: 10,
				},
			},
			wantValid: true,
			want: cachestatGlobalPublish{
				Ratio: 12,
				Dirty: 20,
				Hit:   10,
				Miss:  70,
			},
		},
		"calculates deltas on second sample": {
			updates: []cachestatGlobalCounters{
				{
					MarkPageAccessed:   100,
					MarkBufferDirty:    20,
					AddToPageCacheLru:  80,
					AccountPageDirtied: 10,
				},
				{
					MarkPageAccessed:   130,
					MarkBufferDirty:    30,
					AddToPageCacheLru:  95,
					AccountPageDirtied: 20,
				},
			},
			wantValid: true,
			want: cachestatGlobalPublish{
				Ratio: 75,
				Dirty: 10,
				Hit:   15,
				Miss:  5,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			state := &cachestatGlobalState{}
			var got cachestatGlobalPublish
			var ok bool
			for _, update := range tc.updates {
				got, ok = state.Update(update)
			}

			if ok != tc.wantValid {
				t.Fatalf("Update valid = %v, want %v", ok, tc.wantValid)
			}
			if !ok {
				return
			}

			if got != tc.want {
				t.Fatalf("Update = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestFormatCachestatGlobalChart(t *testing.T) {
	got := formatCachestatGlobalChart(cachestatGlobalCharts[0], 10)

	if !strings.Contains(got, "CHART mem.cachestat_ratio") {
		t.Fatalf("chart header missing expected chart name: %q", got)
	}
	if !strings.Contains(got, "'ebpf-go.plugin' 'cachestat'") {
		t.Fatalf("chart header missing expected plugin/module names: %q", got)
	}
	if !strings.Contains(got, "DIMENSION percentage percentage absolute 1 1") {
		t.Fatalf("chart header missing expected dimension line: %q", got)
	}
}
