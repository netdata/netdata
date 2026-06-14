// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	commonmodel "github.com/prometheus/common/model"
)

const (
	prioDefault   = collectorapi.Priority
	prioGORuntime = prioDefault + 10
)

// getChartTitle derives a chart title (description) from the metric HELP, falling back to
// the metric name when HELP is absent. It is fed into the metrix instrument meta so
// chartengine autogen reproduces the V1 chart title.
func getChartTitle(name, help string) string {
	if help == "" {
		return fmt.Sprintf("Metric \"%s\"", name)
	}

	help = strings.ReplaceAll(help, "'", "")
	help = strings.TrimSuffix(help, ".")

	return help
}

func getChartFamily(name string) (fam string) {
	if strings.HasPrefix(name, "go_") {
		return "go"
	}
	if strings.HasPrefix(name, "process_") {
		return "process"
	}
	if parts := strings.SplitN(name, "_", 3); len(parts) < 3 {
		fam = name
	} else {
		fam = parts[0] + "_" + parts[1]
	}

	// remove number suffix if any
	// load1, load5, load15 => load
	i := len(fam) - 1
	for i >= 0 && fam[i] >= '0' && fam[i] <= '9' {
		i--
	}
	if i > 0 {
		return fam[:i+1]
	}
	return fam
}

func getChartUnits(name string) string {
	// https://prometheus.io/docs/practices/naming/#metric-names
	// ...must have a single unit (i.e. do not mix seconds with milliseconds, or seconds with bytes).
	// ...should have a suffix describing the unit, in plural form.
	// Note that an accumulating count has total as a suffix, in addition to the unit if applicable

	idx := strings.LastIndexByte(name, '_')
	if idx == -1 {
		// snmp_exporter: e.g. ifOutUcastPkts, ifOutOctets.
		if idx = strings.LastIndexFunc(name, func(r rune) bool { return r >= 'A' && r <= 'Z' }); idx != -1 {
			v := strings.ToLower(name[idx:])
			switch v {
			case "pkts":
				return "packets"
			case "octets":
				return "bytes"
			case "mtu":
				return "octets"
			case "speed":
				return "bits"
			}
			return v
		}
		return "events"
	}
	switch suffix := name[idx:]; suffix {
	case "_total", "_sum", "_count", "_ratio":
		return getChartUnits(name[:idx])
	}
	switch units := name[idx+1:]; units {
	case "hertz":
		return "Hz"
	default:
		return units
	}
}

// instrumentUnit returns the chart unit metrix should carry for a family. V1 appends "/s" to summary
// quantile units (except seconds/time). chartengine autogen adds "/s" itself for the incremental
// counter/_sum routes but uses the unit as-is for the absolute summary-quantile route, so the writer
// must add it for summaries; gauges/counters/histograms pass the base unit.
func instrumentUnit(name string, typ commonmodel.MetricType) string {
	unit := getChartUnits(name)
	if typ == commonmodel.MetricTypeSummary {
		switch unit {
		case "seconds", "time":
		default:
			unit += "/s"
		}
	}
	return unit
}

func getChartPriority(name string) int {
	if strings.HasPrefix(name, "go_") || strings.HasPrefix(name, "process_") {
		return prioGORuntime
	}
	return prioDefault
}
