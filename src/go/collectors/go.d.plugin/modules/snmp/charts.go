// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"strings"

	"github.com/netdata/go.d.plugin/agent/module"
)

func newCharts(configs []ChartConfig) (*module.Charts, error) {
	charts := &module.Charts{}
	for _, cfg := range configs {
		if len(cfg.IndexRange) == 2 {
			cs, err := newChartsFromIndexRange(cfg)
			if err != nil {
				return nil, err
			}
			if err := charts.Add(*cs...); err != nil {
				return nil, err
			}
		} else {
			chart, err := newChart(cfg)
			if err != nil {
				return nil, err
			}
			if err = charts.Add(chart); err != nil {
				return nil, err
			}
		}
	}
	return charts, nil
}

func newChartsFromIndexRange(cfg ChartConfig) (*module.Charts, error) {
	var addPrio int
	charts := &module.Charts{}
	for i := cfg.IndexRange[0]; i <= cfg.IndexRange[1]; i++ {
		chart, err := newChartWithOIDIndex(i, cfg)
		if err != nil {
			return nil, err
		}
		chart.Priority += addPrio
		addPrio += 1
		if err = charts.Add(chart); err != nil {
			return nil, err
		}
	}
	return charts, nil
}

func newChartWithOIDIndex(oidIndex int, cfg ChartConfig) (*module.Chart, error) {
	chart, err := newChart(cfg)
	if err != nil {
		return nil, err
	}

	chart.ID = fmt.Sprintf("%s_%d", chart.ID, oidIndex)
	chart.Title = fmt.Sprintf("%s %d", chart.Title, oidIndex)
	for _, dim := range chart.Dims {
		dim.ID = fmt.Sprintf("%s.%d", dim.ID, oidIndex)
	}

	return chart, nil
}

func newChart(cfg ChartConfig) (*module.Chart, error) {
	chart := &module.Chart{
		ID:       cfg.ID,
		Title:    cfg.Title,
		Units:    cfg.Units,
		Fam:      cfg.Family,
		Ctx:      fmt.Sprintf("snmp.%s", cfg.ID),
		Type:     module.ChartType(cfg.Type),
		Priority: cfg.Priority,
	}

	if chart.Title == "" {
		chart.Title = "Untitled chart"
	}
	if chart.Units == "" {
		chart.Units = "num"
	}
	if chart.Priority < module.Priority {
		chart.Priority += module.Priority
	}

	seen := make(map[string]struct{})
	var a string
	for _, cfg := range cfg.Dimensions {
		if cfg.Algorithm != "" {
			seen[cfg.Algorithm] = struct{}{}
			a = cfg.Algorithm
		}
		dim := &module.Dim{
			ID:   strings.TrimPrefix(cfg.OID, "."),
			Name: cfg.Name,
			Algo: module.DimAlgo(cfg.Algorithm),
			Mul:  cfg.Multiplier,
			Div:  cfg.Divisor,
		}
		if err := chart.AddDim(dim); err != nil {
			return nil, err
		}
	}
	if len(seen) == 1 && a != "" && len(chart.Dims) > 1 {
		for _, d := range chart.Dims {
			if d.Algo == "" {
				d.Algo = module.DimAlgo(a)
			}
		}
	}

	return chart, nil
}
