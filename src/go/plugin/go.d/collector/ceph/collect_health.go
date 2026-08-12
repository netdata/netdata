// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
)

func (c *Collector) collectHealth(ctx context.Context, mx map[string]int64) error {
	var resp apiHealthMinimalResponse
	if err := c.apiClient.getJSON(ctx, "get minimal health", urlPathApiHealthMinimal, hdrAcceptVersion, nil, &resp); err != nil {
		return err
	}
	var errs []error
	missing := func(feature, section string) {
		errs = append(
			errs,
			fmt.Errorf(
				"%s: response section %q is unavailable (check Dashboard RBAC and feature support)",
				feature,
				section,
			),
		)
	}

	if resp.Health == nil {
		missing("health_status", "health")
	} else {
		status := strings.ToLower(resp.Health.Status)
		switch status {
		case "health_err", "health_warn", "health_ok":
			for _, key := range []string{"health_err", "health_warn", "health_ok"} {
				mx[key] = 0
			}
			mx[status] = 1
		default:
			errs = append(errs, fmt.Errorf("health_status: unknown status %q", resp.Health.Status))
		}
	}

	if resp.Hosts == nil {
		missing("hosts", "hosts")
	} else if *resp.Hosts < 0 {
		errs = append(errs, errors.New("hosts: negative host count"))
	} else {
		mx["hosts_num"] = *resp.Hosts
	}

	if resp.MonStatus == nil {
		missing("monitors", "mon_status")
	} else {
		mx["monitors_num"] = int64(len(resp.MonStatus.MonMap.Mons))
	}

	if resp.OsdMap == nil {
		missing("osds_summary", "osd_map")
	} else {
		valid := true
		for _, osd := range resp.OsdMap.Osds {
			if osd.In != 0 && osd.In != 1 || osd.Up != 0 && osd.Up != 1 {
				errs = append(errs, errors.New("osds_summary: invalid up/in state"))
				valid = false
				break
			}
		}
		if valid {
			mx["osds_num"] = int64(len(resp.OsdMap.Osds))
			for _, key := range []string{"up", "down", "in", "out"} {
				mx["osds_"+key+"_num"] = 0
			}
			for _, osd := range resp.OsdMap.Osds {
				if osd.In == 1 {
					mx["osds_in_num"]++
				} else {
					mx["osds_out_num"]++
				}
				if osd.Up == 1 {
					mx["osds_up_num"]++
				} else {
					mx["osds_down_num"]++
				}
			}
		}
	}

	if resp.MgrMap == nil {
		missing("managers", "mgr_map")
	} else {
		mx["mgr_active_num"] = 0
		if resp.MgrMap.ActiveName != "" {
			mx["mgr_active_num"] = 1
		}
		mx["mgr_standby_num"] = int64(len(resp.MgrMap.Standbys))
	}

	if resp.Rgw == nil {
		missing("object_gateways", "rgw")
	} else if *resp.Rgw < 0 {
		errs = append(errs, errors.New("object_gateways: negative gateway count"))
	} else {
		mx["rgw_num"] = *resp.Rgw
	}

	if resp.IscsiDaemons == nil {
		missing("iscsi_gateways", "iscsi_daemons")
	} else if resp.IscsiDaemons.Up < 0 || resp.IscsiDaemons.Down < 0 ||
		resp.IscsiDaemons.Down > math.MaxInt64-resp.IscsiDaemons.Up {
		errs = append(errs, errors.New("iscsi_gateways: invalid gateway counts"))
	} else {
		mx["iscsi_daemons_num"] = resp.IscsiDaemons.Up + resp.IscsiDaemons.Down
		mx["iscsi_daemons_up_num"] = resp.IscsiDaemons.Up
		mx["iscsi_daemons_down_num"] = resp.IscsiDaemons.Down
	}

	if resp.Df == nil {
		missing("capacity", "df")
	} else {
		df := resp.Df.Stats
		if df.TotalBytes < 0 || df.TotalUsedRawBytes < 0 || df.TotalAvailBytes < 0 ||
			df.TotalUsedRawBytes > df.TotalBytes || df.TotalAvailBytes > df.TotalBytes {
			errs = append(errs, errors.New("capacity: invalid capacity values"))
		} else if df.TotalBytes > 0 {
			mx["raw_capacity_used_bytes"] = df.TotalUsedRawBytes
			mx["raw_capacity_avail_bytes"] = df.TotalAvailBytes
			mx["raw_capacity_utilization"] = percent(df.TotalUsedRawBytes, df.TotalBytes)
		} else {
			mx["raw_capacity_used_bytes"] = 0
			mx["raw_capacity_avail_bytes"] = 0
			mx["raw_capacity_utilization"] = 0
		}
	}

	if resp.PgInfo == nil {
		missing("objects", "pg_info")
	} else {
		collectObjectHealth(resp.PgInfo.ObjectStats, mx, &errs)
	}

	if resp.Pools == nil {
		missing("pools_summary", "pools")
	} else {
		mx["pools_num"] = int64(len(*resp.Pools))
	}

	if resp.PgInfo == nil {
		missing("pgs", "pg_info")
	} else {
		pgsPerOsd, err := scaledNonnegative(resp.PgInfo.PgsPerOsd)
		if err != nil {
			errs = append(errs, fmt.Errorf("pgs: invalid PGs-per-OSD value: %w", err))
		} else {
			counts := make(map[string]int64, 4)
			var total int64
			valid := true
			for status, count := range resp.PgInfo.Statuses {
				if count < 0 || count > math.MaxInt64-total {
					errs = append(errs, errors.New("pgs: invalid PG state counts"))
					valid = false
					break
				}
				category := pgStatusCategory(status)
				if count > math.MaxInt64-counts[category] {
					errs = append(errs, errors.New("pgs: PG category count exceeds the supported integer range"))
					valid = false
					break
				}
				counts[category] += count
				total += count
			}
			if valid {
				mx["pgs_num"] = total
				for _, category := range []string{"clean", "working", "warning", "unknown"} {
					mx["pg_status_category_"+category] = counts[category]
				}
				mx["pgs_per_osd"] = pgsPerOsd
			}
		}
	}

	if resp.ClientPerf == nil {
		missing("client_io", "client_perf")
		missing("recovery", "client_perf")
	} else {
		perf := resp.ClientPerf
		values, err := scaledNonnegativeValues(perf.ReadBytesSec, perf.ReadOpPerSec, perf.WriteBytesSec, perf.WriteOpPerSec)
		if err != nil {
			errs = append(errs, fmt.Errorf("client_io: invalid performance value: %w", err))
		} else {
			mx["client_perf_read_bytes_sec"] = values[0]
			mx["client_perf_read_op_per_sec"] = values[1]
			mx["client_perf_write_bytes_sec"] = values[2]
			mx["client_perf_write_op_per_sec"] = values[3]
		}
		value, err := scaledNonnegative(perf.RecoveringBytesPerSec)
		if err != nil {
			errs = append(errs, fmt.Errorf("recovery: invalid performance value: %w", err))
		} else {
			mx["client_perf_recovering_bytes_per_sec"] = value
		}
	}

	if resp.ScrubStatus == nil {
		missing("scrub_status", "scrub_status")
	} else {
		status := strings.ToLower(*resp.ScrubStatus)
		switch status {
		case "disabled", "active", "inactive":
			for _, key := range []string{"disabled", "active", "inactive"} {
				mx["scrub_status_"+key] = 0
			}
			mx["scrub_status_"+status] = 1
		default:
			errs = append(errs, fmt.Errorf("scrub_status: unknown status %q", *resp.ScrubStatus))
		}
	}

	return errors.Join(errs...)
}

func scaledNonnegativeValues(values ...float64) ([]int64, error) {
	result := make([]int64, len(values))
	for i, value := range values {
		scaled, err := scaledNonnegative(value)
		if err != nil {
			return nil, err
		}
		result[i] = scaled
	}
	return result, nil
}

func scaledNonnegative(value float64) (int64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > maxScaledInt64Float {
		return 0, errors.New("value is negative, non-finite, or outside the supported range")
	}
	return int64(value * precision), nil
}

func collectObjectHealth(stats struct {
	NumObjects          int64 `json:"num_objects"`
	NumObjectCopies     int64 `json:"num_object_copies"`
	NumObjectsDegraded  int64 `json:"num_objects_degraded"`
	NumObjectsMisplaced int64 `json:"num_objects_misplaced"`
	NumObjectsUnfound   int64 `json:"num_objects_unfound"`
}, mx map[string]int64, errs *[]error) {
	if stats.NumObjects < 0 || stats.NumObjectCopies < 0 || stats.NumObjectsDegraded < 0 ||
		stats.NumObjectsMisplaced < 0 || stats.NumObjectsUnfound < 0 ||
		stats.NumObjectsDegraded > stats.NumObjectCopies ||
		stats.NumObjectsMisplaced > stats.NumObjectCopies ||
		stats.NumObjectsUnfound > stats.NumObjects {
		*errs = append(*errs, errors.New("objects: invalid object/copy health counters"))
		return
	}
	mx["objects_num"] = stats.NumObjects

	if stats.NumObjectCopies == 0 {
		mx["objects_degraded_ratio"] = 0
		mx["objects_misplaced_ratio"] = 0
	} else {
		mx["objects_degraded_ratio"] = percent(stats.NumObjectsDegraded, stats.NumObjectCopies)
		mx["objects_misplaced_ratio"] = percent(stats.NumObjectsMisplaced, stats.NumObjectCopies)
	}
	if stats.NumObjects == 0 {
		mx["objects_unfound_ratio"] = 0
	} else {
		mx["objects_unfound_ratio"] = percent(stats.NumObjectsUnfound, stats.NumObjects)
	}
}

func percent(value, total int64) int64 {
	return int64(float64(value) / float64(total) * 100 * precision)
}

func pgStatusCategory(status string) string {
	// Dashboard returns one compound state per PG, for example active+clean.
	states := strings.Split(status, "+")

	var clean, working, warning, unknown int
	for _, state := range states {
		switch state {
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
			"premerge",
			"recovering",
			"recovery_wait",
			"repair",
			"scrubbing",
			"snaptrim",
			"snaptrim_wait",
			"wait":
			working++
		case "backfill_toofull",
			"backfill_unfound",
			"down",
			"failed_repair",
			"incomplete",
			"inconsistent",
			"laggy",
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
