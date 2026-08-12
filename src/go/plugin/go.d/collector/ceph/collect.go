// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"context"
	"errors"
	"fmt"
	"math"
)

const precision = 1000

var (
	maxInt64Float       = math.Nextafter(float64(math.MaxInt64), 0)
	maxScaledInt64Float = math.Nextafter(float64(math.MaxInt64)/precision, 0)
)

func (c *Collector) collect(ctx context.Context) (map[string]int64, error) {
	mx := make(map[string]int64)

	var identityErr error
	if c.Metrics.DashboardAPIStatus {
		_, identityErr = c.probeClusterIdentity(ctx)
	} else {
		_, identityErr = c.ensureClusterIdentity(ctx)
	}
	if identityErr != nil {
		if c.clusterFSID() == "" {
			return nil, identityErr
		}
		if c.Metrics.DashboardAPIStatus {
			mx["dashboard_api_reachable"] = 0
			mx["dashboard_api_unreachable"] = 1
		}
		return mx, identityErr
	}

	if c.Metrics.DashboardAPIStatus {
		mx["dashboard_api_reachable"] = 1
		mx["dashboard_api_unreachable"] = 0
	}

	var errs []error
	if c.Metrics.anyHealthEnabled() {
		if err := c.collectHealth(ctx, mx); err != nil {
			errs = append(errs, fmt.Errorf("collect health features: %w", err))
		}
	}
	if c.Metrics.OSDs {
		if err := c.collectOsds(ctx, mx); err != nil {
			errs = append(errs, fmt.Errorf("collect OSD features: %w", err))
		}
	}
	if c.Metrics.Pools {
		if err := c.collectPools(ctx, mx); err != nil {
			errs = append(errs, fmt.Errorf("collect pool features: %w", err))
		}
	}

	return mx, errors.Join(errs...)
}

func (c *Collector) ensureClusterIdentity(ctx context.Context) (string, error) {
	if fsid := c.clusterFSID(); fsid != "" {
		return fsid, nil
	}
	return c.probeClusterIdentity(ctx)
}

func (c *Collector) probeClusterIdentity(ctx context.Context) (string, error) {
	var fsid string
	if err := c.apiClient.getJSON(ctx, "get cluster FSID", urlPathAPIClusterFSID, hdrAcceptVersion, nil, &fsid); err != nil {
		if !isUnsupportedEndpointError(err) {
			return "", fmt.Errorf("get cluster identity: %w", err)
		}

		// TODO: Remove the Pacific 16 and Quincy 17 monitor fallback only when
		// the collector intentionally raises its minimum release to Reef 18.
		var monitor struct {
			MonStatus struct {
				MonMap struct {
					FSID string `json:"fsid"`
				} `json:"monmap"`
			} `json:"mon_status"`
		}
		if err := c.apiClient.getJSON(ctx, "get legacy cluster FSID", urlPathApiMonitor, hdrAcceptVersion, nil, &monitor); err != nil {
			return "", fmt.Errorf("get cluster identity: %w", err)
		}
		fsid = monitor.MonStatus.MonMap.FSID
	}
	if fsid == "" {
		return "", errors.New("get cluster identity: empty FSID")
	}

	c.identityMu.Lock()
	switch {
	case c.fsid == "":
		c.fsid = fsid
	case c.fsid != fsid:
		c.identityMu.Unlock()
		return "", errors.New("cluster identity changed after active-MGR discovery")
	}
	c.identityMu.Unlock()

	if !c.FunctionOnly {
		c.addClusterChartsOnce.Do(c.addClusterCharts)
	}
	return fsid, nil
}
