// SPDX-License-Identifier: GPL-3.0-or-later

package selector

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/selectorcore"
)

// Selector matches one metrix series.
type Selector interface {
	Matches(metricName string, labels metrix.LabelView) bool
}

const (
	OpEqual             = selectorcore.OpEqual
	OpNegEqual          = selectorcore.OpNegEqual
	OpRegexp            = selectorcore.OpRegexp
	OpNegRegexp         = selectorcore.OpNegRegexp
	OpSimplePatterns    = selectorcore.OpSimplePatterns
	OpNegSimplePatterns = selectorcore.OpNegSimplePatterns
)

// Func is an adapter for ad-hoc selector functions.
type Func func(metricName string, labels metrix.LabelView) bool

func (fn Func) Matches(metricName string, labels metrix.LabelView) bool {
	return fn(metricName, labels)
}

type coreMetrixSelector struct {
	core selectorcore.Selector
}

func (s coreMetrixSelector) Matches(metricName string, labels metrix.LabelView) bool {
	return s.core.Matches(metricName, metrixLabelsView{labels: labels})
}

func wrapCoreSelector(sel selectorcore.Selector) Selector {
	if sel == nil {
		return nil
	}
	return coreMetrixSelector{core: sel}
}

type metrixLabelsView struct {
	labels metrix.LabelView
}

func (v metrixLabelsView) Get(key string) (string, bool) {
	if v.labels == nil {
		return "", false
	}
	return v.labels.Get(key)
}
