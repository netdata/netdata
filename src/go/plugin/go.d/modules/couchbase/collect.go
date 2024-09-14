// SPDX-License-Identifier: GPL-3.0-or-later

package couchbase

import (
	"fmt"
	"net/url"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const (
	urlPathBucketsStats = "/pools/default/buckets"

	precision = 1000
)

func (cb *Couchbase) collect() (map[string]int64, error) {
	ms, err := cb.scrapeCouchbase()
	if err != nil {
		return nil, fmt.Errorf("error on scraping couchbase: %v", err)
	}
	if ms.empty() {
		return nil, nil
	}

	collected := make(map[string]int64)
	cb.collectBasicStats(collected, ms)

	return collected, nil
}

func (cb *Couchbase) collectBasicStats(collected map[string]int64, ms *cbMetrics) {
	for _, b := range ms.BucketsBasicStats {

		if !cb.collectedBuckets[b.Name] {
			cb.collectedBuckets[b.Name] = true
			cb.addBucketToCharts(b.Name)
		}

		bs := b.BasicStats
		collected[indexDimID(b.Name, "quota_percent_used")] = int64(bs.QuotaPercentUsed * precision)
		collected[indexDimID(b.Name, "ops_per_sec")] = int64(bs.OpsPerSec * precision)
		collected[indexDimID(b.Name, "disk_fetches")] = int64(bs.DiskFetches)
		collected[indexDimID(b.Name, "item_count")] = int64(bs.ItemCount)
		collected[indexDimID(b.Name, "disk_used")] = int64(bs.DiskUsed)
		collected[indexDimID(b.Name, "data_used")] = int64(bs.DataUsed)
		collected[indexDimID(b.Name, "mem_used")] = int64(bs.MemUsed)
		collected[indexDimID(b.Name, "vb_active_num_non_resident")] = int64(bs.VbActiveNumNonResident)
	}
}

func (cb *Couchbase) addBucketToCharts(bucket string) {
	cb.addDimToChart(bucketQuotaPercentUsedChart.ID, &module.Dim{
		ID:   indexDimID(bucket, "quota_percent_used"),
		Name: bucket,
		Div:  precision,
	})

	cb.addDimToChart(bucketOpsPerSecChart.ID, &module.Dim{
		ID:   indexDimID(bucket, "ops_per_sec"),
		Name: bucket,
		Div:  precision,
	})

	cb.addDimToChart(bucketDiskFetchesChart.ID, &module.Dim{
		ID:   indexDimID(bucket, "disk_fetches"),
		Name: bucket,
	})

	cb.addDimToChart(bucketItemCountChart.ID, &module.Dim{
		ID:   indexDimID(bucket, "item_count"),
		Name: bucket,
	})

	cb.addDimToChart(bucketDiskUsedChart.ID, &module.Dim{
		ID:   indexDimID(bucket, "disk_used"),
		Name: bucket,
	})

	cb.addDimToChart(bucketDataUsedChart.ID, &module.Dim{
		ID:   indexDimID(bucket, "data_used"),
		Name: bucket,
	})

	cb.addDimToChart(bucketMemUsedChart.ID, &module.Dim{
		ID:   indexDimID(bucket, "mem_used"),
		Name: bucket,
	})

	cb.addDimToChart(bucketVBActiveNumNonResidentChart.ID, &module.Dim{
		ID:   indexDimID(bucket, "vb_active_num_non_resident"),
		Name: bucket,
	})
}

func (cb *Couchbase) addDimToChart(chartID string, dim *module.Dim) {
	chart := cb.Charts().Get(chartID)
	if chart == nil {
		cb.Warningf("error on adding '%s' dimension: can not find '%s' chart", dim.ID, chartID)
		return
	}
	if err := chart.AddDim(dim); err != nil {
		cb.Warning(err)
		return
	}
	chart.MarkNotCreated()
}

func (cb *Couchbase) scrapeCouchbase() (*cbMetrics, error) {
	req, err := web.NewHTTPRequestWithPath(cb.RequestConfig, urlPathBucketsStats)
	if err != nil {
		return nil, err
	}
	req.URL.RawQuery = url.Values{"skipMap": []string{"true"}}.Encode()

	ms := &cbMetrics{}
	if err := web.DoHTTP(cb.httpClient).RequestJSON(req, &ms.BucketsBasicStats); err != nil {
		return nil, err
	}

	return ms, nil
}

func indexDimID(name, metric string) string {
	return fmt.Sprintf("bucket_%s_%s", name, metric)
}
