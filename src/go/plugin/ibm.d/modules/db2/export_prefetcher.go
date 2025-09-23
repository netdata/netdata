package db2

import "github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/db2/contexts"

func (c *Collector) exportPrefetcherMetrics() {
	for pool, metrics := range c.prefetchers {
		if metrics == nil {
			continue
		}

		labels := contexts.PrefetcherLabels{
			Bufferpool: pool,
		}

		contexts.Prefetcher.PrefetchRatio.Set(c.State, labels, contexts.PrefetcherPrefetchRatioValues{
			Ratio: metrics.PrefetchRatio,
		})

		contexts.Prefetcher.CleanerRatio.Set(c.State, labels, contexts.PrefetcherCleanerRatioValues{
			Ratio: metrics.CleanerRatio,
		})

		contexts.Prefetcher.PhysicalReads.Set(c.State, labels, contexts.PrefetcherPhysicalReadsValues{
			Reads: metrics.PhysicalReads,
		})

		contexts.Prefetcher.AsyncReads.Set(c.State, labels, contexts.PrefetcherAsyncReadsValues{
			Reads: metrics.AsyncReads,
		})

		contexts.Prefetcher.WaitTime.Set(c.State, labels, contexts.PrefetcherWaitTimeValues{
			Wait_time: metrics.AvgWaitTime,
		})

		contexts.Prefetcher.UnreadPages.Set(c.State, labels, contexts.PrefetcherUnreadPagesValues{
			Unread: metrics.UnreadPages,
		})
	}
}
