// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"time"
)

const hdrAcceptVersionV11 = "application/vnd.ceph.api.v1.1+json"

type osdMetricSample struct {
	uuid        string
	deviceClass string
	name        string
	up          bool
	in          bool
	total       int64
	available   int64
	readOps     float64
	writeOps    float64
	readBytes   float64
	writeBytes  float64
	commitMS    float64
	applyMS     float64
}

func (c *Collector) collectOsds(ctx context.Context, fsid string) (bool, error) {
	osds, err := c.fetchAllOSDs(ctx)
	if err != nil {
		return false, err
	}
	if err := validateOSDs(osds); err != nil {
		return false, err
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
			c.Limit("osd-cardinality", 1, time.Hour).Warningf(
				"selected OSD count exceeds max_osds=%d; no per-OSD metrics will be collected; narrow osd_selector or raise max_osds",
				c.MaxOSDs)
			return false, nil
		}
	}
	for _, sample := range selected {
		c.metrics.writeOSD(fsid, sample)
	}
	return len(selected) > 0, nil
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
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
				return fmt.Errorf("list OSDs returned invalid rate or latency values")
			}
		}
	}
	return nil
}

func osdSample(osd apiOsdResponse, name string) osdMetricSample {
	sample := osdMetricSample{
		uuid:        osd.UUID,
		deviceClass: osd.Tree.DeviceClass,
		name:        name,
		up:          osd.Up == 1,
		in:          osd.In == 1,
		total:       osd.OsdStats.Statfs.Total,
		available:   osd.OsdStats.Statfs.Available,
		readOps:     osd.Stats.OpR,
		writeOps:    osd.Stats.OpW,
		readBytes:   osd.Stats.OpOutBytes,
		writeBytes:  osd.Stats.OpInBytes,
		commitMS:    osd.OsdStats.PerfStat.CommitLatencyMs,
		applyMS:     osd.OsdStats.PerfStat.ApplyLatencyMs,
	}
	return sample
}
