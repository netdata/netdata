// SPDX-License-Identifier: GPL-3.0-or-later

package solr

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type count struct {
	Count int64
}

type common struct {
	Count        int64
	MeanRate     float64 `json:"meanRate"`
	MinRate1min  float64 `json:"1minRate"`
	MinRate5min  float64 `json:"5minRate"`
	MinRate15min float64 `json:"15minRate"`
}

type requestTimes struct {
	Count        int64
	MeanRate     float64 `json:"meanRate"`
	MinRate1min  float64 `json:"1minRate"`
	MinRate5min  float64 `json:"5minRate"`
	MinRate15min float64 `json:"15minRate"`
	MinMS        float64 `json:"min_ms"`
	MaxMS        float64 `json:"max_ms"`
	MeanMS       float64 `json:"mean_ms"`
	MedianMS     float64 `json:"median_ms"`
	StdDevMS     float64 `json:"stddev_ms"`
	P75MS        float64 `json:"p75_ms"`
	P95MS        float64 `json:"p95_ms"`
	P99MS        float64 `json:"p99_ms"`
	P999MS       float64 `json:"p999_ms"`
}

type coresMetrics struct {
	Metrics map[string]map[string]json.RawMessage
}

func (s *Solr) parse(resp *http.Response) (map[string]int64, error) {
	var cm coresMetrics
	var metrics = make(map[string]int64)

	if err := json.NewDecoder(resp.Body).Decode(&cm); err != nil {
		return nil, err
	}

	if len(cm.Metrics) == 0 {
		return nil, errors.New("unparsable data")
	}

	for core, data := range cm.Metrics {
		coreName := core[10:]

		if !s.cores[coreName] {
			s.addCoreCharts(coreName)
			s.cores[coreName] = true
		}

		if err := s.parseCore(coreName, data, metrics); err != nil {
			return nil, err
		}
	}

	return metrics, nil
}

func (s *Solr) parseCore(core string, data map[string]json.RawMessage, metrics map[string]int64) error {
	var (
		simpleCount  int64
		count        count
		common       common
		requestTimes requestTimes
	)

	for metric, stats := range data {
		parts := strings.Split(metric, ".")

		if len(parts) != 3 {
			continue
		}

		typ, handler, stat := strings.ToLower(parts[0]), parts[1], parts[2]

		if handler == "updateHandler" {
			// TODO:
			continue
		}

		switch stat {
		case "clientErrors", "errors", "serverErrors", "timeouts":
			if err := json.Unmarshal(stats, &common); err != nil {
				return err
			}
			metrics[format("%s_%s_%s_count", core, typ, stat)] += common.Count
		case "requests", "totalTime":
			var c int64
			if s.version < 7.0 {
				if err := json.Unmarshal(stats, &count); err != nil {
					return err
				}
				c = count.Count
			} else {
				if err := json.Unmarshal(stats, &simpleCount); err != nil {
					return err
				}
				c = simpleCount
			}
			metrics[format("%s_%s_%s_count", core, typ, stat)] += c
		case "requestTimes":
			if err := json.Unmarshal(stats, &requestTimes); err != nil {
				return err
			}
			metrics[format("%s_%s_%s_count", core, typ, stat)] += requestTimes.Count
			metrics[format("%s_%s_%s_min_ms", core, typ, stat)] += int64(requestTimes.MinMS * 1e6)
			metrics[format("%s_%s_%s_mean_ms", core, typ, stat)] += int64(requestTimes.MeanMS * 1e6)
			metrics[format("%s_%s_%s_median_ms", core, typ, stat)] += int64(requestTimes.MedianMS * 1e6)
			metrics[format("%s_%s_%s_max_ms", core, typ, stat)] += int64(requestTimes.MaxMS * 1e6)
			metrics[format("%s_%s_%s_p75_ms", core, typ, stat)] += int64(requestTimes.P75MS * 1e6)
			metrics[format("%s_%s_%s_p95_ms", core, typ, stat)] += int64(requestTimes.P95MS * 1e6)
			metrics[format("%s_%s_%s_p99_ms", core, typ, stat)] += int64(requestTimes.P99MS * 1e6)
			metrics[format("%s_%s_%s_p999_ms", core, typ, stat)] += int64(requestTimes.P999MS * 1e6)
		}
	}

	return nil
}

func (s *Solr) addCoreCharts(core string) {
	charts := charts.Copy()

	for _, chart := range *charts {
		chart.ID = format("%s_%s", core, chart.ID)
		chart.Fam = format("core %s", core)

		for _, dim := range chart.Dims {
			dim.ID = format("%s_%s", core, dim.ID)
		}
	}

	_ = s.charts.Add(*charts...)

}

var format = fmt.Sprintf
