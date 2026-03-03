// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCollectorStoreConcurrencyScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"single writer with concurrent readers is race-safe": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				g := s.Write().SnapshotMeter("collector").Gauge("load")

				const cycles = 200
				const readers = 8

				var writeDone atomic.Bool
				var writerWG sync.WaitGroup
				var readerWG sync.WaitGroup

				writerWG.Add(1)
				go func() {
					defer writerWG.Done()
					for i := 1; i <= cycles; i++ {
						cc.BeginCycle()
						g.Observe(SampleValue(i))
						cc.CommitCycleSuccess()
					}
					writeDone.Store(true)
				}()

				readerWG.Add(readers)
				for i := 0; i < readers; i++ {
					go func() {
						defer readerWG.Done()
						for !writeDone.Load() {
							r := s.Read()
							_ = r.CollectMeta()
							_, _ = r.Value("collector.load", nil)
							r.ForEachSeries(func(_ string, _ LabelView, _ SampleValue) {})
							r.ForEachSeriesIdentity(func(_ SeriesIdentity, _ SeriesMeta, _ string, _ LabelView, _ SampleValue) {})

							raw := s.Read(ReadRaw())
							_ = raw.CollectMeta()
							_, _ = raw.Value("collector.load", nil)
						}
					}()
				}

				writerWG.Wait()
				readerWG.Wait()

				meta := s.Read().CollectMeta()
				require.Equal(t, CollectStatusSuccess, meta.LastAttemptStatus, "unexpected collect metadata: %#v", meta)
				require.Equal(t, uint64(cycles), meta.LastAttemptSeq, "unexpected collect metadata: %#v", meta)
				require.Equal(t, uint64(cycles), meta.LastSuccessSeq, "unexpected collect metadata: %#v", meta)
				mustValue(t, s.Read(), "collector.load", nil, SampleValue(cycles))
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}
