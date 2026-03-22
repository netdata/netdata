// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

const (
	perfFieldValue                 = "value"
	perfThresholdStateNone         = "no_threshold"
	perfThresholdStateOK           = "ok"
	perfThresholdStateWarning      = "warning"
	perfThresholdStateCritical     = "critical"
	perfdataValueLabelKey          = "perfdata_value"
	jobPerfdataThresholdMetricName = "job.perfdata.threshold_state"
)

var (
	perfThresholdStateNames = []string{
		perfThresholdStateNone,
		perfThresholdStateOK,
		perfThresholdStateWarning,
		perfThresholdStateCritical,
	}
)

type perfValueMeasureSet struct {
	name       string
	scriptName string
	unit       string
	counter    bool
	value      metrix.SampleValue
}

type perfThresholdStateSet struct {
	name          string
	scriptName    string
	perfdataValue string
	state         string
}

type perfRouteResult struct {
	values          []perfValueMeasureSet
	thresholdStates []perfThresholdStateSet
}

func perfMeasureSetFieldSpecs() []metrix.MeasureFieldSpec {
	return []metrix.MeasureFieldSpec{
		{Name: perfFieldValue, Float: true},
	}
}

func perfMeasureFieldFloat(field string) bool {
	return field == perfFieldValue
}

func defaultPerfMeasureSetValues() map[string]metrix.SampleValue {
	return map[string]metrix.SampleValue{perfFieldValue: 0}
}

func perfMeasureSetValues(value metrix.SampleValue) map[string]metrix.SampleValue {
	return map[string]metrix.SampleValue{perfFieldValue: value}
}

func perfThresholdStatePoint(active string) metrix.StateSetPoint {
	states := make(map[string]bool, len(perfThresholdStateNames))
	for _, state := range perfThresholdStateNames {
		states[state] = false
	}
	if active != "" {
		states[active] = true
	}
	return metrix.StateSetPoint{States: states}
}
