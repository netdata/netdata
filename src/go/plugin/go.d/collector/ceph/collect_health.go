// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func (c *Collector) collectHealth(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathApiHealthMinimal)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", hdrAcceptVersion)
	req.Header.Set("Content-Type", hdrContentTypeJson)
	req.Header.Set("Authorization", "Bearer "+c.token)

	var resp apiHealthMinimalResponse

	if err := c.webClient().RequestJSON(req, &resp); err != nil {
		return err
	}

	for _, v := range []string{"health_err", "health_warn", "health_ok"} {
		mx[v] = 0
	}
	mx[strings.ToLower(resp.Health.Status)] = 1

	mx["mgr_active_num"] = 0
	if resp.MgrMap.ActiveName != "" {
		mx["mgr_active_num"] = 1
	}
	mx["mgr_standby_num"] = int64(len(resp.MgrMap.Standbys))
	mx["hosts_num"] = resp.Hosts
	mx["rgw_num"] = resp.Rgw
	mx["monitors_num"] = int64(len(resp.MonStatus.MonMap.Mons))
	mx["osds_num"] = int64(len(resp.OsdMap.Osds))

	for _, v := range []string{"up", "down", "in", "out"} {
		mx["osds_"+v+"_num"] = 0
	}
	for _, v := range resp.OsdMap.Osds {
		s := map[int64]string{0: "out", 1: "in"}
		mx["osds_"+s[v.In]+"_num"]++

		s = map[int64]string{0: "down", 1: "up"}
		mx["osds_"+s[v.Up]+"_num"]++
	}

	mx["pools_num"] = int64(len(resp.Pools))
	mx["iscsi_daemons_num"] = resp.IscsiDaemons.Up + resp.IscsiDaemons.Down
	mx["iscsi_daemons_up_num"] = resp.IscsiDaemons.Up
	mx["iscsi_daemons_down_num"] = resp.IscsiDaemons.Down

	df := resp.Df.Stats
	mx["raw_capacity_used_bytes"] = df.TotalBytes - df.TotalAvailBytes
	mx["raw_capacity_avail_bytes"] = df.TotalAvailBytes
	mx["raw_capacity_utilization"] = 0
	if df.TotalAvailBytes > 0 {
		mx["raw_capacity_utilization"] = int64(float64(df.TotalBytes-df.TotalAvailBytes) / float64(df.TotalBytes) * 100 * precision)
	}

	objs := resp.PgInfo.ObjectStats
	mx["objects_num"] = objs.NumObjects
	mx["objects_healthy_num"] = objs.NumObjects - (objs.NumObjectsMisplaced + objs.NumObjectsDegraded + objs.NumObjectsUnfound)
	mx["objects_misplaced_num"] = objs.NumObjectsMisplaced
	mx["objects_degraded_num"] = objs.NumObjectsDegraded
	mx["objects_unfound_num"] = objs.NumObjectsUnfound
	mx["pgs_per_osd"] = int64(resp.PgInfo.PgsPerOsd)

	mx["pgs_num"] = 0
	for _, v := range []string{"clean", "working", "warning", "unknown"} {
		mx["pg_status_category_"+v] = 0
	}
	for k, v := range resp.PgInfo.Statuses {
		mx["pg_status_category_"+pgStatusCategory(k)] += v
		mx["pgs_num"] += v
	}

	perf := resp.ClientPerf
	mx["client_perf_read_bytes_sec"] = int64(perf.ReadBytesSec)
	mx["client_perf_read_op_per_sec"] = int64(perf.ReadOpPerSec)
	mx["client_perf_write_bytes_sec"] = int64(perf.WriteBytesSec)
	mx["client_perf_write_op_per_sec"] = int64(perf.WriteOpPerSec)
	mx["client_perf_recovering_bytes_per_sec"] = int64(perf.RecoveringBytesPerSec)

	for _, v := range []string{"disabled", "active", "inactive"} {
		mx["scrub_status_"+v] = 0
	}
	mx["scrub_status_"+strings.ToLower(resp.ScrubStatus)] = 1

	return nil
}

func pgStatusCategory(status string) string {
	// 'status' is formated as 'status1+status2+...+statusN'

	states := strings.Split(status, "+")

	var clean, working, warning, unknown int

	for _, s := range states {
		switch s {
		case "active", "clean":
			clean++
		case "activating",
			"backfill_wait",
			"backfilling",
			"creating",
			"deep",
			"degraded",
			"forced_backfill",
			"forced_recovery",
			"peering",
			"peered",
			"recovering",
			"recovery_wait",
			"repair",
			"scrubbing",
			"snaptrim",
			"snaptrim_wait":
			working++
		case "backfill_toofull",
			"backfill_unfound",
			"down",
			"incomplete",
			"inconsistent",
			"recovery_toofull",
			"recovery_unfound",
			"remapped",
			"snaptrim_error",
			"stale",
			"undersized":
			warning++
		default:
			unknown++
		}
	}

	switch {
	case warning > 0:
		return "warning"
	case unknown > 0:
		return "unknown"
	case working > 0:
		return "working"
	case clean > 0:
		return "clean"
	default:
		return "unknown"
	}
}
