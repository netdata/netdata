// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"time"
)

const (
	entityAbsenceGrace  = time.Minute
	hdrAcceptVersionV11 = "application/vnd.ceph.api.v1.1+json"
)

type osdMetricSample struct {
	key         string
	deviceClass string
	name        string
	up          int64
	down        int64
	in          int64
	out         int64
	total       int64
	available   int64
	readOps     float64
	writeOps    float64
	readBytes   float64
	writeBytes  float64
	commitMS    float64
	applyMS     float64
}

func (c *Collector) collectOsds(ctx context.Context, mx map[string]int64) error {
	osds, err := c.fetchAllOSDs(ctx)
	if err != nil {
		return err
	}
	if err := validateOSDs(osds); err != nil {
		return err
	}

	selected := make([]osdMetricSample, 0, min(c.MaxOSDs, len(osds)))
	for _, osd := range osds {
		name := osd.Tree.Name
		if name == "" {
			name = fmt.Sprintf("osd.%d", osd.ID)
		}
		if !c.osdMatcher.MatchString(name) && !c.osdMatcher.MatchString(osd.UUID) {
			continue
		}
		selected = append(selected, osdSample(osd, name))
		if len(selected) > c.MaxOSDs {
			c.suppressEntityMetrics("osd", c.seenOsds)
			c.Limit("osd-cardinality", 1, time.Hour).Warningf(
				"selected OSD count exceeds max_osds=%d; no per-OSD metrics will be collected; narrow osd_selector or raise max_osds",
				c.MaxOSDs)
			return nil
		}
	}
	if err := validateOSDChartKeys(selected); err != nil {
		return err
	}

	now := c.now()
	seen := make(map[string]bool, len(selected))
	for _, sample := range selected {
		seen[sample.key] = true
	}
	deferred := c.retireCollidingEntityOwners("osd", c.seenOsds, seen)
	var chartErrs []error
	for _, sample := range selected {
		if deferred[sample.key] {
			continue
		}
		state, ok := c.seenOsds[sample.key]
		if !ok {
			if err := c.addOsdCharts(sample.key, sample.deviceClass, sample.name); err != nil {
				chartErrs = append(chartErrs, fmt.Errorf("add charts for OSD %q: %w", sample.key, err))
				continue
			}
			state = &entityState{}
			c.seenOsds[sample.key] = state
		}
		state.lastSeen = now
		emitOSDMetrics(mx, sample)
	}
	c.expireMissingEntities("osd", c.seenOsds, seen, now)
	return errors.Join(chartErrs...)
}

func (c *Collector) fetchAllOSDs(ctx context.Context) ([]apiOsdResponse, error) {
	ctx, cancel := c.apiClient.withOperationTimeout(ctx)
	defer cancel()

	query := url.Values{
		"offset": {"0"},
		"limit":  {"-1"},
		"sort":   {"+id"},
	}
	var osds []apiOsdResponse
	headers, err := c.apiClient.getJSONWithHeaders(ctx, "list OSDs", urlPathApiOsd, hdrAcceptVersionV11, query, &osds)
	if err != nil {
		if !isUnsupportedAPIVersionError(err) {
			return nil, err
		}
		// TODO: Remove the Pacific 16 and Quincy 17 whole-list fallback only
		// when the collector intentionally raises its minimum release to Reef 18.
		osds = nil
		if err := c.apiClient.getJSON(ctx, "list legacy OSDs", urlPathApiOsd, hdrAcceptVersion, nil, &osds); err != nil {
			return nil, err
		}
		return osds, nil
	}

	rawTotal := headers.Get("X-Total-Count")
	total, err := strconv.Atoi(rawTotal)
	if err != nil || total < 0 {
		return nil, fmt.Errorf("list OSDs returned invalid X-Total-Count")
	}
	if len(osds) != total {
		return nil, fmt.Errorf("list OSDs returned %d entries but advertised %d", len(osds), total)
	}
	return osds, nil
}

func validateOSDs(osds []apiOsdResponse) error {
	ids := make(map[int64]bool, len(osds))
	uuids := make(map[string]bool, len(osds))
	for _, osd := range osds {
		if osd.ID < 0 {
			return fmt.Errorf("list OSDs returned a negative OSD ID")
		}
		if osd.UUID == "" {
			return fmt.Errorf("list OSDs returned an empty OSD UUID")
		}
		if ids[osd.ID] {
			return fmt.Errorf("list OSDs returned a duplicate OSD ID")
		}
		if uuids[osd.UUID] {
			return fmt.Errorf("list OSDs returned a duplicate OSD UUID")
		}
		ids[osd.ID] = true
		uuids[osd.UUID] = true

		if osd.Up != 0 && osd.Up != 1 || osd.In != 0 && osd.In != 1 {
			return fmt.Errorf("list OSDs returned an invalid up/in state")
		}
		total := osd.OsdStats.Statfs.Total
		available := osd.OsdStats.Statfs.Available
		if total < 0 || available < 0 || available > total {
			return fmt.Errorf("list OSDs returned invalid capacity values")
		}
		values := []float64{
			osd.Stats.OpR, osd.Stats.OpW, osd.Stats.OpOutBytes, osd.Stats.OpInBytes,
			osd.OsdStats.PerfStat.CommitLatencyMs, osd.OsdStats.PerfStat.ApplyLatencyMs,
		}
		for _, value := range values {
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > maxScaledInt64Float {
				return fmt.Errorf("list OSDs returned invalid rate or latency values")
			}
		}
	}
	return nil
}

func validateOSDChartKeys(samples []osdMetricSample) error {
	seen := make(map[string]bool, len(samples))
	for _, sample := range samples {
		key := normalizedEntityChartKey("osd", sample.key)
		if seen[key] {
			return fmt.Errorf("selected OSD UUIDs collide after legacy chart-ID normalization")
		}
		seen[key] = true
	}
	return nil
}

func osdSample(osd apiOsdResponse, name string) osdMetricSample {
	sample := osdMetricSample{
		key:         osd.UUID,
		deviceClass: osd.Tree.DeviceClass,
		name:        name,
		total:       osd.OsdStats.Statfs.Total,
		available:   osd.OsdStats.Statfs.Available,
		readOps:     osd.Stats.OpR,
		writeOps:    osd.Stats.OpW,
		readBytes:   osd.Stats.OpOutBytes,
		writeBytes:  osd.Stats.OpInBytes,
		commitMS:    osd.OsdStats.PerfStat.CommitLatencyMs,
		applyMS:     osd.OsdStats.PerfStat.ApplyLatencyMs,
	}
	if osd.Up == 1 {
		sample.up = 1
	} else {
		sample.down = 1
	}
	if osd.In == 1 {
		sample.in = 1
	} else {
		sample.out = 1
	}
	return sample
}

func emitOSDMetrics(mx map[string]int64, sample osdMetricSample) {
	px := "osd_" + sample.key + "_"
	mx[px+"status_up"] = sample.up
	mx[px+"status_down"] = sample.down
	mx[px+"status_in"] = sample.in
	mx[px+"status_out"] = sample.out
	mx[px+"space_used_bytes"] = sample.total - sample.available
	mx[px+"space_avail_bytes"] = sample.available
	mx[px+"read_ops"] = int64(sample.readOps * precision)
	mx[px+"read_bytes"] = int64(sample.readBytes * precision)
	mx[px+"write_ops"] = int64(sample.writeOps * precision)
	mx[px+"written_bytes"] = int64(sample.writeBytes * precision)
	mx[px+"commit_latency_ms"] = int64(sample.commitMS * precision)
	mx[px+"apply_latency_ms"] = int64(sample.applyMS * precision)
}
