// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"net/url"
)

// https://docs.ceph.com/en/reef/mgr/ceph_api/

const (
	urlPathApiAuth          = "/api/auth"
	urlPathApiAuthCheck     = "/api/auth/check"
	urlPathApiAuthLogout    = "/api/auth/logout"
	urlPathApiHealthMinimal = "/api/health/minimal"
	urlPathApiMonitor       = "/api/monitor"
	urlPathApiOsd           = "/api/osd"
	urlPathApiPool          = "/api/pool"
)

var (
	urlQueryApiPool = url.Values{"stats": {"true"}}.Encode()
)

const (
	hdrAcceptVersion   = "application/vnd.ceph.api.v1.0+json"
	hdrContentTypeJson = "application/json"
)

type apiHealthMinimalResponse struct {
	Health struct {
		Status string `json:"status"`
	} `json:"health"`
	MonStatus struct {
		MonMap struct {
			Mons []any `json:"mons"`
		} `json:"monmap"`
	} `json:"mon_status"`
	ScrubStatus string `json:"scrub_status"`
	OsdMap      struct {
		Osds []struct {
			In int64 `json:"in"`
			Up int64 `json:"up"`
		} `json:"osds"`
	} `json:"osd_map"`
	PgInfo struct {
		ObjectStats struct {
			NumObjects          int64 `json:"num_objects"`
			NumObjectsDegraded  int64 `json:"num_objects_degraded"`
			NumObjectsMisplaced int64 `json:"num_objects_misplaced"`
			NumObjectsUnfound   int64 `json:"num_objects_unfound"`
		} `json:"object_stats"`
		Statuses  map[string]int64 `json:"statuses"`
		PgsPerOsd float64          `json:"pgs_per_osd"`
	} `json:"pg_info"`
	Pools  []any `json:"pools"`
	MgrMap struct {
		ActiveName string `json:"active_name"`
		Standbys   []struct {
			Gid int `json:"gid"`
		} `json:"standbys"`
	} `json:"mgr_map"`
	Df struct {
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
	Rgw          int64 `json:"rgw"`
	IscsiDaemons struct {
		Up   int64 `json:"up"`
		Down int64 `json:"down"`
	} `json:"iscsi_daemons"`
}

type apiOsdResponse struct {
	UUID     string `json:"uuid"`
	ID       int64  `json:"id"`
	Up       int64  `json:"up"`
	In       int64  `json:"in"`
	OsdStats struct {
		Statfs struct {
			Total     int64 `json:"total"`
			Available int64 `json:"available"`
		} `json:"statfs"`
		PerfStat struct {
			CommitLatencyMs float64 `json:"commit_latency_ms"`
			ApplyLatencyMs  float64 `json:"apply_latency_ms"`
		} `json:"perf_stat"`
	} `json:"osd_stats"`
	Stats struct {
		OpW        float64 `json:"op_w"`
		OpInBytes  float64 `json:"op_in_bytes"`
		OpR        float64 `json:"op_r"`
		OpOutBytes float64 `json:"op_out_bytes"`
	} `json:"stats"`
	Tree struct {
		DeviceClass string `json:"device_class"`
		Type        string `json:"type"`
		Name        string `json:"name"`
	} `json:"tree"`
}

type apiPoolResponse struct {
	PoolName string `json:"pool_name"`
	Stats    struct {
		Stored       struct{ Latest float64 } `json:"stored"`
		Objects      struct{ Latest float64 } `json:"objects"`
		AvailRaw     struct{ Latest float64 } `json:"avail_raw"`
		BytesUsed    struct{ Latest float64 } `json:"bytes_used"`
		PercentUsed  struct{ Latest float64 } `json:"percent_used"`
		Reads        struct{ Latest float64 } `json:"rd"`
		ReadBytes    struct{ Latest float64 } `json:"rd_bytes"`
		Writes       struct{ Latest float64 } `json:"wr"`
		WrittenBytes struct{ Latest float64 } `json:"wr_bytes"`
	} `json:"stats"`
}
