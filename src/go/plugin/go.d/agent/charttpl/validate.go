// SPDX-License-Identifier: GPL-3.0-or-later

package charttpl

import (
	"fmt"
	"slices"
	"strings"
)

var (
	validAlgorithms = []string{"absolute", "incremental"}
	validChartTypes = []string{"line", "area", "stacked", "heatmap"}
)

// Validate performs semantic checks for one chart template spec.
func (s *Spec) Validate() error {
	if s == nil {
		return fmt.Errorf("%w: nil spec", errSemanticCheck)
	}
	if s.Version != VersionV1 {
		return fmt.Errorf("%w: %v", errSemanticCheck, FieldError{
			Path:   "version",
			Reason: fmt.Sprintf("expected %q", VersionV1),
		})
	}
	if len(s.Groups) == 0 {
		return fmt.Errorf("%w: %v", errSemanticCheck, FieldError{
			Path:   "groups",
			Reason: "groups[] is required",
		})
	}
	if err := validateEngine(s.Engine); err != nil {
		return err
	}

	for i := range s.Groups {
		if err := validateGroup(s.Groups[i], fmt.Sprintf("groups[%d]", i), nil); err != nil {
			return err
		}
	}
	return nil
}

// Validate is a compatibility wrapper that delegates to Spec.Validate.
func Validate(spec *Spec) error { return spec.Validate() }

func validateGroup(group Group, path string, inheritedMetrics map[string]struct{}) error {
	if strings.TrimSpace(group.Family) == "" {
		return semErr(path+".family", "must not be empty")
	}

	ownMetrics := make(map[string]struct{}, len(group.Metrics))
	for i, name := range group.Metrics {
		name = strings.TrimSpace(name)
		if name == "" {
			return semErr(fmt.Sprintf("%s.metrics[%d]", path, i), "metric name must not be empty")
		}
		if _, ok := ownMetrics[name]; ok {
			return semErr(fmt.Sprintf("%s.metrics[%d]", path, i), fmt.Sprintf("duplicate metric %q", name))
		}
		ownMetrics[name] = struct{}{}
	}

	effective := make(map[string]struct{}, len(inheritedMetrics)+len(ownMetrics))
	for k := range inheritedMetrics {
		effective[k] = struct{}{}
	}
	for k := range ownMetrics {
		effective[k] = struct{}{}
	}

	for i := range group.Charts {
		if err := validateChart(group.Charts[i], fmt.Sprintf("%s.charts[%d]", path, i), effective); err != nil {
			return err
		}
	}
	for i := range group.Groups {
		if err := validateGroup(group.Groups[i], fmt.Sprintf("%s.groups[%d]", path, i), effective); err != nil {
			return err
		}
	}
	return nil
}

func validateChart(chart Chart, path string, effectiveMetrics map[string]struct{}) error {
	if strings.TrimSpace(chart.Title) == "" {
		return semErr(path+".title", "must not be empty")
	}
	if strings.TrimSpace(chart.Context) == "" {
		return semErr(path+".context", "must not be empty")
	}
	if strings.TrimSpace(chart.Units) == "" {
		return semErr(path+".units", "must not be empty")
	}
	if chart.Algorithm != "" && !slices.Contains(validAlgorithms, chart.Algorithm) {
		return semErr(path+".algorithm", fmt.Sprintf("must be one of %v", validAlgorithms))
	}
	if chart.Type != "" && !slices.Contains(validChartTypes, chart.Type) {
		return semErr(path+".type", fmt.Sprintf("must be one of %v", validChartTypes))
	}
	if chart.Lifecycle != nil {
		if chart.Lifecycle.MaxInstances < 0 {
			return semErr(path+".lifecycle.max_instances", "must be >= 0")
		}
		if chart.Lifecycle.ExpireAfterCycles < 0 {
			return semErr(path+".lifecycle.expire_after_cycles", "must be >= 0")
		}
		if chart.Lifecycle.Dimensions != nil {
			if chart.Lifecycle.Dimensions.MaxDims < 0 {
				return semErr(path+".lifecycle.dimensions.max_dims", "must be >= 0")
			}
			if chart.Lifecycle.Dimensions.ExpireAfterCycles < 0 {
				return semErr(path+".lifecycle.dimensions.expire_after_cycles", "must be >= 0")
			}
		}
	}

	if chart.Instances != nil {
		if len(chart.Instances.ByLabels) == 0 {
			return semErr(path+".instances.by_labels", "must contain at least one token when instances is set")
		}
		seen := make(map[string]struct{}, len(chart.Instances.ByLabels))
		for i, token := range chart.Instances.ByLabels {
			token = strings.TrimSpace(token)
			if token == "" {
				return semErr(fmt.Sprintf("%s.instances.by_labels[%d]", path, i), "must not be empty")
			}
			if token != "*" && strings.HasPrefix(token, "!") && len(token) == 1 {
				return semErr(fmt.Sprintf("%s.instances.by_labels[%d]", path, i), "exclude token must include label key")
			}
			if _, ok := seen[token]; ok {
				return semErr(fmt.Sprintf("%s.instances.by_labels[%d]", path, i), fmt.Sprintf("duplicate token %q", token))
			}
			seen[token] = struct{}{}
		}
	}

	if len(chart.Dimensions) == 0 {
		return semErr(path+".dimensions", "at least one dimension is required")
	}

	seenDimNames := make(map[string]struct{}, len(chart.Dimensions))
	for i := range chart.Dimensions {
		d := chart.Dimensions[i]
		selectorExpr := strings.TrimSpace(d.Selector)
		if selectorExpr == "" {
			return semErr(fmt.Sprintf("%s.dimensions[%d].selector", path, i), "must not be empty")
		}

		metricName, ok := selectorMetricName(selectorExpr)
		if !ok {
			return semErr(fmt.Sprintf("%s.dimensions[%d].selector", path, i), "selector must include explicit metric name")
		}
		if _, ok := effectiveMetrics[metricName]; !ok {
			return semErr(fmt.Sprintf("%s.dimensions[%d].selector", path, i), fmt.Sprintf("metric %q is not visible in current group scope", metricName))
		}

		name := strings.TrimSpace(d.Name)
		nameFrom := strings.TrimSpace(d.NameFromLabel)
		if name != "" && nameFrom != "" {
			return semErr(fmt.Sprintf("%s.dimensions[%d]", path, i), "use either name or name_from_label, not both")
		}
		if name != "" {
			if _, ok := seenDimNames[name]; ok {
				return semErr(fmt.Sprintf("%s.dimensions[%d].name", path, i), fmt.Sprintf("duplicate dimension name %q", name))
			}
			seenDimNames[name] = struct{}{}
		}
	}

	return nil
}

func validateEngine(engine *Engine) error {
	if engine == nil {
		return nil
	}

	if engine.Selector != nil {
		for i, expr := range engine.Selector.Allow {
			if strings.TrimSpace(expr) == "" {
				return semErr(fmt.Sprintf("engine.selector.allow[%d]", i), "must not be empty")
			}
		}
		for i, expr := range engine.Selector.Deny {
			if strings.TrimSpace(expr) == "" {
				return semErr(fmt.Sprintf("engine.selector.deny[%d]", i), "must not be empty")
			}
		}
	}

	if engine.Autogen != nil {
		if engine.Autogen.MaxTypeIDLen < 0 {
			return semErr("engine.autogen.max_type_id_len", "must be >= 0")
		}
		if engine.Autogen.MaxTypeIDLen > 0 && engine.Autogen.MaxTypeIDLen < 4 {
			return semErr("engine.autogen.max_type_id_len", "must be >= 4 when set")
		}
	}

	return nil
}

func selectorMetricName(expr string) (string, bool) {
	s := strings.TrimSpace(expr)
	if s == "" {
		return "", false
	}
	if strings.HasPrefix(s, "{") {
		return "", false
	}
	if idx := strings.IndexByte(s, '{'); idx >= 0 {
		s = strings.TrimSpace(s[:idx])
	}
	if s == "" {
		return "", false
	}
	return s, true
}

func semErr(path, reason string) error {
	return fmt.Errorf("%w: %v", errSemanticCheck, FieldError{
		Path:   path,
		Reason: reason,
	})
}
