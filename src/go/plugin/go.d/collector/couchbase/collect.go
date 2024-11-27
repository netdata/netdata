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

func (c *Collector) collect() (map[string]int64, error) {
	ms, err := c.scrapeCouchbase()
	if err != nil {
		return nil, fmt.Errorf("error on scraping couchbase: %v", err)
	}
	if ms.empty() {
		return nil, nil
	}

	collected := make(map[string]int64)
	c.collectBasicStats(collected, ms)

	return collected, nil
}

func (c *Collector) collectBasicStats(collected map[string]int64, ms *cbMetrics) {
	for _, b := range ms.BucketsBasicStats {

		if !c.collectedBuckets[b.Name] {
			c.collectedBuckets[b.Name] = true
			c.addBucketToCharts(b.Name)
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

func (c *Collector) addBucketToCharts(bucket string) {
	c.addDimToChart(bucketQuotaPercentUsedChart.ID, &module.Dim{
		ID:   indexDimID(bucket, "quota_percent_used"),
		Name: bucket,
		Div:  precision,
	})

	c.addDimToChart(bucketOpsPerSecChart.ID, &module.Dim{
		ID:   indexDimID(bucket, "ops_per_sec"),
		Name: bucket,
		Div:  precision,
	})

	c.addDimToChart(bucketDiskFetchesChart.ID, &module.Dim{
		ID:   indexDimID(bucket, "disk_fetches"),
		Name: bucket,
	})

	c.addDimToChart(bucketItemCountChart.ID, &module.Dim{
		ID:   indexDimID(bucket, "item_count"),
		Name: bucket,
	})

	c.addDimToChart(bucketDiskUsedChart.ID, &module.Dim{
		ID:   indexDimID(bucket, "disk_used"),
		Name: bucket,
	})

	c.addDimToChart(bucketDataUsedChart.ID, &module.Dim{
		ID:   indexDimID(bucket, "data_used"),
		Name: bucket,
	})

	c.addDimToChart(bucketMemUsedChart.ID, &module.Dim{
		ID:   indexDimID(bucket, "mem_used"),
		Name: bucket,
	})

	c.addDimToChart(bucketVBActiveNumNonResidentChart.ID, &module.Dim{
		ID:   indexDimID(bucket, "vb_active_num_non_resident"),
		Name: bucket,
	})
}

func (c *Collector) addDimToChart(chartID string, dim *module.Dim) {
	chart := c.Charts().Get(chartID)
	if chart == nil {
		c.Warningf("error on adding '%s' dimension: can not find '%s' chart", dim.ID, chartID)
		return
	}
	if err := chart.AddDim(dim); err != nil {
		c.Warning(err)
		return
	}
	chart.MarkNotCreated()
}

func (c *Collector) scrapeCouchbase() (*cbMetrics, error) {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathBucketsStats)
	if err != nil {
		return nil, err
	}
	req.URL.RawQuery = url.Values{"skipMap": []string{"true"}}.Encode()

	ms := &cbMetrics{}
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &ms.BucketsBasicStats); err != nil {
		return nil, err
	}

	return ms, nil
}

func indexDimID(name, metric string) string {
	return fmt.Sprintf("bucket_%s_%s", name, metric)
}
