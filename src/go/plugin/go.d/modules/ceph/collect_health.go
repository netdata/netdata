// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const (
	urlPathApiHealthMinimal = "/api/health/minimal"
)

type apiHealthMinimalResponse struct {
	Health struct {
		Status string `json:"status"`
	} `json:"health"`
	MonStatus struct {
		MonMap struct {
			Mons []any `json:"mons"`
		} `json:"mon_map"`
	} `json:"mon_status"`
	ScrubStatus string `json:"scrub_status"`
	OsdMap      struct {
		Osds []any `json:"osds"`
	} `json:"osd_map"`
	PgInfo struct {
		ObjectStats struct {
			NumObjects          int `json:"num_objects"`
			NumObjectsDegraded  int `json:"num_objects_degraded"`
			NumObjectsMisplaced int `json:"num_objects_misplaced"`
			NumObjectsUnfound   int `json:"num_objects_unfound"`
		} `json:"object_stats"`
		Statuses  map[string]int64 `json:"statuses"`
		PgsPerOsd int              `json:"pgs_per_osd"`
	} `json:"pg_info"`
	Pools []any `json:"pools"`
	Df    struct {
		Stats struct {
			TotalAvailBytes   int64 `json:"total_avail_bytes"`
			TotalBytes        int64 `json:"total_bytes"`
			TotalUsedRawBytes int64 `json:"total_used_raw_bytes"`
		} `json:"stats"`
	} `json:"df"`
	ClientPerf struct {
		ReadBytesSec          float64 `json:"read_bytes_sec"`
		ReadOpPerSec          float64 `json:"read_op_per_sec"`
		WriteBytesSec         float64 `json:"write_bytes_sec"`
		WriteOpPerSec         float64 `json:"write_op_per_sec"`
		RecoveringBytesPerSec float64 `json:"recovering_bytes_per_sec"`
	} `json:"client_perf"`
	Hosts        int64 `json:"hosts"`
	IscsiDaemons struct {
		Up   int `json:"up"`
		Down int `json:"down"`
	} `json:"iscsi_daemons"`
}

func (c *Ceph) collectHealth(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathApiHealthMinimal)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	var health apiHealthMinimalResponse

	if err := c.webClient().RequestJSON(req, &health); err != nil {
		return err
	}

	return nil
}
