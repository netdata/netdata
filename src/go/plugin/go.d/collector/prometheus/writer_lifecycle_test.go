// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMetricFamilyWriterHandleLifecycle pins the family-handle lifetime coupling to metrix's
// descriptor lifetime: handles are aged on the successful-commit clock to the descriptor window
// (expire+grace), so a name idle past the window is evicted and re-registers cleanly, a bogus
// handle from an aborted commit does not drift-skip, and a previously accepted handle survives an
// aborted re-touch.
func TestMetricFamilyWriterHandleLifecycle(t *testing.T) {
	t.Run("the writer couples its family-handle window to the metrix descriptor window", func(t *testing.T) {
		store := metrix.NewCollectorStore() // expire=10, grace=10 -> window=20
		w := newMetricFamilyWriter(store, metricFamilyWriterPolicy{}, logger.New())
		require.NotNil(t, w.retention, "a metrix store must expose DescriptorRetention")
		require.Equal(t, uint64(20), w.window)
	})

	t.Run("a name idle past the descriptor window is evicted and re-registers with a changed type", func(t *testing.T) {
		store := metrix.NewCollectorStore()
		w := newMetricFamilyWriter(store, metricFamilyWriterPolicy{}, logger.New())
		cc := cycle(t, store)
		window := int(store.(metrix.DescriptorRetention).DescriptorRetentionWindow())

		cc.BeginCycle()
		require.Equal(t, 1, w.writeMetricFamilies(scrape(t, "# TYPE foo gauge\nfoo 1\n")))
		require.NoError(t, cc.CommitCycleSuccess())

		// foo absent for longer than the descriptor window while the endpoint stays up (keepalive
		// keeps the writer reconciling); metrix evicts foo's gauge descriptor and the writer evicts
		// its family handle in lockstep.
		for range window + 2 {
			cc.BeginCycle()
			w.writeMetricFamilies(scrape(t, "# TYPE ka gauge\nka 1\n"))
			require.NoError(t, cc.CommitCycleSuccess())
		}
		require.NotContains(t, w.handles, "foo", "the family handle must be evicted after the descriptor window")

		// foo reappears as a counter; both the metrix descriptor and the writer handle are gone, so
		// it re-registers cleanly instead of being drift-skipped forever.
		cc.BeginCycle()
		assert.NotPanics(t, func() {
			assert.Equal(t, 1, w.writeMetricFamilies(scrape(t, "# TYPE foo counter\nfoo 5\n")),
				"a name idle past the window must re-register with its new type")
		})
		require.NoError(t, cc.CommitCycleSuccess())

		v, ok := store.Read(metrix.ReadRaw()).Value("foo", nil)
		require.True(t, ok, "the re-registered counter must be readable")
		require.Equal(t, metrix.SampleValue(5), v)
	})

	t.Run("a never-accepted handle from an aborted commit does not drift-skip the next scrape", func(t *testing.T) {
		store := metrix.NewCollectorStore()
		w := newMetricFamilyWriter(store, metricFamilyWriterPolicy{}, logger.New())
		cc := cycle(t, store)

		// foo scraped as a gauge but the cycle ABORTS: metrix never commits the descriptor and the
		// staged family handle was never accepted.
		cc.BeginCycle()
		w.writeMetricFamilies(scrape(t, "# TYPE foo gauge\nfoo 1\n"))
		cc.AbortCycle()

		// foo reappears as a counter; the bogus never-accepted gauge handle must be cleared, not used
		// to drift-skip the counter.
		cc.BeginCycle()
		assert.Equal(t, 1, w.writeMetricFamilies(scrape(t, "# TYPE foo counter\nfoo 5\n")),
			"a bogus handle from an aborted commit must not drift-skip the next scrape")
		require.NoError(t, cc.CommitCycleSuccess())
	})

	t.Run("a previously accepted handle survives an aborted re-touch", func(t *testing.T) {
		store := metrix.NewCollectorStore()
		w := newMetricFamilyWriter(store, metricFamilyWriterPolicy{}, logger.New())
		cc := cycle(t, store)

		cc.BeginCycle()
		require.Equal(t, 1, w.writeMetricFamilies(scrape(t, "# TYPE foo gauge\nfoo 1\n")))
		require.NoError(t, cc.CommitCycleSuccess())

		// foo re-scraped but the cycle aborts; foo was already accepted, so its handle must survive.
		cc.BeginCycle()
		w.writeMetricFamilies(scrape(t, "# TYPE foo gauge\nfoo 2\n"))
		cc.AbortCycle()

		cc.BeginCycle()
		assert.Equal(t, 1, w.writeMetricFamilies(scrape(t, "# TYPE foo gauge\nfoo 3\n")),
			"a previously accepted handle must survive an aborted re-touch")
		require.NoError(t, cc.CommitCycleSuccess())
		require.Contains(t, w.handles, "foo")
	})

	t.Run("a summary whose series all drift is not refreshed forever and re-adopts the new quantiles", func(t *testing.T) {
		store := metrix.NewCollectorStore()
		w := newMetricFamilyWriter(store, metricFamilyWriterPolicy{}, logger.New())
		cc := cycle(t, store)
		window := int(store.(metrix.DescriptorRetention).DescriptorRetentionWindow())

		cc.BeginCycle()
		require.Equal(t, 1, w.writeMetricFamilies(scrape(t,
			"# TYPE app_lat summary\napp_lat{quantile=\"0.5\"} 1\napp_lat_sum 1\napp_lat_count 1\n")))
		require.NoError(t, cc.CommitCycleSuccess())

		// app_lat now only ever exposes {0.5,0.9}: every series is schema-drift-skipped while the old
		// {0.5} descriptor is live. The bug was that staging on type-match kept the handle alive forever;
		// with the fix the zero-writing family ages out and re-adopts the new quantile set.
		drift := "# TYPE app_lat summary\napp_lat{quantile=\"0.5\"} 1\napp_lat{quantile=\"0.9\"} 2\napp_lat_sum 3\napp_lat_count 2\n"
		reAdopted := false
		for range window * 2 {
			cc.BeginCycle()
			if w.writeMetricFamilies(scrape(t, drift)) > 0 {
				reAdopted = true
			}
			require.NoError(t, cc.CommitCycleSuccess())
		}
		require.True(t, reAdopted, "a fully-drifting family must re-adopt its new schema after the window, not drift-skip forever")
		require.Contains(t, w.handles, "app_lat")
		require.Equal(t, []float64{0.5, 0.9}, w.handles["app_lat"].summaryQuantiles, "the handle now carries the new quantile set")
	})

	t.Run("a histogram whose series all drift is not refreshed forever and re-adopts the new bounds", func(t *testing.T) {
		store := metrix.NewCollectorStore()
		w := newMetricFamilyWriter(store, metricFamilyWriterPolicy{}, logger.New())
		cc := cycle(t, store)
		window := int(store.(metrix.DescriptorRetention).DescriptorRetentionWindow())

		cc.BeginCycle()
		require.Equal(t, 1, w.writeMetricFamilies(scrape(t,
			"# TYPE app_lat histogram\napp_lat_bucket{le=\"0.1\"} 1\napp_lat_bucket{le=\"0.5\"} 2\napp_lat_bucket{le=\"+Inf\"} 2\napp_lat_sum 1\napp_lat_count 2\n")))
		require.NoError(t, cc.CommitCycleSuccess())

		// app_lat now only ever exposes a different bucket schema: skipped while the old descriptor is
		// live, then aged out and re-adopted with the new bounds.
		drift := "# TYPE app_lat histogram\napp_lat_bucket{le=\"0.1\"} 1\napp_lat_bucket{le=\"0.5\"} 2\napp_lat_bucket{le=\"1\"} 3\napp_lat_bucket{le=\"+Inf\"} 3\napp_lat_sum 5\napp_lat_count 3\n"
		reAdopted := false
		for range window * 2 {
			cc.BeginCycle()
			if w.writeMetricFamilies(scrape(t, drift)) > 0 {
				reAdopted = true
			}
			require.NoError(t, cc.CommitCycleSuccess())
		}
		require.True(t, reAdopted, "a fully-drifting histogram must re-adopt its new schema after the window, not drift-skip forever")
		require.Contains(t, w.handles, "app_lat")
		require.Equal(t, []float64{0.1, 0.5, 1}, w.handles["app_lat"].histogramBounds, "the handle now carries the new bucket bounds")
	})

	t.Run("with series expiry disabled the window is unbounded and handles are never evicted", func(t *testing.T) {
		store := metrix.NewCollectorStore(metrix.WithExpireAfterSuccessCycles(0))
		w := newMetricFamilyWriter(store, metricFamilyWriterPolicy{}, logger.New())
		require.Equal(t, metrix.DescriptorRetentionUnbounded, w.window)
		cc := cycle(t, store)

		cc.BeginCycle()
		require.Equal(t, 1, w.writeMetricFamilies(scrape(t, "# TYPE foo gauge\nfoo 1\n")))
		require.NoError(t, cc.CommitCycleSuccess())

		for range 50 {
			cc.BeginCycle()
			w.writeMetricFamilies(scrape(t, "# TYPE ka gauge\nka 1\n"))
			require.NoError(t, cc.CommitCycleSuccess())
		}
		require.Contains(t, w.handles, "foo", "an unbounded window must never evict the family handle")
	})

	t.Run("at a tight window a drifting name is kept until the boundary then re-registers", func(t *testing.T) {
		store := metrix.NewCollectorStore(metrix.WithExpireAfterSuccessCycles(2), metrix.WithDescriptorGraceCycles(0))
		w := newMetricFamilyWriter(store, metricFamilyWriterPolicy{}, logger.New())
		require.Equal(t, uint64(2), w.window)
		cc := cycle(t, store)

		cc.BeginCycle()
		require.Equal(t, 1, w.writeMetricFamilies(scrape(t, "# TYPE foo gauge\nfoo 1\n")))
		require.NoError(t, cc.CommitCycleSuccess())

		// foo now drifts to a counter every cycle. While the gauge descriptor is within its window the
		// counter is drift-skipped (0); once the handle ages out at the boundary it re-registers (1),
		// then stays. This pins the exact >= window boundary, not just eventual re-adoption.
		drift := "# TYPE foo counter\nfoo 5\n"
		var writtenPerCycle []int
		for range 4 {
			cc.BeginCycle()
			writtenPerCycle = append(writtenPerCycle, w.writeMetricFamilies(scrape(t, drift)))
			require.NoError(t, cc.CommitCycleSuccess())
		}
		require.Equal(t, []int{0, 0, 1, 1}, writtenPerCycle,
			"drift-skipped within the window, re-registered at the boundary, then stable")
	})

	t.Run("a scalar family that becomes all-unwritable ages out instead of refreshing", func(t *testing.T) {
		store := metrix.NewCollectorStore()
		w := newMetricFamilyWriter(store, metricFamilyWriterPolicy{}, logger.New())
		cc := cycle(t, store)
		window := int(store.(metrix.DescriptorRetention).DescriptorRetentionWindow())

		cc.BeginCycle()
		require.Equal(t, 1, w.writeMetricFamilies(scrape(t, "# TYPE foo gauge\nfoo 1\n")))
		require.NoError(t, cc.CommitCycleSuccess())

		// foo keeps appearing but only ever as NaN, which is not a writable scalar: zero series write,
		// so the handle must not refresh and must age out (a keepalive family keeps the endpoint up).
		nan := "# TYPE foo gauge\nfoo NaN\n# TYPE ka gauge\nka 1\n"
		for range window + 2 {
			cc.BeginCycle()
			w.writeMetricFamilies(scrape(t, nan))
			require.NoError(t, cc.CommitCycleSuccess())
		}
		require.NotContains(t, w.handles, "foo", "an all-unwritable family must age out, not refresh forever")
	})

	t.Run("with no descriptor-retention accessor, family handles are kept (fallback)", func(t *testing.T) {
		// A CollectorStore that genuinely does not expose DescriptorRetention, so this exercises
		// newMetricFamilyWriter's real constructor fallback rather than mutating the writer after.
		real := metrix.NewCollectorStore()
		store := &noRetentionStore{CollectorStore: real}
		w := newMetricFamilyWriter(store, metricFamilyWriterPolicy{}, logger.New())
		require.Nil(t, w.retention, "constructor must not resolve the optional accessor")
		require.Equal(t, metrix.DescriptorRetentionUnbounded, w.window, "fallback window must be unbounded")

		cc := cycle(t, real) // controls the same core the wrapper's Write() delegates to

		cc.BeginCycle()
		require.Equal(t, 1, w.writeMetricFamilies(scrape(t, "# TYPE foo gauge\nfoo 1\n")))
		require.NoError(t, cc.CommitCycleSuccess())

		for range 50 {
			cc.BeginCycle()
			w.writeMetricFamilies(scrape(t, "# TYPE ka gauge\nka 1\n"))
			require.NoError(t, cc.CommitCycleSuccess())
		}
		require.Contains(t, w.handles, "foo", "with no accessor, reconcile must keep handles")
	})
}

// noRetentionStore is a metrix.CollectorStore that deliberately does NOT expose the optional
// DescriptorRetention accessor: embedding the interface promotes only Read/Write, so a type
// assertion to DescriptorRetention fails. It exercises the writer's no-accessor fallback path.
type noRetentionStore struct {
	metrix.CollectorStore
}
