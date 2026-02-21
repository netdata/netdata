// SPDX-License-Identifier: GPL-3.0-or-later

package selector

import (
	"github.com/netdata/netdata/go/plugins/pkg/selectorcore"

	"github.com/prometheus/prometheus/model/labels"
)

type Selector interface {
	Matches(lbs labels.Labels) bool
}

const (
	OpEqual             = selectorcore.OpEqual
	OpNegEqual          = selectorcore.OpNegEqual
	OpRegexp            = selectorcore.OpRegexp
	OpNegRegexp         = selectorcore.OpNegRegexp
	OpSimplePatterns    = selectorcore.OpSimplePatterns
	OpNegSimplePatterns = selectorcore.OpNegSimplePatterns
)

type Func func(lbs labels.Labels) bool

func (fn Func) Matches(lbs labels.Labels) bool {
	return fn(lbs)
}

type corePromSelector struct {
	core selectorcore.Selector
}

func (s corePromSelector) Matches(lbs labels.Labels) bool {
	return s.core.Matches(metricNameFromPromLabels(lbs), promLabelsView{labels: lbs})
}

func wrapCoreSelector(sel selectorcore.Selector) Selector {
	if sel == nil {
		return nil
	}
	return corePromSelector{core: sel}
}

type promLabelsView struct {
	labels labels.Labels
}

func (v promLabelsView) Get(key string) (string, bool) {
	for _, label := range v.labels {
		if label.Name == key {
			return label.Value, true
		}
	}
	return "", false
}

func metricNameFromPromLabels(lbs labels.Labels) string {
	if len(lbs) == 0 {
		return ""
	}
	if lbs[0].Name == labels.MetricName {
		return lbs[0].Value
	}
	for _, label := range lbs {
		if label.Name == labels.MetricName {
			return label.Value
		}
	}
	return ""
}
