// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"time"
)

const (
	osdPageSize         = 100
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
	latencyN    int64
	overflow    bool
}

func (c *Collector) collectOsds(ctx context.Context, mx map[string]int64) error {
	osds, err := c.fetchAllOSDs(ctx)
	if err != nil {
		return err
	}
	if err := validateOSDs(osds); err != nil {
		return err
	}

	sort.SliceStable(osds, func(i, j int) bool {
		if osds[i].ID != osds[j].ID {
			return osds[i].ID < osds[j].ID
		}
		return osds[i].UUID < osds[j].UUID
	})

	usedChartKeys := make(map[string]bool)
	for _, osd := range osds {
		name := osd.Tree.Name
		if name == "" {
			name = fmt.Sprintf("osd.%d", osd.ID)
		}
		if c.osdMatcher.MatchString(name) || c.osdMatcher.MatchString(osd.UUID) {
			usedChartKeys[cleanChartID("osd_"+osd.UUID+"_")] = true
		}
	}
	otherKey := "other"
	for suffix := 0; usedChartKeys[cleanChartID("osd_"+otherKey+"_")]; suffix++ {
		otherKey = fmt.Sprintf("other_overflow_%d", suffix+1)
	}

	selected := make([]osdMetricSample, 0, min(c.MaxOSDs, len(osds)))
	other := osdMetricSample{key: otherKey, name: "other", deviceClass: "mixed", overflow: true}
	for _, osd := range osds {
		name := osd.Tree.Name
		if name == "" {
			name = fmt.Sprintf("osd.%d", osd.ID)
		}
		if !c.osdMatcher.MatchString(name) && !c.osdMatcher.MatchString(osd.UUID) {
			continue
		}
		sample := osdSample(osd, name)
		if len(selected) < c.MaxOSDs {
			selected = append(selected, sample)
		} else {
			if err := aggregateOSDSample(&other, sample); err != nil {
				return err
			}
		}
	}
	if other.latencyN > 0 {
		selected = append(selected, other)
	}
	if err := validateOSDChartKeys(selected); err != nil {
		return err
	}

	now := c.now()
	seen := make(map[string]bool, len(selected))
	for _, sample := range selected {
		seen[sample.key] = true
		if _, ok := c.seenOsds[sample.key]; !ok {
			c.seenOsds[sample.key] = &entityState{}
			c.addOsdCharts(sample.key, sample.deviceClass, sample.name, !sample.overflow)
		}
		c.seenOsds[sample.key].lastSeen = now
		emitOSDMetrics(mx, sample)
	}
	c.expireMissingEntities("osd", c.seenOsds, seen, now)
	return nil
}

func (c *Collector) fetchAllOSDs(ctx context.Context) ([]apiOsdResponse, error) {
	var all []apiOsdResponse
	total := -1
	for offset := 0; ; offset += osdPageSize {
		query := url.Values{
			"offset": {strconv.Itoa(offset)},
			"limit":  {strconv.Itoa(osdPageSize)},
			"sort":   {"+id"},
		}
		var page []apiOsdResponse
		headers, err := c.apiClient.getJSONWithHeaders(ctx, "list OSDs", urlPathApiOsd, hdrAcceptVersionV11, query, &page)
		if err != nil {
			return nil, err
		}
		if len(page) > osdPageSize {
			return nil, fmt.Errorf("list OSDs returned a page larger than the requested limit")
		}
		if rawTotal := headers.Get("X-Total-Count"); rawTotal != "" {
			parsedTotal, err := strconv.Atoi(rawTotal)
			if err != nil || parsedTotal < 0 {
				return nil, fmt.Errorf("list OSDs returned invalid X-Total-Count")
			}
			if total >= 0 && parsedTotal != total {
				return nil, fmt.Errorf("list OSDs changed X-Total-Count between pages")
			}
			total = parsedTotal
		}
		if total >= 0 && len(all)+len(page) > total {
			return nil, fmt.Errorf("list OSDs returned more entries than X-Total-Count")
		}
		all = append(all, page...)
		if total >= 0 && len(all) == total {
			break
		}
		if len(page) < osdPageSize {
			if total >= 0 {
				return nil, fmt.Errorf("list OSDs returned a short page before the advertised total")
			}
			break
		}
	}
	return all, nil
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
		key := cleanChartID("osd_" + sample.key + "_")
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
		latencyN:    1,
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

func aggregateOSDSample(dst *osdMetricSample, src osdMetricSample) error {
	if src.total > math.MaxInt64-dst.total || src.available > math.MaxInt64-dst.available {
		return fmt.Errorf("aggregate OSD capacity exceeds the supported integer range")
	}
	dst.total += src.total
	dst.available += src.available

	if err := addOSDMetric(&dst.readOps, src.readOps, "read operations"); err != nil {
		return err
	}
	if err := addOSDMetric(&dst.writeOps, src.writeOps, "write operations"); err != nil {
		return err
	}
	if err := addOSDMetric(&dst.readBytes, src.readBytes, "read bytes"); err != nil {
		return err
	}
	if err := addOSDMetric(&dst.writeBytes, src.writeBytes, "write bytes"); err != nil {
		return err
	}

	dst.latencyN++
	dst.commitMS += (src.commitMS - dst.commitMS) / float64(dst.latencyN)
	dst.applyMS += (src.applyMS - dst.applyMS) / float64(dst.latencyN)
	return nil
}

func addOSDMetric(dst *float64, value float64, name string) error {
	if value > maxScaledInt64Float-*dst {
		return fmt.Errorf("aggregate OSD %s exceeds the supported numeric range", name)
	}
	*dst += value
	return nil
}

func emitOSDMetrics(mx map[string]int64, sample osdMetricSample) {
	px := "osd_" + sample.key + "_"
	if !sample.overflow {
		mx[px+"status_up"] = sample.up
		mx[px+"status_down"] = sample.down
		mx[px+"status_in"] = sample.in
		mx[px+"status_out"] = sample.out
	}
	mx[px+"space_used_bytes"] = sample.total - sample.available
	mx[px+"space_avail_bytes"] = sample.available
	mx[px+"read_ops"] = int64(sample.readOps * precision)
	mx[px+"read_bytes"] = int64(sample.readBytes * precision)
	mx[px+"write_ops"] = int64(sample.writeOps * precision)
	mx[px+"written_bytes"] = int64(sample.writeBytes * precision)
	mx[px+"commit_latency_ms"] = int64(sample.commitMS * precision)
	mx[px+"apply_latency_ms"] = int64(sample.applyMS * precision)
}
