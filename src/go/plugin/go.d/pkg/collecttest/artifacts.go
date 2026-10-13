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
// metadata.yaml must describe both. The checks compare decoded artifacts with
// each other so no test needs to restate a context, dimension or alert by name.
// Collectors call the byte-level Assert functions on their shipped files; the
// structure-level Check functions carry the logic and take decoded values.

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

// MetadataMetric is one metadata.yaml metrics.scopes[].metrics[] row with the
// scope it belongs to and its dimension names.
type MetadataMetric struct {
	Scope       string
	Name        string
	Description string
	Unit        string
	ChartType   string
	Dimensions  []string
}

// MetadataModule is the decoded part of one metadata.yaml module these checks use.
type MetadataModule struct {
	ID        string
	Metrics   []MetadataMetric
	Functions []MetadataFunction
}

type metadataDocument struct {
	Modules []struct {
		Meta struct {
			ID string `yaml:"id"`
		} `yaml:"meta"`
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
		Functions struct {
			List []struct {
				ID           string `yaml:"id"`
				FunctionName string `yaml:"function_name"`
				Parameters   []struct {
					ID      string `yaml:"id"`
					Name    string `yaml:"name"`
					Type    string `yaml:"type"`
					Options []struct {
						ID      string `yaml:"id"`
						Name    string `yaml:"name"`
						Default bool   `yaml:"default"`
					} `yaml:"options"`
				} `yaml:"parameters"`
				Returns struct {
					Columns []struct {
						Name       string `yaml:"name"`
						Type       string `yaml:"type"`
						Unit       string `yaml:"unit"`
						Visibility string `yaml:"visibility"`
					} `yaml:"columns"`
				} `yaml:"returns"`
			} `yaml:"list"`
		} `yaml:"functions"`
	} `yaml:"modules"`
}

// DecodeMetadataModule decodes metadata.yaml and returns one module: the one
// whose meta.id equals moduleID, or the only module when moduleID is empty.
func DecodeMetadataModule(metadataYAML []byte, moduleID string) (MetadataModule, error) {
	var doc metadataDocument
	if err := yaml.Unmarshal(metadataYAML, &doc); err != nil {
		return MetadataModule{}, fmt.Errorf("metadata: %w", err)
	}
	index := -1
	switch {
	case moduleID == "" && len(doc.Modules) == 1:
		index = 0
	case moduleID == "":
		return MetadataModule{}, fmt.Errorf("metadata: expected exactly one module, got %d (select one by meta.id)", len(doc.Modules))
	default:
		for i := range doc.Modules {
			if doc.Modules[i].Meta.ID != moduleID {
				continue
			}
			if index >= 0 {
				return MetadataModule{}, fmt.Errorf("metadata: several modules with meta.id %q", moduleID)
			}
			index = i
		}
		if index < 0 {
			return MetadataModule{}, fmt.Errorf("metadata: no module with meta.id %q", moduleID)
		}
	}
	raw := doc.Modules[index]
	out := MetadataModule{ID: raw.Meta.ID}
	for _, scope := range raw.Metrics.Scopes {
		for _, metric := range scope.Metrics {
			row := MetadataMetric{
				Scope:       scope.Name,
				Name:        metric.Name,
				Description: metric.Description,
				Unit:        metric.Unit,
				ChartType:   metric.ChartType,
			}
			for _, dimension := range metric.Dimensions {
				row.Dimensions = append(row.Dimensions, dimension.Name)
			}
			out.Metrics = append(out.Metrics, row)
		}
	}
	for _, function := range raw.Functions.List {
		row := MetadataFunction{ID: function.ID, FunctionName: function.FunctionName}
		for _, parameter := range function.Parameters {
			param := MetadataFunctionParameter{ID: parameter.ID, Name: parameter.Name, Type: parameter.Type}
			for _, option := range parameter.Options {
				param.Options = append(param.Options, MetadataFunctionOption{ID: option.ID, Name: option.Name, Default: option.Default})
			}
			row.Parameters = append(row.Parameters, param)
		}
		for _, column := range function.Returns.Columns {
			row.Columns = append(row.Columns, MetadataFunctionColumn{
				Name: column.Name, Type: column.Type, Unit: column.Unit, Visibility: column.Visibility,
			})
		}
		out.Functions = append(out.Functions, row)
	}
	return out, nil
}

// CheckMetadataMetricsMatchCharts requires the documented metrics to be exactly
// the template charts: every documented metric is a chart context and every
// chart is documented once; description equals the chart title, unit equals
// units, chart_type equals the (defaulted) type, and the dimension names equal
// the names the chart renders. Charts whose dimension names are only known at
// runtime are compared on everything but dimensions.
func CheckMetadataMetricsMatchCharts(metrics []MetadataMetric, charts map[string]charttpl.Chart, statesetStates map[string][]string) error {
	var problems []error
	seen := make(map[string]bool)
	for _, metric := range metrics {
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
		if chart.Type != metric.ChartType {
			problems = append(problems, fmt.Errorf("%s: chart_type %q, chart type %q", metric.Name, metric.ChartType, chart.Type))
		}
		expected, ok, err := ChartDimensionNames(chart, statesetStates)
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", metric.Name, err))
			continue
		}
		if ok && !slices.Equal(expected, metric.Dimensions) {
			problems = append(problems, fmt.Errorf("%s: dimensions %v, chart renders %v", metric.Name, metric.Dimensions, expected))
		}
	}
	for context := range charts {
		if !seen[context] {
			problems = append(problems, fmt.Errorf("%s: in the chart template but not documented", context))
		}
	}
	return errors.Join(sortedErrors(problems)...)
}

// CheckMetadataDocumentsChartTemplate decodes a single-module metadata.yaml and
// a chart template and runs CheckMetadataMetricsMatchCharts.
func CheckMetadataDocumentsChartTemplate(metadataYAML []byte, templateYAML string, statesetStates map[string][]string) error {
	module, err := DecodeMetadataModule(metadataYAML, "")
	if err != nil {
		return err
	}
	charts, err := ChartTemplateCharts(templateYAML)
	if err != nil {
		return fmt.Errorf("template: %w", err)
	}
	return CheckMetadataMetricsMatchCharts(module.Metrics, charts, statesetStates)
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
	Name   string
	On     string
	Units  string
	Lookup string
}

var healthAlertLine = regexp.MustCompile(`^\s*(template|alarm|on|units|lookup)\s*:\s*(.*?)\s*$`)

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
		case "units":
			if len(out) > 0 {
				out[len(out)-1].Units = match[2]
			}
		case "lookup":
			if len(out) > 0 {
				out[len(out)-1].Lookup = match[2]
			}
		}
	}
	return out
}

// lookupDimensions returns the dimension names a lookup line selects by name:
// the comma- or pipe-separated list between "of" and an optional "foreach"
// clause, without the "all" keyword and without pattern entries (wildcards and
// negations), which cannot be resolved against documentation. The grammar is
// the "of" branch of health_config.c.
func lookupDimensions(lookup string) []string {
	fields := strings.Fields(lookup)
	start := slices.Index(fields, "of") + 1
	if start == 0 {
		return nil
	}
	fields = fields[start:]
	if end := slices.IndexFunc(fields, func(f string) bool { return strings.EqualFold(f, "foreach") }); end >= 0 {
		fields = fields[:end]
	}
	var names []string
	for _, entry := range strings.FieldsFunc(strings.Join(fields, ","), func(r rune) bool { return r == ',' || r == '|' }) {
		if !strings.EqualFold(entry, "all") && !strings.ContainsAny(entry, "*!") {
			names = append(names, entry)
		}
	}
	return names
}

// HealthAlertsCheck narrows the health-alerts checks to the alerts of one
// collector when a health.d file is shared by several, and selects the
// metadata module for the checks that read metadata.yaml.
type HealthAlertsCheck struct {
	// ContextPrefix keeps only alerts whose on context has this prefix (for
	// example "ceph."); empty checks every alert.
	ContextPrefix string
	// ModuleID selects the metadata module by meta.id; empty requires a single module.
	ModuleID string
}

// CheckHealthAlertsTargetCharts requires every alert (within opts) to target a
// context the charts provide.
func CheckHealthAlertsTargetCharts(alerts []HealthAlert, charts map[string]charttpl.Chart, opts HealthAlertsCheck) error {
	var problems []error
	for _, alert := range alerts {
		if opts.ContextPrefix != "" && !strings.HasPrefix(alert.On, opts.ContextPrefix) {
			continue
		}
		if _, ok := charts[alert.On]; !ok {
			problems = append(problems, fmt.Errorf("%s: on %q is not a chart template context", alert.Name, alert.On))
		}
	}
	return errors.Join(sortedErrors(problems)...)
}

// CheckHealthAlertsTargetChartTemplate parses a health.d configuration and a
// chart template and runs CheckHealthAlertsTargetCharts over every alert.
func CheckHealthAlertsTargetChartTemplate(healthConfig []byte, templateYAML string) error {
	return CheckHealthAlertsTargetChartTemplateWith(healthConfig, templateYAML, HealthAlertsCheck{})
}

// CheckHealthAlertsTargetChartTemplateWith is CheckHealthAlertsTargetChartTemplate
// restricted by opts.
func CheckHealthAlertsTargetChartTemplateWith(healthConfig []byte, templateYAML string, opts HealthAlertsCheck) error {
	charts, err := ChartTemplateCharts(templateYAML)
	if err != nil {
		return fmt.Errorf("template: %w", err)
	}
	return CheckHealthAlertsTargetCharts(ParseHealthAlerts(healthConfig), charts, opts)
}

// AssertHealthAlertsTargetChartTemplate fails the test when an alert targets a
// context the template does not provide.
func AssertHealthAlertsTargetChartTemplate(t testing.TB, healthConfig []byte, templateYAML string) {
	t.Helper()
	AssertHealthAlertsTargetChartTemplateWith(t, healthConfig, templateYAML, HealthAlertsCheck{})
}

// AssertHealthAlertsTargetChartTemplateWith is AssertHealthAlertsTargetChartTemplate
// restricted by opts.
func AssertHealthAlertsTargetChartTemplateWith(t testing.TB, healthConfig []byte, templateYAML string, opts HealthAlertsCheck) {
	t.Helper()
	if err := CheckHealthAlertsTargetChartTemplateWith(healthConfig, templateYAML, opts); err != nil {
		t.Fatalf("health alerts do not match the chart template:\n%v", err)
	}
}

// CheckHealthAlertsMatchMetadataMetrics requires every alert (within opts) to
// target a documented metric, to state that metric's documented unit and to
// look up only its documented dimensions. An alert whose lookup derives another
// unit (a percentage of the window, an anomaly rate) does not fit this check.
func CheckHealthAlertsMatchMetadataMetrics(alerts []HealthAlert, metrics []MetadataMetric, opts HealthAlertsCheck) error {
	var problems []error
	byName := make(map[string]MetadataMetric, len(metrics))
	for _, metric := range metrics {
		if _, dup := byName[metric.Name]; dup {
			problems = append(problems, fmt.Errorf("%s: documented twice", metric.Name))
			continue
		}
		byName[metric.Name] = metric
	}
	for _, alert := range alerts {
		if opts.ContextPrefix != "" && !strings.HasPrefix(alert.On, opts.ContextPrefix) {
			continue
		}
		metric, ok := byName[alert.On]
		if !ok {
			problems = append(problems, fmt.Errorf("%s: on %q is not a documented metric", alert.Name, alert.On))
			continue
		}
		if alert.Units != metric.Unit {
			problems = append(problems, fmt.Errorf("%s: units %q, documented unit %q", alert.Name, alert.Units, metric.Unit))
		}
		for _, dimension := range lookupDimensions(alert.Lookup) {
			if !slices.Contains(metric.Dimensions, dimension) {
				problems = append(problems, fmt.Errorf("%s: lookup dimension %q is not documented for %s", alert.Name, dimension, alert.On))
			}
		}
	}
	return errors.Join(sortedErrors(problems)...)
}

// CheckHealthAlertsMatchMetadata parses a health.d configuration, decodes a
// single-module metadata.yaml and runs CheckHealthAlertsMatchMetadataMetrics
// over every alert.
func CheckHealthAlertsMatchMetadata(healthConfig, metadataYAML []byte) error {
	return CheckHealthAlertsMatchMetadataWith(healthConfig, metadataYAML, HealthAlertsCheck{})
}

// CheckHealthAlertsMatchMetadataWith is CheckHealthAlertsMatchMetadata
// restricted and selected by opts.
func CheckHealthAlertsMatchMetadataWith(healthConfig, metadataYAML []byte, opts HealthAlertsCheck) error {
	module, err := DecodeMetadataModule(metadataYAML, opts.ModuleID)
	if err != nil {
		return err
	}
	return CheckHealthAlertsMatchMetadataMetrics(ParseHealthAlerts(healthConfig), module.Metrics, opts)
}

// AssertHealthAlertsMatchMetadata fails the test when an alert targets an
// undocumented metric, states another unit or looks up an undocumented dimension.
func AssertHealthAlertsMatchMetadata(t testing.TB, healthConfig, metadataYAML []byte) {
	t.Helper()
	AssertHealthAlertsMatchMetadataWith(t, healthConfig, metadataYAML, HealthAlertsCheck{})
}

// AssertHealthAlertsMatchMetadataWith is AssertHealthAlertsMatchMetadata
// restricted and selected by opts.
func AssertHealthAlertsMatchMetadataWith(t testing.TB, healthConfig, metadataYAML []byte, opts HealthAlertsCheck) {
	t.Helper()
	if err := CheckHealthAlertsMatchMetadataWith(healthConfig, metadataYAML, opts); err != nil {
		t.Fatalf("health alerts do not match metadata.yaml:\n%v", err)
	}
}

func sortedErrors(problems []error) []error {
	slices.SortFunc(problems, func(a, b error) int { return strings.Compare(a.Error(), b.Error()) })
	return problems
}
