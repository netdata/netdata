// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"strings"

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
			NumObjects          int64 `json:"num_objects"`
			NumObjectsDegraded  int64 `json:"num_objects_degraded"`
			NumObjectsMisplaced int64 `json:"num_objects_misplaced"`
			NumObjectsUnfound   int64 `json:"num_objects_unfound"`
		} `json:"object_stats"`
		Statuses  map[string]int64 `json:"statuses"`
		PgsPerOsd int64            `json:"pgs_per_osd"`
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
		Up   int64 `json:"up"`
		Down int64 `json:"down"`
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

	var resp apiHealthMinimalResponse

	if err := c.webClient().RequestJSON(req, &resp); err != nil {
		return err
	}

	for _, v := range []string{"health_err", "health_warn", "health_ok"} {
		mx[v] = 0
	}
	mx[strings.ToLower(resp.Health.Status)] = 1

	mx["hosts_num"] = resp.Hosts
	mx["monitors_num"] = int64(len(resp.MonStatus.MonMap.Mons))
	mx["osds_num"] = int64(len(resp.OsdMap.Osds))
	mx["pools_num"] = int64(len(resp.Pools))
	mx["iscsi_daemons_num"] = resp.IscsiDaemons.Up + resp.IscsiDaemons.Down
	mx["iscsi_daemons_up_num"] = resp.IscsiDaemons.Up
	mx["iscsi_daemons_down_num"] = resp.IscsiDaemons.Down

	df := resp.Df.Stats
	mx["raw_capacity_used_bytes"] = df.TotalBytes - df.TotalAvailBytes
	mx["raw_capacity_avail_bytes"] = df.TotalAvailBytes
	mx["raw_capacity_utilization"] = 0
	if resp.Df.Stats.TotalAvailBytes > 0 {
		mx["raw_capacity_utilization"] = int64(float64(df.TotalBytes-df.TotalAvailBytes) / float64(df.TotalAvailBytes) * 100 * precision)
	}

	objs := resp.PgInfo.ObjectStats
	mx["objects_num"] = objs.NumObjects
	mx["objects_healthy_num"] = objs.NumObjects - (objs.NumObjectsMisplaced + objs.NumObjectsDegraded + objs.NumObjectsUnfound)
	mx["objects_misplaced_num"] = objs.NumObjectsMisplaced
	mx["objects_degraded_num"] = objs.NumObjectsDegraded
	mx["objects_unfound_num"] = objs.NumObjectsUnfound
	mx["pgs_per_osd"] = resp.PgInfo.PgsPerOsd

	for _, v := range []string{"clean", "working", "warning", "unknown"} {
		mx["pg_status_"+v] = 0
	}
	mx["pgs_num"] = 0
	for k, v := range resp.PgInfo.Statuses {
		mx["pg_status_"+pgStatus(k)] = v
		mx["pgs_num"] += v
	}

	perf := resp.ClientPerf
	mx["client_perf_read_bytes_sec"] = int64(perf.ReadBytesSec)
	mx["client_perf_read_op_per_sec"] = int64(perf.ReadOpPerSec)
	mx["client_perf_write_bytes_sec"] = int64(perf.WriteBytesSec)
	mx["client_perf_write_op_per_sec"] = int64(perf.WriteOpPerSec)
	mx["client_perf_recovering_bytes_per_sec"] = int64(perf.RecoveringBytesPerSec)

	return nil
}

func pgStatus(status string) string {
	// 'status' is formated as 'status1+status2+...+statusN'

	states := map[string]string{
		"active":           "clean",
		"clean":            "clean",
		"activating":       "working",
		"backfill_wait":    "working",
		"backfilling":      "working",
		"creating":         "working",
		"deep":             "working",
		"degraded":         "working",
		"forced_backfill":  "working",
		"forced_recovery":  "working",
		"peering":          "working",
		"peered":           "working",
		"recovering":       "working",
		"recovery_wait":    "working",
		"repair":           "working",
		"scrubbing":        "working",
		"snaptrim":         "working",
		"snaptrim_wait":    "working",
		"backfill_toofull": "warning",
		"backfill_unfound": "warning",
		"down":             "warning",
		"incomplete":       "warning",
		"inconsistent":     "warning",
		"recovery_toofull": "warning",
		"recovery_unfound": "warning",
		"remapped":         "warning",
		"snaptrim_error":   "warning",
		"stale":            "warning",
		"undersized":       "warning",
	}

	parts := strings.Split(status, "+")

	var clean, working, warning, unknown int

	for _, part := range parts {
		cat, _ := states[part]
		switch cat {
		case "clean":
			clean++
		case "working":
			working++
		case "warning":
			warning++
		default:
			unknown++
		}
	}

	if warning > 0 {
		return "warning"
	}
	if unknown > 0 {
		return "unknown"
	}
	if working > 0 {
		return "working"
	}
	if clean > 0 {
		return "clean"
	}
	return "unknown"
}
