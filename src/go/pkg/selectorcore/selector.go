// SPDX-License-Identifier: GPL-3.0-or-later

package selectorcore

import "github.com/netdata/netdata/go/plugins/pkg/matcher"

// Labels is the minimal key/value label accessor used by selector matching.
type Labels interface {
	Get(key string) (string, bool)
}

// Selector matches one metric series represented by metric name + labels.
type Selector interface {
	Matches(metricName string, labels Labels) bool
}

const (
	MetricNameLabel     = "__name__"
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

func (s labelSelector) Matches(metricName string, labels Labels) bool {
	if s.name == MetricNameLabel {
		return s.m.MatchString(metricName)
	}
	if labels == nil {
		return false
	}
	if value, ok := labels.Get(s.name); ok {
		return s.m.MatchString(value)
	}
	return false
}

// Func is an adapter for ad-hoc selector functions.
type Func func(metricName string, labels Labels) bool

func (fn Func) Matches(metricName string, labels Labels) bool {
	return fn(metricName, labels)
}
