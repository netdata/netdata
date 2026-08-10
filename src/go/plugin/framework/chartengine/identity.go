// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
)

type instanceLabelValue struct {
	Key      string
	Value    string
	Optional bool
}

type labelAccessor interface {
	Get(key string) (string, bool)
	Range(fn func(key, value string) bool)
}

type mapLabelAccessor map[string]string

func (m mapLabelAccessor) Get(key string) (string, bool) {
	value, ok := m[key]
	return value, ok
}

func (m mapLabelAccessor) Range(fn func(key, value string) bool) {
	for key, value := range m {
		if !fn(key, value) {
			return
		}
	}
}

type labelViewAccessor struct {
	view metrix.LabelView
}

func (l labelViewAccessor) Get(key string) (string, bool) {
	return l.view.Get(key)
}

func (l labelViewAccessor) Range(fn func(key, value string) bool) {
	l.view.Range(fn)
}

func renderChartInstanceID(identity program.ChartIdentity, labels map[string]string) (string, bool, error) {
	return renderChartInstanceIDWithAccessor(identity, mapLabelAccessor(labels))
}

func renderChartInstanceIDFromViewWithPlan(
	identity program.ChartIdentity,
	plan compiledInstanceLabelPlan,
	labels metrix.LabelView,
) (string, bool, error) {
	return renderChartInstanceIDWithAccessorAndPlan(identity, plan, labelViewAccessor{view: labels})
}

func renderChartInstanceIDWithAccessor(identity program.ChartIdentity, labels labelAccessor) (string, bool, error) {
	return renderChartInstanceIDWithAccessorAndPlan(identity, compileInstanceLabelPlan(identity), labels)
}

func renderChartInstanceIDWithAccessorAndPlan(
	identity program.ChartIdentity,
	plan compiledInstanceLabelPlan,
	labels labelAccessor,
) (string, bool, error) {
	baseID, ok, err := renderTemplate(identity.IDTemplate, labels)
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "", false, nil
	}

	suffix, ok, err := renderInstanceSuffix(plan, labels)
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "", false, nil
	}
	if suffix == "" {
		return baseID, true, nil
	}
	return baseID + suffix, true, nil
}

func renderTemplate(tpl program.Template, _ labelAccessor) (string, bool, error) {
	return tpl.Raw, true, nil
}

func renderInstanceSuffix(plan compiledInstanceLabelPlan, labels labelAccessor) (string, bool, error) {
	values, ok := resolveInstanceLabelValuesWithPlan(plan, labels, nil)
	if !ok {
		return "", false, nil
	}
	if len(values) == 0 {
		return "", true, nil
	}

	parts := make([]string, 0, len(values)*2)
	hasNonEmpty := false
	for _, item := range values {
		if item.Optional {
			parts = append(parts, sanitizeChartIDLabelValue(item.Key))
		}
		part := sanitizeChartIDLabelValue(item.Value)
		if strings.TrimSpace(part) != "" {
			hasNonEmpty = true
		}
		parts = append(parts, part)
	}
	if !hasNonEmpty {
		// Keep base chart ID when every instance label value is empty.
		return "", true, nil
	}
	for i := range parts {
		if strings.TrimSpace(parts[i]) == "" {
			parts[i] = "empty"
		}
	}
	return "_" + strings.Join(parts, "_"), true, nil
}

func resolveInstanceLabelValuesWithPlan(
	plan compiledInstanceLabelPlan,
	labels labelAccessor,
	out []instanceLabelValue,
) ([]instanceLabelValue, bool) {
	out = out[:0]
	for _, key := range plan.explicitKeys {
		value, ok := labels.Get(key)
		if !ok {
			// Explicit instance keys are required to materialize an instance.
			return out[:0], false
		}
		out = append(out, instanceLabelValue{Key: key, Value: value})
	}
	for _, key := range plan.optionalKeys {
		value, ok := labels.Get(key)
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		out = append(out, instanceLabelValue{Key: key, Value: value, Optional: true})
	}

	if plan.includeAll {
		start := len(out)
		labels.Range(func(key, value string) bool {
			if _, excluded := plan.excludeSet[key]; excluded {
				return true
			}
			if _, exists := plan.explicitSet[key]; exists {
				return true
			}
			out = append(out, instanceLabelValue{Key: key, Value: value})
			return true
		})
		extras := out[start:]
		sort.Slice(extras, func(i, j int) bool { return extras[i].Key < extras[j].Key })
	}
	return out, true
}

var chartIDLabelValueSanitizer = strings.NewReplacer(
	"\\", "_",
	"'", "",
	" ", "_",
	".", "_",
)

func sanitizeChartIDLabelValue(value string) string {
	return chartIDLabelValueSanitizer.Replace(value)
}
