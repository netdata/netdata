// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import "github.com/prometheus/prometheus/model/labels"

func copyLabels(lbs []labels.Label) []labels.Label {
	return append([]labels.Label(nil), lbs...)
}

// copyLabelsWithoutName returns a fresh copy of lbs with __name__ removed. In the
// common case __name__ sorts first (it precedes lowercase label names), so the
// remainder is contiguous and copied directly; otherwise a rare label that sorts
// before __name__ (e.g. "UUID") is skipped element by element.
func copyLabelsWithoutName(lbs labels.Labels) labels.Labels {
	if len(lbs) > 0 && lbs[0].Name == labels.MetricName {
		return copyLabels(lbs[1:])
	}
	out := make([]labels.Label, 0, len(lbs))
	for _, lb := range lbs {
		if lb.Name == labels.MetricName {
			continue
		}
		out = append(out, lb)
	}
	return out
}

func removeLabel(lbs labels.Labels, name string) (labels.Labels, string, bool) {
	for i, v := range lbs {
		if v.Name == name {
			return append(lbs[:i], lbs[i+1:]...), v.Value, true
		}
	}
	return lbs, "", false
}

func metricNameValue(lbs labels.Labels) (string, bool) {
	for _, v := range lbs {
		if v.Name == labels.MetricName {
			return v.Value, true
		}
	}
	return "", false
}
