// SPDX-License-Identifier: GPL-3.0-or-later

package intelgpu

import (
	"encoding/json"
	"errors"
	"fmt"
)

type (
	gpuSummaryStats struct {
		Frequency struct {
			Actual float64 `json:"actual"`
		} `json:"frequency"`
		Power struct {
			GPU     float64 `json:"gpu"`
			Package float64 `json:"package"`
		} `json:"power"`
		Engines map[string]struct {
			Busy float64 `json:"busy"`
		} `json:"engines"`
	}
)

const precision = 100

func (c *Collector) collect() (map[string]int64, error) {
	if c.exec == nil {
		return nil, errors.New("collector not initialized")
	}

	stats, err := c.getGPUSummaryStats()
	if err != nil {
		return nil, err
	}

	mx := make(map[string]int64)

	mx["frequency_actual"] = int64(stats.Frequency.Actual * precision)
	mx["power_gpu"] = int64(stats.Power.GPU * precision)
	mx["power_package"] = int64(stats.Power.Package * precision)

	for name, es := range stats.Engines {
		if !c.engines[name] {
			c.addEngineCharts(name)
			c.engines[name] = true
		}

		key := fmt.Sprintf("engine_%s_busy", name)
		mx[key] = int64(es.Busy * precision)
	}

	return mx, nil
}
func (c *Collector) getGPUSummaryStats() (*gpuSummaryStats, error) {
	bs, err := c.exec.queryGPUSummaryJson()
	if err != nil {
		return nil, err
	}

	if len(bs) == 0 {
		return nil, errors.New("query returned empty response")
	}

	var stats gpuSummaryStats
	if err := json.Unmarshal(bs, &stats); err != nil {
		return nil, err
	}

	if len(stats.Engines) == 0 {
		return nil, errors.New("query returned unexpected response")
	}

	return &stats, nil
}
