// SPDX-License-Identifier: GPL-3.0-or-later

package couchbase

type cbMetrics struct {
	// https://developer.couchbase.com/resources/best-practice-guides/monitoring-guide.pdf
	BucketsBasicStats []bucketsBasicStats
}

func (m cbMetrics) empty() bool {
	switch {
	case m.hasBucketsStats():
		return false
	}
	return true
}

func (m cbMetrics) hasBucketsStats() bool { return len(m.BucketsBasicStats) > 0 }

type bucketsBasicStats struct {
	Name string `json:"name"`

	BasicStats struct {
		DataUsed               float64 `json:"dataUsed"`
		DiskFetches            float64 `json:"diskFetches"`
		ItemCount              float64 `json:"itemCount"`
		DiskUsed               float64 `json:"diskUsed"`
		MemUsed                float64 `json:"memUsed"`
		OpsPerSec              float64 `json:"opsPerSec"`
		QuotaPercentUsed       float64 `json:"quotaPercentUsed"`
		VbActiveNumNonResident float64 `json:"vbActiveNumNonResident"`
	} `json:"basicStats"`
}
