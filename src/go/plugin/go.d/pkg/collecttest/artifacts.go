// SPDX-License-Identifier: GPL-3.0-or-later

package collecttest

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"gopkg.in/yaml.v3"
)

// Artifact drift checks: the chart template is the source of truth for
// contexts and dimensions, health.d is the source of truth for alerts, and
// metadata.yaml must describe both. These checks compare the artifacts with each
// other so no test needs to restate a context, dimension or alert by name.

// ChartTemplateCharts decodes a chart template and returns its charts keyed by
// the fully composed context (group namespaces and chart context joined with
// dots), the same composition chartengine applies. Defaults are applied, so an
// omitted chart type reads as "line".
func ChartTemplateCharts(templateYAML string) (map[string]charttpl.Chart, error) {
	spec, err := charttpl.DecodeYAML([]byte(templateYAML))
	if err != nil {
		return nil, err
	}
	out := make(map[string]charttpl.Chart)
	var visit func(groups []charttpl.Group, parent []string) error
	visit = func(groups []charttpl.Group, parent []string) error {
		for _, group := range groups {
			scope := append(append([]string(nil), parent...), normalizeOptionalContextPart(group.ContextNamespace)...)
			for _, chart := range group.Charts {
				parts := append(append([]string(nil), scope...), strings.TrimSpace(chart.Context))
				context := strings.Join(filterEmptyString(parts), ".")
				if _, dup := out[context]; dup {
					return fmt.Errorf("duplicate chart context %q", context)
				}
				out[context] = chart
			}
			if err := visit(group.Groups, scope); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(spec.Groups, normalizeOptionalContextPart(spec.ContextNamespace)); err != nil {
		return nil, err
	}
	return out, nil
}

// ChartDimensionNames resolves the dimension names a template chart renders:
// static names as written, and one dimension per declared state for a stateset
// selector without a name (statesetStates maps metric name to its declared
// states). It returns ok=false when a name is only known at runtime
// (name_from_label, histogram buckets, summary quantiles) and an error when an
// unnamed selector is neither of those and its metric has no declared states.
func ChartDimensionNames(chart charttpl.Chart, statesetStates map[string][]string) ([]string, bool, error) {
	var names []string
	for _, dim := range chart.Dimensions {
		if name := strings.TrimSpace(dim.Name); name != "" {
			names = append(names, name)
			continue
		}
		if strings.TrimSpace(dim.NameFromLabel) != "" {
			return nil, false, nil
		}
		compiled, err := metrixselector.ParseCompiled(dim.Selector)
		if err != nil {
			return nil, false, fmt.Errorf("selector %q: %w", dim.Selector, err)
		}
		meta := compiled.Meta()
		if isRuntimeNamedSelector(meta) {
			return nil, false, nil
		}
		for _, metric := range meta.MetricNames {
			states, ok := statesetStates[metric]
			if !ok {
				return nil, false, fmt.Errorf("selector %q has no dimension name and metric %q has no declared states", dim.Selector, metric)
			}
			names = append(names, states...)
		}
	}
	return names, true, nil
}

// isRuntimeNamedSelector reports histogram bucket and summary quantile
// selectors, whose dimension names exist only at runtime.
func isRuntimeNamedSelector(meta metrixselector.Meta) bool {
	if slices.Contains(meta.ConstrainedLabelKeys, "le") || slices.Contains(meta.ConstrainedLabelKeys, "quantile") {
		return true
	}
	for _, metric := range meta.MetricNames {
		if strings.HasSuffix(metric, "_bucket") {
			return true
		}
	}
	return false
}

type metadataDocument struct {
	Modules []struct {
		Alerts []struct {
			Name   string `yaml:"name"`
			Metric string `yaml:"metric"`
			Info   string `yaml:"info"`
		} `yaml:"alerts"`
		Metrics struct {
			Scopes []struct {
				Name    string `yaml:"name"`
				Metrics []struct {
					Name        string `yaml:"name"`
					Description string `yaml:"description"`
					Unit        string `yaml:"unit"`
					ChartType   string `yaml:"chart_type"`
					Dimensions  []struct {
						Name string `yaml:"name"`
					} `yaml:"dimensions"`
				} `yaml:"metrics"`
			} `yaml:"scopes"`
		} `yaml:"metrics"`
	} `yaml:"modules"`
}

func decodeSingleModuleMetadata(metadataYAML []byte) (*metadataDocument, error) {
	var doc metadataDocument
	if err := yaml.Unmarshal(metadataYAML, &doc); err != nil {
		return nil, fmt.Errorf("metadata: %w", err)
	}
	if len(doc.Modules) != 1 {
		return nil, fmt.Errorf("metadata: expected exactly one module, got %d", len(doc.Modules))
	}
	return &doc, nil
}

// CheckMetadataDocumentsChartTemplate requires the metrics documented in a
// single-module metadata.yaml to be exactly the charts of the template: every
// documented metric is a template context and every template context is
// documented once; description equals the chart title, unit equals units,
// chart_type equals the (defaulted) type, and the dimension names equal the
// names the chart renders. Charts whose dimension names are only known at
// runtime are compared on everything but dimensions.
func CheckMetadataDocumentsChartTemplate(metadataYAML []byte, templateYAML string, statesetStates map[string][]string) error {
	doc, err := decodeSingleModuleMetadata(metadataYAML)
	if err != nil {
		return err
	}
	charts, err := ChartTemplateCharts(templateYAML)
	if err != nil {
		return fmt.Errorf("template: %w", err)
	}
	var problems []error
	seen := make(map[string]bool)
	for _, scope := range doc.Modules[0].Metrics.Scopes {
		for _, metric := range scope.Metrics {
			if seen[metric.Name] {
				problems = append(problems, fmt.Errorf("%s: documented twice", metric.Name))
				continue
			}
			seen[metric.Name] = true
			chart, ok := charts[metric.Name]
			if !ok {
				problems = append(problems, fmt.Errorf("%s: documented but not in the chart template", metric.Name))
				continue
			}
			if chart.Title != metric.Description {
				problems = append(problems, fmt.Errorf("%s: description %q, chart title %q", metric.Name, metric.Description, chart.Title))
			}
			if chart.Units != metric.Unit {
				problems = append(problems, fmt.Errorf("%s: unit %q, chart units %q", metric.Name, metric.Unit, chart.Units))
			}
			if string(chart.Type) != metric.ChartType {
				problems = append(problems, fmt.Errorf("%s: chart_type %q, chart type %q", metric.Name, metric.ChartType, chart.Type))
			}
			expected, ok, err := ChartDimensionNames(chart, statesetStates)
			if err != nil {
				problems = append(problems, fmt.Errorf("%s: %w", metric.Name, err))
				continue
			}
			if !ok {
				continue
			}
			actual := make([]string, 0, len(metric.Dimensions))
			for _, dimension := range metric.Dimensions {
				actual = append(actual, dimension.Name)
			}
			if !slices.Equal(expected, actual) {
				problems = append(problems, fmt.Errorf("%s: dimensions %v, chart renders %v", metric.Name, actual, expected))
			}
		}
	}
	for context := range charts {
		if !seen[context] {
			problems = append(problems, fmt.Errorf("%s: in the chart template but not documented", context))
		}
	}
	return errors.Join(sortedErrors(problems)...)
}

// AssertMetadataDocumentsChartTemplate fails the test when
// CheckMetadataDocumentsChartTemplate reports drift.
func AssertMetadataDocumentsChartTemplate(t testing.TB, metadataYAML []byte, templateYAML string, statesetStates map[string][]string) {
	t.Helper()
	if err := CheckMetadataDocumentsChartTemplate(metadataYAML, templateYAML, statesetStates); err != nil {
		t.Fatalf("metadata.yaml does not match the chart template:\n%v", err)
	}
}

// HealthAlert is one alarm or template block of a health.d configuration.
type HealthAlert struct {
	Name string
	On   string
	Info string
}

var healthAlertLine = regexp.MustCompile(`^\s*(template|alarm|on|info)\s*:\s*(.*?)\s*$`)

// ParseHealthAlerts extracts the alarm/template blocks of a health.d
// configuration with the fields the artifact checks compare.
func ParseHealthAlerts(healthConfig []byte) []HealthAlert {
	var out []HealthAlert
	for line := range strings.SplitSeq(string(healthConfig), "\n") {
		match := healthAlertLine.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		switch match[1] {
		case "template", "alarm":
			out = append(out, HealthAlert{Name: match[2]})
		case "on":
			if len(out) > 0 {
				out[len(out)-1].On = match[2]
			}
		case "info":
			if len(out) > 0 {
				out[len(out)-1].Info = match[2]
			}
		}
	}
	return out
}

// CheckHealthAlertsTargetChartTemplate requires every alert in a health.d
// configuration to target a context the chart template provides.
func CheckHealthAlertsTargetChartTemplate(healthConfig []byte, templateYAML string) error {
	charts, err := ChartTemplateCharts(templateYAML)
	if err != nil {
		return fmt.Errorf("template: %w", err)
	}
	var problems []error
	for _, alert := range ParseHealthAlerts(healthConfig) {
		if _, ok := charts[alert.On]; !ok {
			problems = append(problems, fmt.Errorf("%s: on %q is not a chart template context", alert.Name, alert.On))
		}
	}
	return errors.Join(sortedErrors(problems)...)
}

// AssertHealthAlertsTargetChartTemplate fails the test when an alert targets a
// context the template does not provide.
func AssertHealthAlertsTargetChartTemplate(t testing.TB, healthConfig []byte, templateYAML string) {
	t.Helper()
	if err := CheckHealthAlertsTargetChartTemplate(healthConfig, templateYAML); err != nil {
		t.Fatalf("health alerts do not match the chart template:\n%v", err)
	}
}

// CheckMetadataAlertsMatchHealthConfig requires the alerts documented in a
// single-module metadata.yaml to be exactly the alerts of the health.d
// configuration: same names, metric equal to the alert's on context, and info
// text equal verbatim.
func CheckMetadataAlertsMatchHealthConfig(metadataYAML, healthConfig []byte) error {
	doc, err := decodeSingleModuleMetadata(metadataYAML)
	if err != nil {
		return err
	}
	// A health configuration may repeat a name for OS- or label-specific
	// variants; every variant must match the single documented entry.
	shipped := make(map[string][]HealthAlert)
	for _, alert := range ParseHealthAlerts(healthConfig) {
		shipped[alert.Name] = append(shipped[alert.Name], alert)
	}
	var problems []error
	seen := make(map[string]bool)
	for _, alert := range doc.Modules[0].Alerts {
		if seen[alert.Name] {
			problems = append(problems, fmt.Errorf("%s: documented twice", alert.Name))
			continue
		}
		seen[alert.Name] = true
		variants, ok := shipped[alert.Name]
		if !ok {
			problems = append(problems, fmt.Errorf("%s: documented but not in the health configuration", alert.Name))
			continue
		}
		for _, actual := range variants {
			if alert.Metric != actual.On {
				problems = append(problems, fmt.Errorf("%s: metric %q, alert on %q", alert.Name, alert.Metric, actual.On))
			}
			if alert.Info != actual.Info {
				problems = append(problems, fmt.Errorf("%s: info %q, alert info %q", alert.Name, alert.Info, actual.Info))
			}
		}
	}
	for name := range shipped {
		if !seen[name] {
			problems = append(problems, fmt.Errorf("%s: in the health configuration but not documented", name))
		}
	}
	return errors.Join(sortedErrors(problems)...)
}

// AssertMetadataAlertsMatchHealthConfig fails the test when
// CheckMetadataAlertsMatchHealthConfig reports drift.
func AssertMetadataAlertsMatchHealthConfig(t testing.TB, metadataYAML, healthConfig []byte) {
	t.Helper()
	if err := CheckMetadataAlertsMatchHealthConfig(metadataYAML, healthConfig); err != nil {
		t.Fatalf("metadata.yaml alerts do not match the health configuration:\n%v", err)
	}
}

func sortedErrors(problems []error) []error {
	slices.SortFunc(problems, func(a, b error) int { return strings.Compare(a.Error(), b.Error()) })
	return problems
}
