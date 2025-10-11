// SPDX-License-Identifier: GPL-3.0-or-later

package selector

import (
	"github.com/netdata/netdata/go/plugins/pkg/matcher"

	"github.com/prometheus/prometheus/model/labels"
)

type Selector interface {
	Matches(lbs labels.Labels) bool
}

const (
	OpEqual             = "="
	OpNegEqual          = "!="
	OpRegexp            = "=~"
	OpNegRegexp         = "!~"
	OpSimplePatterns    = "=*"
	OpNegSimplePatterns = "!*"
)

type labelSelector struct {
	name string
	m    matcher.Matcher
}

func (s labelSelector) Matches(lbs labels.Labels) bool {
	if s.name == labels.MetricName {
		return s.m.MatchString(lbs[0].Value)
	}
	if label, ok := lookupLabel(s.name, lbs[1:]); ok {
		return s.m.MatchString(label.Value)
	}
	return false
}

type Func func(lbs labels.Labels) bool

func (fn Func) Matches(lbs labels.Labels) bool {
	return fn(lbs)
}

func lookupLabel(name string, lbs labels.Labels) (labels.Label, bool) {
	for _, label := range lbs {
		if label.Name == name {
			return label, true
		}
	}
	return labels.Label{}, false
}
