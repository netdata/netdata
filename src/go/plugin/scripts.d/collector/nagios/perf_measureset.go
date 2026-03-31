// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

const (
	perfFieldValue                 = "value"
	perfFieldWarnLow               = "warn_low"
	perfFieldWarnHigh              = "warn_high"
	perfFieldCritLow               = "crit_low"
	perfFieldCritHigh              = "crit_high"
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
	name       string
	scriptName string
	unit       string
	counter    bool
	fieldMask  perfMeasureFieldMask
	value      metrix.SampleValue
	warnLow    metrix.SampleValue
	warnHigh   metrix.SampleValue
	critLow    metrix.SampleValue
	critHigh   metrix.SampleValue
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

type perfMeasureFieldMask uint8

const (
	perfMeasureFieldWarnLow perfMeasureFieldMask = 1 << iota
	perfMeasureFieldWarnHigh
	perfMeasureFieldCritLow
	perfMeasureFieldCritHigh
)

func perfMeasureSetFieldSpecs(mask perfMeasureFieldMask) []metrix.MeasureFieldSpec {
	specs := []metrix.MeasureFieldSpec{
		{Name: perfFieldValue, Float: true},
	}
	if mask.has(perfMeasureFieldWarnLow) {
		specs = append(specs, metrix.MeasureFieldSpec{Name: perfFieldWarnLow, Float: true})
	}
	if mask.has(perfMeasureFieldWarnHigh) {
		specs = append(specs, metrix.MeasureFieldSpec{Name: perfFieldWarnHigh, Float: true})
	}
	if mask.has(perfMeasureFieldCritLow) {
		specs = append(specs, metrix.MeasureFieldSpec{Name: perfFieldCritLow, Float: true})
	}
	if mask.has(perfMeasureFieldCritHigh) {
		specs = append(specs, metrix.MeasureFieldSpec{Name: perfFieldCritHigh, Float: true})
	}
	return specs
}

func perfMeasureFieldFloat(field string) bool {
	switch field {
	case perfFieldValue, perfFieldWarnLow, perfFieldWarnHigh, perfFieldCritLow, perfFieldCritHigh:
		return true
	default:
		return false
	}
}

func defaultPerfMeasureSetValues(mask perfMeasureFieldMask) map[string]metrix.SampleValue {
	fields := map[string]metrix.SampleValue{
		perfFieldValue: 0,
	}
	if mask.has(perfMeasureFieldWarnLow) {
		fields[perfFieldWarnLow] = 0
	}
	if mask.has(perfMeasureFieldWarnHigh) {
		fields[perfFieldWarnHigh] = 0
	}
	if mask.has(perfMeasureFieldCritLow) {
		fields[perfFieldCritLow] = 0
	}
	if mask.has(perfMeasureFieldCritHigh) {
		fields[perfFieldCritHigh] = 0
	}
	return fields
}

func perfMeasureSetValues(measureSet perfValueMeasureSet) map[string]metrix.SampleValue {
	fields := defaultPerfMeasureSetValues(measureSet.fieldMask)
	fields[perfFieldValue] = measureSet.value
	if measureSet.fieldMask.has(perfMeasureFieldWarnLow) {
		fields[perfFieldWarnLow] = measureSet.warnLow
	}
	if measureSet.fieldMask.has(perfMeasureFieldWarnHigh) {
		fields[perfFieldWarnHigh] = measureSet.warnHigh
	}
	if measureSet.fieldMask.has(perfMeasureFieldCritLow) {
		fields[perfFieldCritLow] = measureSet.critLow
	}
	if measureSet.fieldMask.has(perfMeasureFieldCritHigh) {
		fields[perfFieldCritHigh] = measureSet.critHigh
	}
	return fields
}

func (m perfMeasureFieldMask) has(flag perfMeasureFieldMask) bool {
	return m&flag != 0
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
