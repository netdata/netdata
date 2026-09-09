// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

const (
	perfFieldValue                 = "value"
	perfThresholdStateNone         = "no_threshold"
	perfThresholdStateOK           = "ok"
	perfThresholdStateWarning      = "warning"
	perfThresholdStateCritical     = "critical"
	perfThresholdStateRetry        = "retry"
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
	perfThresholdAlertStateNames = []string{
		perfThresholdStateNone,
		perfThresholdStateOK,
		perfThresholdStateWarning,
		perfThresholdStateCritical,
		perfThresholdStateRetry,
	}
)

type perfValueMeasureSet struct {
	name      string
	checkName string
	unit      string
	counter   bool
	value     metrix.SampleValue
}

type perfThresholdStateSet struct {
	name          string
	checkName     string
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
	return stateSetPoint(perfThresholdStateNames, active)
}

func perfThresholdAlertStatePoint(actives ...string) metrix.StateSetPoint {
	return stateSetPoint(perfThresholdAlertStateNames, actives...)
}

func stateSetPoint(names []string, actives ...string) metrix.StateSetPoint {
	states := make(map[string]bool, len(names))
	for _, state := range names {
		states[state] = false
	}
	for _, active := range actives {
		if active == "" {
			continue
		}
		states[active] = true
	}
	return metrix.StateSetPoint{States: states}
}
