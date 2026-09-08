// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"context"
	"io"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

// BenchmarkV1HostCollection measures the real collection/render/write path. Its
// per-cycle cost is independent of other jobs; frame admission uses the production owner.
func BenchmarkV1HostCollection(b *testing.B) {
	for name, tc := range map[string]struct{ vnode bool }{"global": {}, "vnode": {vnode: true}} {
		b.Run(name, func(b *testing.B) {
			charts := collectorapi.Charts{
				&collectorapi.Chart{
					ID:    "work",
					Title: "Work",
					Units: "units",
					Dims:  collectorapi.Dims{{ID: "value"}},
				},
			}
			module := &collectorapi.MockCollectorV1{
				ChartsFunc:  func() *collectorapi.Charts { return &charts },
				CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"value": 1} },
			}
			frames, err := lifecycle.NewFrameOwner(io.Discard)
			if err != nil {
				b.Fatal(err)
			}
			cfg := JobConfig{
				PluginName: "go.d",
				Name:       "device",
				ModuleName: "test",
				FullName:   "test_device",
				Module:     module,
				Out: v1BenchmarkFrameWriter{
					frames: frames,
				},
			}
			if tc.vnode {
				cfg.Vnode = vnodes.VirtualNode{
					GUID:     "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
					Hostname: "device",
					Labels:   map[string]string{"site": "athens", "vendor": "cisco"},
				}
			}
			job := NewJob(cfg)
			if err := job.AutoDetectionManaged(context.Background()); err != nil {
				b.Fatal(err)
			}
			job.runOnce()
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				job.runOnce()
			}
			b.StopTimer()
			job.Cleanup()
		})
	}
}

type v1BenchmarkFrameWriter struct{ frames *lifecycle.FrameOwner }

func (w v1BenchmarkFrameWriter) Write(payload []byte) (int, error) {
	if err := w.frames.CommitBorrowedProtocolFrame(payload); err != nil {
		return 0, err
	}
	return len(payload), nil
}

func (w v1BenchmarkFrameWriter) CommitBuiltJobOutput(build func() ([]byte, error), state OutputStateTransaction) error {
	return w.frames.CommitBuiltProtocolTransaction(build, state)
}
