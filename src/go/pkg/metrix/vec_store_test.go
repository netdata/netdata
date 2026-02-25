// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"math"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVecStoreScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"snapshot gauge vec writes labeled series": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				vec := s.Write().SnapshotMeter("apache").Vec("state").Gauge("workers")

				cc.BeginCycle()
				vec.WithLabelValues("busy").Observe(7)
				vec.WithLabelValues("idle").Observe(3)
				cc.CommitCycleSuccess()

				mustValue(t, s.Read(), "apache.workers", Labels{"state": "busy"}, 7)
				mustValue(t, s.Read(), "apache.workers", Labels{"state": "idle"}, 3)
			},
		},
		"snapshot vec scope shares label keys across metric families": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				vm := s.Write().SnapshotMeter("apache").Vec("state")
				workers := vm.Gauge("workers")
				requests := vm.Counter("requests_total")

				cc.BeginCycle()
				workers.WithLabelValues("busy").Observe(7)
				requests.WithLabelValues("200").ObserveTotal(11)
				cc.CommitCycleSuccess()

				mustValue(t, s.Read(), "apache.workers", Labels{"state": "busy"}, 7)
				mustValue(t, s.Read(), "apache.requests_total", Labels{"state": "200"}, 11)
			},
		},
		"snapshot counter vec merges meter labels and vec labels": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				sm := s.Write().SnapshotMeter("http").WithLabels(Label{Key: "instance", Value: "job1"})
				vec := sm.Vec("code").Counter("requests_total")

				cc.BeginCycle()
				vec.WithLabelValues("200").ObserveTotal(11)
				cc.CommitCycleSuccess()

				mustValue(t, s.Read(), "http.requests_total", Labels{"instance": "job1", "code": "200"}, 11)
			},
		},
		"stateful counter vec preserves baseline and delta semantics": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				vec := s.Write().StatefulMeter("runtime").Vec("queue").Counter("jobs_total")

				cc.BeginCycle()
				vec.WithLabelValues("default").Add(5)
				cc.CommitCycleSuccess()
				mustValue(t, s.Read(), "runtime.jobs_total", Labels{"queue": "default"}, 5)
				mustNoDelta(t, s.Read(), "runtime.jobs_total", Labels{"queue": "default"})

				cc.BeginCycle()
				vec.WithLabelValues("default").Add(2)
				cc.CommitCycleSuccess()
				mustValue(t, s.Read(), "runtime.jobs_total", Labels{"queue": "default"}, 7)
				mustDelta(t, s.Read(), "runtime.jobs_total", Labels{"queue": "default"}, 2)
			},
		},
		"vec scope validates label keys through family declaration": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				vm := s.Write().SnapshotMeter("svc").Vec("zone", "zone")

				expectPanic(t, func() {
					_ = vm.Gauge("load")
				})
			},
		},
		"vec returns cached handle for same label values": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				vec := s.Write().SnapshotMeter("svc").Vec("zone").Gauge("load")

				a := vec.WithLabelValues("a")
				b := vec.WithLabelValues("a")
				require.Same(t, a, b, "expected same cached handle for repeated label values")
			},
		},
		"vec handle cache remains race-safe under concurrent writes": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				vec := s.Write().SnapshotMeter("svc").Vec("zone").Gauge("load")

				const (
					workers    = 32
					iterations = 200
					distinct   = 64
				)

				cc.BeginCycle()
				var wg sync.WaitGroup
				wg.Add(workers)
				for worker := 0; worker < workers; worker++ {
					worker := worker
					go func() {
						defer wg.Done()
						for i := 0; i < iterations; i++ {
							label := strconv.Itoa((worker*iterations + i) % distinct)
							vec.WithLabelValues(label).Observe(SampleValue(i))
						}
					}()
				}
				wg.Wait()
				cc.CommitCycleSuccess()

				seen := make(map[string]struct{}, distinct)
				s.Read(ReadRaw()).ForEachByName("svc.load", func(labels LabelView, _ SampleValue) {
					value, ok := labels.Get("zone")
					require.True(t, ok, "expected zone label on vec series")
					seen[value] = struct{}{}
				})
				require.Len(t, seen, distinct, "expected one committed scalar series per distinct vec label value")
			},
		},
		"GetWithLabelValues validates label value count": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				vec := s.Write().SnapshotMeter("svc").Vec("zone", "role").Gauge("load")

				_, err := vec.GetWithLabelValues("only-one")
				require.ErrorIs(t, err, errVecLabelValueCount)
				expectPanic(t, func() {
					_ = vec.WithLabelValues("only-one")
				})
			},
		},
		"vec with no label keys accepts empty label values": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				vec := s.Write().SnapshotMeter("svc").Vec().Gauge("load")

				cc.BeginCycle()
				vec.WithLabelValues().Observe(9)
				cc.CommitCycleSuccess()

				mustValue(t, s.Read(), "svc.load", nil, 9)
			},
		},
		"vec constructor rejects invalid label keys": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").Vec("zone", "zone").Gauge("load")
				})
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").Vec("").Counter("requests_total")
				})
			},
		},
		"snapshot histogram vec writes and reads point": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				vec := s.Write().SnapshotMeter("mysql").Vec("database").Histogram(
					"query_seconds",
					WithHistogramBounds(0.1, 0.5),
				)

				cc.BeginCycle()
				vec.WithLabelValues("db1").ObservePoint(HistogramPoint{
					Count: 2,
					Sum:   0.4,
					Buckets: []BucketPoint{
						{UpperBound: 0.1, CumulativeCount: 1},
						{UpperBound: 0.5, CumulativeCount: 2},
					},
				})
				cc.CommitCycleSuccess()

				p, ok := s.Read().Histogram("mysql.query_seconds", Labels{"database": "db1"})
				require.True(t, ok, "expected histogram point")
				require.Equal(t, SampleValue(2), p.Count)
				require.Equal(t, SampleValue(0.4), p.Sum)
				require.Len(t, p.Buckets, 2)
			},
		},
		"stateful histogram vec observes cumulative samples": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				vec := s.Write().StatefulMeter("mysql").Vec("database").Histogram(
					"query_seconds",
					WithHistogramBounds(0.1, 0.5),
				)

				cc.BeginCycle()
				vec.WithLabelValues("db1").Observe(0.05)
				cc.CommitCycleSuccess()

				cc.BeginCycle()
				vec.WithLabelValues("db1").Observe(0.2)
				cc.CommitCycleSuccess()

				p, ok := s.Read().Histogram("mysql.query_seconds", Labels{"database": "db1"})
				require.True(t, ok, "expected histogram point")
				require.Equal(t, SampleValue(2), p.Count)
				require.LessOrEqual(t, math.Abs(float64(p.Sum-0.25)), 1e-9)
				require.Len(t, p.Buckets, 2)
			},
		},
		"snapshot summary vec writes and reads point": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				vec := s.Write().SnapshotMeter("mysql").Vec("database").Summary(
					"query_seconds",
					WithSummaryQuantiles(0.5),
				)

				cc.BeginCycle()
				vec.WithLabelValues("db1").ObservePoint(SummaryPoint{
					Count: 2,
					Sum:   0.4,
					Quantiles: []QuantilePoint{
						{Quantile: 0.5, Value: 0.2},
					},
				})
				cc.CommitCycleSuccess()

				p, ok := s.Read().Summary("mysql.query_seconds", Labels{"database": "db1"})
				require.True(t, ok, "expected summary point")
				require.Equal(t, SampleValue(2), p.Count)
				require.Equal(t, SampleValue(0.4), p.Sum)
				require.Len(t, p.Quantiles, 1)
			},
		},
		"stateful summary vec observes cumulative samples": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				vec := s.Write().StatefulMeter("mysql").Vec("database").Summary("query_seconds")

				cc.BeginCycle()
				vec.WithLabelValues("db1").Observe(0.1)
				cc.CommitCycleSuccess()

				cc.BeginCycle()
				vec.WithLabelValues("db1").Observe(0.2)
				cc.CommitCycleSuccess()

				p, ok := s.Read().Summary("mysql.query_seconds", Labels{"database": "db1"})
				require.True(t, ok, "expected summary point")
				require.Equal(t, SampleValue(2), p.Count)
				require.LessOrEqual(t, math.Abs(float64(p.Sum-0.3)), 1e-9)
			},
		},
		"snapshot stateset vec enable writes active state": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				vec := s.Write().SnapshotMeter("net").Vec("nic").StateSet(
					"link_state",
					WithStateSetStates("up", "down"),
					WithStateSetMode(ModeEnum),
				)

				cc.BeginCycle()
				vec.WithLabelValues("eth0").Enable("up")
				cc.CommitCycleSuccess()

				p, ok := s.Read().StateSet("net.link_state", Labels{"nic": "eth0"})
				require.True(t, ok, "expected stateset point")
				require.True(t, p.States["up"], "unexpected stateset: %#v", p.States)
				require.False(t, p.States["down"], "unexpected stateset: %#v", p.States)
			},
		},
		"stateful stateset vec observe writes full state": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				vec := s.Write().StatefulMeter("net").Vec("nic").StateSet(
					"link_state",
					WithStateSetStates("up", "down"),
					WithStateSetMode(ModeBitSet),
				)

				cc.BeginCycle()
				vec.WithLabelValues("eth0").ObserveStateSet(StateSetPoint{States: map[string]bool{"down": true}})
				cc.CommitCycleSuccess()

				p, ok := s.Read().StateSet("net.link_state", Labels{"nic": "eth0"})
				require.True(t, ok, "expected stateset point")
				require.False(t, p.States["up"], "unexpected stateset: %#v", p.States)
				require.True(t, p.States["down"], "unexpected stateset: %#v", p.States)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}
