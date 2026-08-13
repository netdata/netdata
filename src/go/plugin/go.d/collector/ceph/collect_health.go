// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

func (c *Collector) collectHealth(ctx context.Context, labels metrix.LabelSet) (healthCollectionResult, error) {
	var result healthCollectionResult
	var resp apiHealthMinimalResponse
	if err := c.apiClient.getJSON(ctx, "get minimal health", urlPathApiHealthMinimal, hdrAcceptVersion, nil, &resp); err != nil {
		return result, err
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
			observeEnum(c.metrics.clusterHealthStatus, strings.TrimPrefix(status, "health_"), labels)
			result.observed = true
		default:
			errs = append(errs, fmt.Errorf("health_status: unknown status %q", resp.Health.Status))
		}
	}

	if resp.Hosts == nil {
		missing("hosts", "hosts")
	} else if *resp.Hosts < 0 {
		errs = append(errs, errors.New("hosts: negative host count"))
	} else {
		c.metrics.clusterHosts.Observe(float64(*resp.Hosts), labels)
		result.observed = true
	}

	if resp.MonStatus == nil {
		missing("monitors", "mon_status")
	} else {
		c.metrics.clusterMonitors.Observe(float64(len(resp.MonStatus.MonMap.Mons)), labels)
		result.observed = true
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
			counts := map[string]int64{"up": 0, "down": 0, "in": 0, "out": 0}
			for _, osd := range resp.OsdMap.Osds {
				if osd.In == 1 {
					counts["in"]++
				} else {
					counts["out"]++
				}
				if osd.Up == 1 {
					counts["up"]++
				} else {
					counts["down"]++
				}
			}
			count := int64(len(resp.OsdMap.Osds))
			c.metrics.clusterOSDs.Observe(float64(count), labels)
			for _, status := range []string{"up", "down", "in", "out"} {
				c.metrics.clusterOSDsByStatus.Observe(float64(counts[status]), labels, c.metrics.statusLabels[status])
			}
			result.osds = metricCount{value: count, present: true}
			result.observed = true
		}
	}

	if resp.MgrMap == nil {
		missing("managers", "mgr_map")
	} else {
		active := 0
		if resp.MgrMap.ActiveName != "" {
			active = 1
		}
		c.metrics.clusterManagers.Observe(float64(active), labels, c.metrics.statusLabels["active"])
		c.metrics.clusterManagers.Observe(float64(len(resp.MgrMap.Standbys)), labels, c.metrics.statusLabels["standby"])
		result.observed = true
	}

	if resp.Rgw == nil {
		missing("object_gateways", "rgw")
	} else if *resp.Rgw < 0 {
		errs = append(errs, errors.New("object_gateways: negative gateway count"))
	} else {
		c.metrics.clusterObjectGateways.Observe(float64(*resp.Rgw), labels)
		result.observed = true
	}

	if resp.IscsiDaemons == nil {
		missing("iscsi_gateways", "iscsi_daemons")
	} else if resp.IscsiDaemons.Up < 0 || resp.IscsiDaemons.Down < 0 ||
		resp.IscsiDaemons.Down > math.MaxInt64-resp.IscsiDaemons.Up {
		errs = append(errs, errors.New("iscsi_gateways: invalid gateway counts"))
	} else {
		c.metrics.clusterISCSIGateways.Observe(float64(resp.IscsiDaemons.Up+resp.IscsiDaemons.Down), labels)
		c.metrics.clusterISCSIGatewaysByStatus.Observe(float64(resp.IscsiDaemons.Up), labels, c.metrics.statusLabels["up"])
		c.metrics.clusterISCSIGatewaysByStatus.Observe(float64(resp.IscsiDaemons.Down), labels, c.metrics.statusLabels["down"])
		result.observed = true
	}

	if resp.Df == nil {
		missing("capacity", "df")
	} else {
		df := resp.Df.Stats
		if df.TotalBytes < 0 || df.TotalUsedRawBytes < 0 || df.TotalAvailBytes < 0 ||
			df.TotalUsedRawBytes > df.TotalBytes || df.TotalAvailBytes > df.TotalBytes {
			errs = append(errs, errors.New("capacity: invalid capacity values"))
		} else {
			utilization := 0.0
			if df.TotalBytes > 0 {
				utilization = percent(df.TotalUsedRawBytes, df.TotalBytes)
			}
			c.metrics.clusterCapacityBytes.Observe(float64(df.TotalUsedRawBytes), labels, c.metrics.stateLabels["used"])
			c.metrics.clusterCapacityBytes.Observe(float64(df.TotalAvailBytes), labels, c.metrics.stateLabels["avail"])
			c.metrics.clusterCapacityUtilization.Observe(utilization, labels)
			result.observed = true
		}
	}

	if resp.PgInfo == nil {
		missing("objects", "pg_info")
	} else {
		if err := c.collectObjectHealth(resp.PgInfo.ObjectStats, labels); err != nil {
			errs = append(errs, err)
		} else {
			result.observed = true
		}
	}

	if resp.Pools == nil {
		missing("pools_summary", "pools")
	} else {
		count := int64(len(*resp.Pools))
		c.metrics.clusterPools.Observe(float64(count), labels)
		result.pools = metricCount{value: count, present: true}
		result.observed = true
	}

	if resp.PgInfo == nil {
		missing("pgs", "pg_info")
	} else {
		if err := validateNonnegativeValues(resp.PgInfo.PgsPerOsd); err != nil {
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
				c.metrics.clusterPGs.Observe(float64(total), labels)
				for _, category := range []string{"clean", "working", "warning", "unknown"} {
					c.metrics.clusterPGsByStatus.Observe(float64(counts[category]), labels, c.metrics.statusLabels[category])
				}
				c.metrics.clusterPGsPerOSD.Observe(resp.PgInfo.PgsPerOsd, labels)
				result.observed = true
			}
		}
	}

	if resp.ClientPerf == nil {
		missing("client_io", "client_perf")
		missing("recovery", "client_perf")
	} else {
		perf := resp.ClientPerf
		if err := validateNonnegativeValues(perf.ReadBytesSec, perf.ReadOpPerSec, perf.WriteBytesSec, perf.WriteOpPerSec); err != nil {
			errs = append(errs, fmt.Errorf("client_io: invalid performance value: %w", err))
		} else {
			c.metrics.clusterClientIOBytes.Observe(perf.ReadBytesSec, labels, c.metrics.directionLabels["read"])
			c.metrics.clusterClientIOBytes.Observe(perf.WriteBytesSec, labels, c.metrics.directionLabels["written"])
			c.metrics.clusterClientIOPS.Observe(perf.ReadOpPerSec, labels, c.metrics.directionLabels["read"])
			c.metrics.clusterClientIOPS.Observe(perf.WriteOpPerSec, labels, c.metrics.directionLabels["write"])
			result.observed = true
		}
		if err := validateNonnegativeValues(perf.RecoveringBytesPerSec); err != nil {
			errs = append(errs, fmt.Errorf("recovery: invalid performance value: %w", err))
		} else {
			c.metrics.clusterRecoveryBytes.Observe(perf.RecoveringBytesPerSec, labels)
			result.observed = true
		}
	}

	if resp.ScrubStatus == nil {
		missing("scrub_status", "scrub_status")
	} else {
		status := strings.ToLower(*resp.ScrubStatus)
		switch status {
		case "disabled", "active", "inactive":
			observeEnum(c.metrics.clusterScrubStatus, status, labels)
			result.observed = true
		default:
			errs = append(errs, fmt.Errorf("scrub_status: unknown status %q", *resp.ScrubStatus))
		}
	}

	return result, errors.Join(errs...)
}

func validateNonnegativeValues(values ...float64) error {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return errors.New("value is negative or non-finite")
		}
	}
	return nil
}

func (c *Collector) collectObjectHealth(stats struct {
	NumObjects          int64 `json:"num_objects"`
	NumObjectCopies     int64 `json:"num_object_copies"`
	NumObjectsDegraded  int64 `json:"num_objects_degraded"`
	NumObjectsMisplaced int64 `json:"num_objects_misplaced"`
	NumObjectsUnfound   int64 `json:"num_objects_unfound"`
}, labels metrix.LabelSet) error {
	if stats.NumObjects < 0 || stats.NumObjectCopies < 0 || stats.NumObjectsDegraded < 0 ||
		stats.NumObjectsMisplaced < 0 || stats.NumObjectsUnfound < 0 ||
		stats.NumObjectsDegraded > stats.NumObjectCopies ||
		stats.NumObjectsMisplaced > stats.NumObjectCopies ||
		stats.NumObjectsUnfound > stats.NumObjects {
		return errors.New("objects: invalid object/copy health counters")
	}
	c.metrics.clusterObjects.Observe(float64(stats.NumObjects), labels)

	degraded, misplaced := 0.0, 0.0
	if stats.NumObjectCopies > 0 {
		degraded = percent(stats.NumObjectsDegraded, stats.NumObjectCopies)
		misplaced = percent(stats.NumObjectsMisplaced, stats.NumObjectCopies)
	}
	unfound := 0.0
	if stats.NumObjects > 0 {
		unfound = percent(stats.NumObjectsUnfound, stats.NumObjects)
	}
	c.metrics.clusterObjectCopiesHealth.Observe(degraded, labels, c.metrics.stateLabels["degraded"])
	c.metrics.clusterObjectCopiesHealth.Observe(misplaced, labels, c.metrics.stateLabels["misplaced"])
	c.metrics.clusterObjectsUnfound.Observe(unfound, labels)
	return nil
}

func percent(value, total int64) float64 {
	return float64(value) / float64(total) * 100
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
