package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
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
	updateEvery := 7
	got := formatCachestatGlobalChart(cachestatGlobalCharts[0], updateEvery)

	wantHeader := "HOST ''\n\nCHART 'mem.cachestat_ratio' '' 'Hit ratio' '%' 'page_cache' 'mem.cachestat_ratio' 'line' '21100' '7' '' 'ebpf-go.plugin' 'cachestat'\n"
	wantDimension := "DIMENSION 'ratio' 'ratio' 'absolute' '1' '1' ''\n"

	if !strings.Contains(got, wantHeader) {
		t.Fatalf("chart header = %q, want substring %q", got, wantHeader)
	}
	if !strings.Contains(got, wantDimension) {
		t.Fatalf("chart dimension = %q, want substring %q", got, wantDimension)
	}
}

func formatCachestatGlobalChart(chart cachestatGlobalChart, updateEvery int) string {
	var buf bytes.Buffer
	api := netdataapi.New(&buf)

	api.HOST("")
	emitCachestatGlobalChart(api, chart, updateEvery)

	return buf.String()
}
