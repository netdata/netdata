// SPDX-License-Identifier: GPL-3.0-or-later

package charttpl

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
)

var (
	validAlgorithms = []string{"absolute", "incremental"}
	validChartTypes = []string{"line", "area", "stacked", "heatmap"}
)

// Validation contains immutable runtime artifacts derived while validating a
// spec. It is separate from Spec so validation never mutates caller-owned data.
type Validation struct {
	autogenRules []ValidatedAutogenRule
}

// ValidatedAutogenRule is an immutable compiled autogen rule.
type ValidatedAutogenRule struct {
	scope    matcher.Matcher
	selector metrixselector.Selector
}

// ScopeMatches reports whether the rule applies to a source metric family.
func (r ValidatedAutogenRule) ScopeMatches(metricName string) bool {
	return r.scope != nil && r.scope.MatchString(metricName)
}

// Selects reports whether the rule permits fallback for a source series.
func (r ValidatedAutogenRule) Selects(metricName string, labels metrix.LabelView) bool {
	return r.selector != nil && r.selector.Matches(metricName, labels)
}

// AutogenRules returns the validated compiled autogen rules.
func (v Validation) AutogenRules() []ValidatedAutogenRule {
	return slices.Clone(v.autogenRules)
}

// Validate performs semantic checks for one chart template spec.
func (s *Spec) Validate() error {
	_, err := Validate(s)
	return err
}

// Validate performs semantic checks and returns immutable derived artifacts.
func Validate(s *Spec) (Validation, error) {
	if s == nil {
		return Validation{}, semErr("", "nil spec")
	}
	var errs []error
	if s.Version != VersionV1 {
		errs = append(errs, semErr("version", fmt.Sprintf("expected %q", VersionV1)))
	}
	if len(s.Groups) == 0 {
		errs = append(errs, semErr("groups", "groups[] is required"))
	}
	rules, err := validateEngine(s.Engine)
	errs = append(errs, err)

	for i := range s.Groups {
		errs = append(errs, validateGroup(s.Groups[i], fmt.Sprintf("groups[%d]", i), nil))
	}
	return Validation{autogenRules: rules}, errors.Join(errs...)
}

func validateGroup(group Group, path string, inheritedMetrics map[string]struct{}) error {
	var errs []error
	if strings.TrimSpace(group.Family) == "" {
		errs = append(errs, semErr(path+".family", "must not be empty"))
	}
	errs = append(errs, validateChartDefaults(group.ChartDefaults, path))

	ownMetrics := make(map[string]struct{}, len(group.Metrics))
	for i, name := range group.Metrics {
		name = strings.TrimSpace(name)
		if name == "" {
			errs = append(errs, semErr(fmt.Sprintf("%s.metrics[%d]", path, i), "metric name must not be empty"))
			continue
		}
		if _, ok := ownMetrics[name]; ok {
			errs = append(errs, semErr(fmt.Sprintf("%s.metrics[%d]", path, i), fmt.Sprintf("duplicate metric %q", name)))
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
		errs = append(errs, validateChart(group.Charts[i], fmt.Sprintf("%s.charts[%d]", path, i), effective))
	}
	for i := range group.Groups {
		errs = append(errs, validateGroup(group.Groups[i], fmt.Sprintf("%s.groups[%d]", path, i), effective))
	}
	return errors.Join(errs...)
}

func validateChartDefaults(defaults *ChartDefaults, path string) error {
	if defaults == nil {
		return nil
	}
	return errors.Join(
		validateLabelPromotion(defaults.LabelPromoted, path+".chart_defaults.label_promotion"),
		validateInstances(defaults.Instances, path+".chart_defaults"),
	)
}

func validateChart(chart Chart, path string, effectiveMetrics map[string]struct{}) error {
	return errors.Join(
		validateChartCore(chart, path),
		validateLabelPromotion(chart.LabelPromoted, path+".label_promotion"),
		validateLifecycle(chart.Lifecycle, path),
		validateInstances(chart.Instances, path),
		validateDimensions(chart.Dimensions, path, effectiveMetrics),
	)
}

func validateChartCore(chart Chart, path string) error {
	var errs []error
	if strings.TrimSpace(chart.Title) == "" {
		errs = append(errs, semErr(path+".title", "must not be empty"))
	}
	if strings.TrimSpace(chart.Context) == "" {
		errs = append(errs, semErr(path+".context", "must not be empty"))
	}
	if strings.TrimSpace(chart.Units) == "" {
		errs = append(errs, semErr(path+".units", "must not be empty"))
	}
	if chart.Algorithm != "" && !slices.Contains(validAlgorithms, chart.Algorithm) {
		errs = append(errs, semErr(path+".algorithm", fmt.Sprintf("must be one of %v", validAlgorithms)))
	}
	if chart.Type != "" && !slices.Contains(validChartTypes, chart.Type) {
		errs = append(errs, semErr(path+".type", fmt.Sprintf("must be one of %v", validChartTypes)))
	}
	return errors.Join(errs...)
}

func validateLifecycle(lifecycle *Lifecycle, path string) error {
	if lifecycle == nil {
		return nil
	}
	var errs []error
	if lifecycle.MaxInstances < 0 {
		errs = append(errs, semErr(path+".lifecycle.max_instances", "must be >= 0"))
	}
	if lifecycle.ExpireAfterCycles < 0 {
		errs = append(errs, semErr(path+".lifecycle.expire_after_cycles", "must be >= 0"))
	}
	if lifecycle.Dimensions != nil {
		if lifecycle.Dimensions.MaxDims < 0 {
			errs = append(errs, semErr(path+".lifecycle.dimensions.max_dims", "must be >= 0"))
		}
		if lifecycle.Dimensions.ExpireAfterCycles < 0 {
			errs = append(errs, semErr(path+".lifecycle.dimensions.expire_after_cycles", "must be >= 0"))
		}
	}
	return errors.Join(errs...)
}

func validateInstances(instances *Instances, path string) error {
	if instances == nil {
		return nil
	}
	var errs []error
	hasPositive := false
	if len(instances.ByLabels) == 0 {
		return semErr(path+".instances.by_labels", "must contain at least one token when instances is set")
	}

	seen := make(map[string]struct{}, len(instances.ByLabels))
	for i, token := range instances.ByLabels {
		token = strings.TrimSpace(token)
		if token == "" {
			errs = append(errs, semErr(fmt.Sprintf("%s.instances.by_labels[%d]", path, i), "must not be empty"))
			continue
		}
		switch {
		case token == "*":
			hasPositive = true
		case strings.HasPrefix(token, "!"):
			key := strings.TrimPrefix(token, "!")
			if key == "" {
				errs = append(errs, semErr(fmt.Sprintf("%s.instances.by_labels[%d]", path, i), "exclude token must include label key"))
				continue
			}
			if strings.TrimSpace(key) != key {
				errs = append(errs, semErr(fmt.Sprintf("%s.instances.by_labels[%d]", path, i), "exclude token must use !label_key syntax"))
				continue
			}
		default:
			hasPositive = true
		}
		if _, ok := seen[token]; ok {
			errs = append(errs, semErr(fmt.Sprintf("%s.instances.by_labels[%d]", path, i), fmt.Sprintf("duplicate token %q", token)))
		}
		seen[token] = struct{}{}
	}
	if !hasPositive {
		errs = append(errs, semErr(path+".instances.by_labels", "must include at least one positive selector ('*' or label key)"))
	}
	return errors.Join(errs...)
}

func validateDimensions(dimensions []Dimension, path string, effectiveMetrics map[string]struct{}) error {
	if len(dimensions) == 0 {
		return semErr(path+".dimensions", "at least one dimension is required")
	}

	var errs []error
	seenDimNames := make(map[string]struct{}, len(dimensions))
	for i := range dimensions {
		d := dimensions[i]
		selectorExpr := strings.TrimSpace(d.Selector)
		if selectorExpr == "" {
			errs = append(errs, semErr(fmt.Sprintf("%s.dimensions[%d].selector", path, i), "must not be empty"))
		} else {
			metricName, ok := selectorMetricName(selectorExpr)
			if !ok {
				errs = append(errs, semErr(fmt.Sprintf("%s.dimensions[%d].selector", path, i), "selector must include explicit metric name"))
			} else if _, ok := effectiveMetrics[metricName]; !ok {
				errs = append(errs, semErr(fmt.Sprintf("%s.dimensions[%d].selector", path, i), fmt.Sprintf("metric %q is not visible in current group scope", metricName)))
			}
		}

		name := strings.TrimSpace(d.Name)
		nameFrom := strings.TrimSpace(d.NameFromLabel)
		if d.Name != "" && name == "" {
			errs = append(errs, semErr(fmt.Sprintf("%s.dimensions[%d].name", path, i), "must not be whitespace-only"))
		}
		if d.NameFromLabel != "" && nameFrom == "" {
			errs = append(errs, semErr(fmt.Sprintf("%s.dimensions[%d].name_from_label", path, i), "must not be whitespace-only"))
		}
		if name != "" && nameFrom != "" {
			errs = append(errs, semErr(fmt.Sprintf("%s.dimensions[%d]", path, i), "use either name or name_from_label, not both"))
		}
		if name != "" {
			if _, ok := seenDimNames[name]; ok {
				errs = append(errs, semErr(fmt.Sprintf("%s.dimensions[%d].name", path, i), fmt.Sprintf("duplicate dimension name %q", name)))
			} else {
				seenDimNames[name] = struct{}{}
			}
		}
	}
	return errors.Join(errs...)
}

func validateEngine(engine *Engine) ([]ValidatedAutogenRule, error) {
	if engine == nil {
		return nil, nil
	}

	var errs []error
	if engine.Selector != nil {
		errs = append(errs, validateSelectorExpr(*engine.Selector, "engine.selector", false))
	}

	var rules []ValidatedAutogenRule
	if engine.Autogen != nil {
		var err error
		rules, err = CompileAutogenRules(engine.Autogen.Rules)
		if err != nil {
			errs = append(errs, err)
		}
		if engine.Autogen.MaxTypeIDLen < 0 {
			errs = append(errs, semErr("engine.autogen.max_type_id_len", "must be >= 0"))
		}
		if engine.Autogen.MaxTypeIDLen > 0 && engine.Autogen.MaxTypeIDLen < 4 {
			errs = append(errs, semErr("engine.autogen.max_type_id_len", "must be >= 4 when set"))
		}
	}

	return rules, errors.Join(errs...)
}

// CompileAutogenRules validates and compiles generic conditional autogen rules.
func CompileAutogenRules(rules []EngineAutogenRule) ([]ValidatedAutogenRule, error) {
	if len(rules) == 0 {
		return nil, nil
	}

	var errs []error
	compiled := make([]ValidatedAutogenRule, len(rules))
	for i, rule := range rules {
		path := fmt.Sprintf("engine.autogen.rules[%d]", i)
		scope := strings.TrimSpace(rule.Scope)
		if scope == "" {
			errs = append(errs, semErr(path+".scope", "must not be empty"))
		} else {
			m, err := matcher.NewSimplePatternsMatcher(scope)
			if err != nil {
				errs = append(errs, semErr(path+".scope", err.Error()))
			} else {
				compiled[i].scope = m
			}
		}

		if err := validateSelectorExpr(rule.Selector, path+".selector", true); err != nil {
			errs = append(errs, err)
		} else {
			selector, err := rule.Selector.Parse()
			if err != nil {
				errs = append(errs, semErr(path+".selector", err.Error()))
			} else {
				compiled[i].selector = selector
			}
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return compiled, nil
}

func validateSelectorExpr(expr metrixselector.Expr, path string, requireNonEmpty bool) error {
	var errs []error
	if requireNonEmpty && expr.Empty() {
		errs = append(errs, semErr(path, "must contain at least one allow or deny selector"))
	}
	for i, item := range expr.Allow {
		if strings.TrimSpace(item) == "" {
			errs = append(errs, semErr(fmt.Sprintf("%s.allow[%d]", path, i), "must not be empty"))
		}
	}
	for i, item := range expr.Deny {
		if strings.TrimSpace(item) == "" {
			errs = append(errs, semErr(fmt.Sprintf("%s.deny[%d]", path, i), "must not be empty"))
		}
	}
	return errors.Join(errs...)
}

func validateLabelPromotion(labels []string, path string) error {
	var errs []error
	for i, label := range labels {
		if strings.TrimSpace(label) == "" {
			errs = append(errs, semErr(fmt.Sprintf("%s[%d]", path, i), "must not be empty"))
		}
	}
	return errors.Join(errs...)
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
	return fmt.Errorf("%w: %v", errSemanticCheck, fieldError{
		Path:   path,
		Reason: reason,
	})
}
