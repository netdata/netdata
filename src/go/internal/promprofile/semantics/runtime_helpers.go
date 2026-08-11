// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
	promselector "github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
)

func exactRouteMetricName(expression string) (string, bool) {
	compiled, err := promselector.ParseCompiled(expression)
	if err != nil {
		return "", false
	}
	meta := compiled.Meta()
	if len(meta.MetricNames) != 1 || len(meta.ConstrainedLabelKeys) != 0 {
		return "", false
	}
	name := meta.MetricNames[0]
	return name, strings.TrimSpace(expression) == name
}

func structuralLabelForComponent(component string) string {
	switch component {
	case "histogram_bucket":
		return "le"
	case "summary_quantile":
		return "quantile"
	default:
		return ""
	}
}

func semanticLabelMap(values []promreplay.SemanticLabel) map[string]string {
	out := make(map[string]string, len(values))
	for _, label := range values {
		out[label.Name] = label.Value
	}
	return out
}

func defaultAlgorithmForSeriesKind(value string) string {
	if value == "counter" {
		return "incremental"
	}
	return "absolute"
}
