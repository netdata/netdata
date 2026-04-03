// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioDefault   = collectorapi.Priority
	prioGORuntime = prioDefault + 10
)

func getChartFamily(metric string) (fam string) {
	if strings.HasPrefix(metric, "go_") {
		return "go"
	}
	if strings.HasPrefix(metric, "process_") {
		return "process"
	}
	if parts := strings.SplitN(metric, "_", 3); len(parts) < 3 {
		fam = metric
	} else {
		fam = parts[0] + "_" + parts[1]
	}

	i := len(fam) - 1
	for i >= 0 && fam[i] >= '0' && fam[i] <= '9' {
		i--
	}
	if i > 0 {
		return fam[:i+1]
	}
	return fam
}

func getChartUnits(metric string) string {
	idx := strings.LastIndexByte(metric, '_')
	if idx == -1 {
		if idx = strings.LastIndexFunc(metric, func(r rune) bool { return r >= 'A' && r <= 'Z' }); idx != -1 {
			v := strings.ToLower(metric[idx:])
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
	switch suffix := metric[idx:]; suffix {
	case "_total", "_sum", "_count", "_ratio":
		return getChartUnits(metric[:idx])
	}
	switch units := metric[idx+1:]; units {
	case "hertz":
		return "Hz"
	default:
		return units
	}
}

func getChartPriority(name string) int {
	if strings.HasPrefix(name, "go_") || strings.HasPrefix(name, "process_") {
		return prioGORuntime
	}
	return prioDefault
}
