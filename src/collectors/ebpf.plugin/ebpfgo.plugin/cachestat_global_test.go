package main

import "testing"

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
			wantValid: false,
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
