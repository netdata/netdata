// SPDX-License-Identifier: GPL-3.0-or-later

package program

import (
	"errors"
	"fmt"
)

// Variable is one compiled chart-scope variable (VARIABLE CHART) template.
type Variable struct {
	// Name is the wire and health name of the variable.
	Name     string
	Selector SelectorBinding
	// Aggregation is the chart's reducer for several series in one build.
	Aggregation Aggregation
}

func validateVariable(variable Variable) error {
	var errs []error
	if variable.Name == "" {
		errs = append(errs, fmt.Errorf("name is required"))
	}
	if variable.Selector.Expression == "" {
		errs = append(errs, fmt.Errorf("selector expression is required"))
	}
	if variable.Selector.Matcher == nil {
		errs = append(errs, fmt.Errorf("selector matcher is required"))
	}
	switch variable.Aggregation {
	case AggregationSum, AggregationMin, AggregationMax, AggregationAvg:
	default:
		errs = append(errs, fmt.Errorf("invalid aggregation %d", variable.Aggregation))
	}
	return errors.Join(errs...)
}

func (v Variable) clone() Variable {
	out := v
	out.Selector = v.Selector.clone()
	return out
}
