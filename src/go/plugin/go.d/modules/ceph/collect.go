// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"encoding/json"
	"fmt"
)

var osdNameMap = make(map[int]string)
var seenPools, seenOsds = make(map[string]bool), make(map[string]bool)

func (c *Ceph) collect() (map[string]int64, error) {

	mx := make(map[string]int64)

	bs, err := c.exec.df()
	if err != nil {
		return nil, err
	}

	if err := c.parseDf(mx, bs); err != nil {
		return nil, err
	}

	bs, err = c.exec.osdPoolStats()
	if err != nil {
		return nil, err
	}

	if err := c.parseOsdPoolStats(mx, bs); err != nil {
		return nil, err
	}

	bs, err = c.exec.osdDf()
	if err != nil {
		return nil, err
	}

	if err := c.parseOsdDf(mx, bs); err != nil {
		return nil, err
	}

	bs, err = c.exec.osdPerf()
	if err != nil {
		return nil, err
	}

	if err := c.parseOsdPerf(mx, bs); err != nil {
		return nil, err
	}

	for pool := range seenPools {
		if !c.seenPools[pool] {
			c.seenPools[pool] = true
			c.addPoolCharts(pool)
		}
	}
	for pool := range c.seenPools {
		if !seenPools[pool] {
			delete(c.seenPools, pool)
			c.removePoolCharts(pool)
		}
	}

	for osd := range seenOsds {
		if !c.seenOsds[osd] {
			c.seenOsds[osd] = true
			c.addOsdCharts(osd)
		}
	}
	for osd := range c.seenOsds {
		if !seenOsds[osd] {
			delete(c.seenOsds, osd)
			c.removeOsdCharts(osd)
		}
	}

	return mx, nil
}

func (c *Ceph) parseDf(mx map[string]int64, bs []byte) error {

	var dfs DfStats

	if err := json.Unmarshal(bs, &dfs); err != nil {
		return fmt.Errorf("error unmarshalling JSON: %s", err)
	} else {

		totalAvailable := 0
		totalKbUsed := 0
		totalObjects := 0

		for _, pool := range dfs.Pools {

			seenPools[pool.Name] = true

			mx[fmt.Sprintf("%s_kb_used", pool.Name)] = pool.Stats.KbUsed
			mx[fmt.Sprintf("%s_objects", pool.Name)] = pool.Stats.Objects

			totalAvailable += int(pool.Stats.MaxAvail)
			totalKbUsed += int(pool.Stats.KbUsed)
			totalObjects += int(pool.Stats.Objects)

		}

		mx["general_available"] = int64(totalAvailable / 1024)
		mx["general_usage"] = int64(totalKbUsed)
		mx["general_objects"] = int64(totalObjects)

		return nil
	}
}

func (c *Ceph) parseOsdPoolStats(mx map[string]int64, bs []byte) error {

	var osdPoolStats []OsdPoolStats

	if err := json.Unmarshal(bs, &osdPoolStats); err != nil {
		return fmt.Errorf("error unmarshalling JSON: %s", err)
	} else {

		totalReadBytes := 0
		totalWriteBytes := 0
		totalReadOps := 0
		totalWriteOps := 0

		for _, pool := range osdPoolStats {

			seenPools[pool.PoolName] = true

			mx[fmt.Sprintf("%s_read_bytes", pool.PoolName)] = pool.ClientIORate.ReadBytesSec
			mx[fmt.Sprintf("%s_write_bytes", pool.PoolName)] = pool.ClientIORate.WriteBytesSec
			mx[fmt.Sprintf("%s_read_operations", pool.PoolName)] = pool.ClientIORate.ReadOpPerSec
			mx[fmt.Sprintf("%s_write_operations", pool.PoolName)] = pool.ClientIORate.WriteOpPerSec

			totalReadBytes += int(pool.ClientIORate.ReadBytesSec)
			totalWriteBytes += int(pool.ClientIORate.WriteBytesSec)
			totalReadOps += int(pool.ClientIORate.ReadOpPerSec)
			totalWriteOps += int(pool.ClientIORate.WriteOpPerSec)
		}

		mx["general_read_bytes"] = int64(totalReadBytes)
		mx["general_write_bytes"] = int64(totalWriteBytes)
		mx["general_read_operations"] = int64(totalReadOps)
		mx["general_write_operations"] = int64(totalWriteOps)
		return nil
	}
}

func (c *Ceph) parseOsdDf(mx map[string]int64, bs []byte) error {

	var osdDf OsdDf

	if err := json.Unmarshal(bs, &osdDf); err != nil {
		return fmt.Errorf("error unmarshalling JSON: %s", err)
	} else {

		for _, osd := range osdDf.Nodes {

			seenOsds[osd.Name] = true

			osdNameMap[int(osd.ID)] = osd.Name
			mx[fmt.Sprintf("%s_usage", osd.Name)] = osd.KBUsed
			mx[fmt.Sprintf("%s_size", osd.Name)] = osd.KB
		}
		return nil
	}
}

func (c *Ceph) parseOsdPerf(mx map[string]int64, bs []byte) error {

	var osdPerf OsdPerf

	if err := json.Unmarshal(bs, &osdPerf); err != nil {
		return fmt.Errorf("error unmarshalling JSON: %s", err)
	} else {

		totalApplyLatency := int64(0)
		totalCommitLatency := int64(0)

		for _, osd := range osdPerf.OsdStats.OsdPerfInfos {
			// This command does not return OSD name
			name := osdNameMap[int(osd.ID)]
			seenOsds[name] = true

			mx[fmt.Sprintf("%s_apply_latency", name)] = osd.PerfStats.ApplyLatencyMs
			mx[fmt.Sprintf("%s_commit_latency", name)] = osd.PerfStats.CommitLatencyMs

			totalApplyLatency += osd.PerfStats.ApplyLatencyMs
			totalCommitLatency += osd.PerfStats.CommitLatencyMs
		}

		mx["general_apply_latency"] = totalApplyLatency
		mx["general_commit_latency"] = totalCommitLatency
		return nil
	}
}
