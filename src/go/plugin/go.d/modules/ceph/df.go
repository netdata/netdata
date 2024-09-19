// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

type (
	DfStats struct {
		Stats        Stats        `json:"stats"`
		StatsByClass StatsByClass `json:"stats_by_class"`
		Pools        PoolList     `json:"pools"`
	}

	Stats struct {
		TotalBytes         int64   `json:"total_bytes"`
		TotalAvailBytes    int64   `json:"total_avail_bytes"`
		TotalUsedBytes     int64   `json:"total_used_bytes"`
		TotalUsedRawBytes  int64   `json:"total_used_raw_bytes"`
		TotalUsedRawRatio  float64 `json:"total_used_raw_ratio"`
		NumOsds            int64   `json:"num_osds"`
		NumPerPoolOsds     int64   `json:"num_per_pool_osds"`
		NumPerPoolOmapOsds int64   `json:"num_per_pool_omap_osds"`
	}

	StatsByClass map[string]interface{}

	PoolList []Pool

	Pool struct {
		Name  string    `json:"name"`
		ID    int64     `json:"id"`
		Stats PoolStats `json:"stats"`
	}

	PoolStats struct {
		Stored      int64   `json:"stored"`
		KbUsed      int64   `json:"kb_used"`
		Objects     int64   `json:"objects"`
		BytesUsed   int64   `json:"bytes_used"`
		PercentUsed float64 `json:"percent_used"`
		MaxAvail    int64   `json:"max_avail"`
	}
)
