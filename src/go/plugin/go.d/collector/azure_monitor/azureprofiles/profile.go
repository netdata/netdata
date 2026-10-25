// SPDX-License-Identifier: GPL-3.0-or-later

package azureprofiles

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

const (
	SeriesKindGauge   = "gauge"
	SeriesKindCounter = "counter"
)

const (
	ResourceLabelValueRaw     = "raw"
	ResourceLabelValueARMName = "arm_name"
)

var (
	reResourceType     = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
	reIdentityID       = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	reResourceProperty = regexp.MustCompile(`^properties(\.[A-Za-z][A-Za-z0-9]*)+$`)
)

// ReservedLabelNames are the labels the collector sets on every resource series.
var ReservedLabelNames = []string{"resource_uid", "subscription_id", "resource_name", "resource_group", "region", "resource_type", "profile"}

var SupportedTimeGrains = map[string]time.Duration{
	"PT1M":  time.Minute,
	"PT5M":  5 * time.Minute,
	"PT15M": 15 * time.Minute,
	"PT30M": 30 * time.Minute,
	"PT1H":  time.Hour,
}

type Profile struct {
	DisplayName     string          `yaml:"display_name" json:"display_name,omitempty"`
	ResourceType    string          `yaml:"resource_type" json:"resource_type,omitempty"`
	MetricNamespace string          `yaml:"metric_namespace,omitempty" json:"metric_namespace,omitempty"`
	Metrics         []Metric        `yaml:"metrics" json:"metrics,omitempty"`
	ResourceLabels  []ResourceLabel `yaml:"resource_labels,omitempty" json:"resource_labels,omitempty"`
	ResourceStates  []ResourceState `yaml:"resource_states,omitempty" json:"resource_states,omitempty"`
	Template        charttpl.Group  `yaml:"template" json:"template"`
}

// ResourceLabel adds a series label read from an Azure Resource Graph property of each resource.
type ResourceLabel struct {
	Name     string `yaml:"name" json:"name,omitempty"`
	Property string `yaml:"property" json:"property,omitempty"`
	Value    string `yaml:"value,omitempty" json:"value,omitempty"`
}

// ResourceState exports one 0/1 series per state, from an Azure Resource Graph property
// of each resource. It reports state even when Azure Monitor has no metric data for it.
type ResourceState struct {
	ID       string        `yaml:"id" json:"id,omitempty"`
	Property string        `yaml:"property" json:"property,omitempty"`
	States   []StateOption `yaml:"states" json:"states,omitempty"`
}

type StateOption struct {
	Name  string `yaml:"name" json:"name,omitempty"`
	Value string `yaml:"value" json:"value,omitempty"`
}

type Metric struct {
	ID        string         `yaml:"id" json:"id,omitempty"`
	AzureName string         `yaml:"azure_name" json:"azure_name,omitempty"`
	TimeGrain string         `yaml:"time_grain,omitempty" json:"time_grain,omitempty"`
	Series    []MetricSeries `yaml:"series" json:"series,omitempty"`
}

type MetricSeries struct {
	Aggregation string `yaml:"aggregation" json:"aggregation,omitempty"`
	Kind        string `yaml:"kind" json:"kind,omitempty"`
}

func (p *Profile) Normalize(baseName string) error {
	visibleMetrics := visibleMetricsForProfile(baseName, *p)
	normalizeGroupSelectors(baseName, visibleMetrics, &p.Template)
	return nil
}

func (p Profile) Validate(prefix, baseName string) error {
	var errs []error

	if !IsValidProfileName(baseName) {
		errs = append(errs, fmt.Errorf("%s: profile basename must match %q", prefix, reIdentityID.String()))
	}
	if strings.TrimSpace(p.DisplayName) == "" {
		errs = append(errs, fmt.Errorf("%s: 'display_name' is required", prefix))
	}
	if !IsValidResourceType(p.ResourceType) {
		errs = append(errs, fmt.Errorf("%s: 'resource_type' is invalid", prefix))
	}
	if strings.TrimSpace(p.MetricNamespace) != "" && !IsValidResourceType(p.MetricNamespace) {
		errs = append(errs, fmt.Errorf("%s: 'metric_namespace' is invalid", prefix))
	}
	if len(p.Metrics) == 0 {
		errs = append(errs, fmt.Errorf("%s: 'metrics' must contain at least one metric", prefix))
	}
	if len(p.Template.Metrics) > 0 {
		errs = append(errs, fmt.Errorf("%s: 'template.metrics' is collector-owned and must not be authored manually", prefix))
	}
	if countChartsInGroup(p.Template) == 0 {
		errs = append(errs, fmt.Errorf("%s: 'template' must contain at least one chart", prefix))
	}

	seenMetricIDs := map[string]struct{}{}
	seenAzureNames := map[string]struct{}{}
	for i, metric := range p.Metrics {
		if err := metric.validate(prefix, i); err != nil {
			errs = append(errs, err)
		}

		id := strings.ToLower(strings.TrimSpace(metric.ID))
		if id != "" {
			if _, ok := seenMetricIDs[id]; ok {
				errs = append(errs, fmt.Errorf("%s: duplicate metric id '%s'", prefix, metric.ID))
			}
			seenMetricIDs[id] = struct{}{}
		}

		azureName := strings.ToLower(strings.TrimSpace(metric.AzureName))
		if azureName != "" {
			if _, ok := seenAzureNames[azureName]; ok {
				errs = append(errs, fmt.Errorf("%s: duplicate azure metric name '%s'", prefix, metric.AzureName))
			}
			seenAzureNames[azureName] = struct{}{}
		}
	}

	errs = append(errs, validateResourceLabels(prefix, p.ResourceLabels))
	errs = append(errs, validateResourceStates(prefix, p.ResourceStates, seenMetricIDs, metricSeriesNames(p.Metrics)))

	if errors.Join(errs...) == nil {
		if err := validateTemplate(prefix, baseName, p); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (m Metric) validate(profilePrefix string, idx int) error {
	prefix := fmt.Sprintf("%s.metrics[%d]", profilePrefix, idx)
	var errs []error

	if !IsValidIdentityID(m.ID) {
		errs = append(errs, fmt.Errorf("%s: 'id' must match %q", prefix, reIdentityID.String()))
	}
	if strings.TrimSpace(m.AzureName) == "" {
		errs = append(errs, fmt.Errorf("%s: 'azure_name' is required", prefix))
	}
	if strings.TrimSpace(m.TimeGrain) != "" {
		if _, ok := SupportedTimeGrains[strings.ToUpper(strings.TrimSpace(m.TimeGrain))]; !ok {
			errs = append(errs, fmt.Errorf("%s: unsupported time_grain '%s'", prefix, m.TimeGrain))
		}
	}
	if len(m.Series) == 0 {
		errs = append(errs, fmt.Errorf("%s: 'series' must contain at least one series", prefix))
	}

	seenAggregations := map[string]struct{}{}
	for i, series := range m.Series {
		if err := series.validate(prefix, i); err != nil {
			errs = append(errs, err)
		}
		aggregation := NormalizeAggregation(series.Aggregation)
		if aggregation == "" {
			continue
		}
		if _, ok := seenAggregations[aggregation]; ok {
			errs = append(errs, fmt.Errorf("%s: duplicate series aggregation '%s'", prefix, aggregation))
			continue
		}
		seenAggregations[aggregation] = struct{}{}
	}

	return errors.Join(errs...)
}

func (s MetricSeries) validate(metricPrefix string, idx int) error {
	prefix := fmt.Sprintf("%s.series[%d]", metricPrefix, idx)
	var errs []error

	if NormalizeAggregation(s.Aggregation) == "" {
		errs = append(errs, fmt.Errorf("%s: 'aggregation' must be one of average, minimum, maximum, total, count", prefix))
	}
	if NormalizeSeriesKind(s.Kind) == "" {
		errs = append(errs, fmt.Errorf("%s: 'kind' must be one of %q or %q", prefix, SeriesKindGauge, SeriesKindCounter))
	}

	return errors.Join(errs...)
}

func validateResourceLabels(prefix string, labels []ResourceLabel) error {
	var errs []error
	seen := map[string]struct{}{}
	for i, label := range labels {
		labelPrefix := fmt.Sprintf("%s.resource_labels[%d]", prefix, i)
		name := strings.TrimSpace(label.Name)
		if !IsValidIdentityID(name) {
			errs = append(errs, fmt.Errorf("%s: 'name' must match %q", labelPrefix, reIdentityID.String()))
		}
		for _, reserved := range ReservedLabelNames {
			if name == reserved {
				errs = append(errs, fmt.Errorf("%s: 'name' %q is reserved", labelPrefix, name))
			}
		}
		if _, ok := seen[name]; ok {
			errs = append(errs, fmt.Errorf("%s: duplicate label name %q", labelPrefix, name))
		}
		seen[name] = struct{}{}
		if !IsValidResourceProperty(label.Property) {
			errs = append(errs, fmt.Errorf("%s: 'property' must match %q", labelPrefix, reResourceProperty.String()))
		}
		if NormalizeResourceLabelValue(label.Value) == "" {
			errs = append(errs, fmt.Errorf("%s: 'value' must be one of %q or %q", labelPrefix, ResourceLabelValueRaw, ResourceLabelValueARMName))
		}
	}
	return errors.Join(errs...)
}

func metricSeriesNames(metrics []Metric) map[string]struct{} {
	names := make(map[string]struct{})
	for _, metric := range metrics {
		for _, series := range metric.Series {
			names[ExportedSeriesName("", metric.ID, series.Aggregation)] = struct{}{}
		}
	}
	return names
}

func validateResourceStates(prefix string, states []ResourceState, metricIDs, seriesNames map[string]struct{}) error {
	var errs []error
	seen := map[string]struct{}{}
	for i, state := range states {
		statePrefix := fmt.Sprintf("%s.resource_states[%d]", prefix, i)
		id := strings.TrimSpace(state.ID)
		if !IsValidIdentityID(id) {
			errs = append(errs, fmt.Errorf("%s: 'id' must match %q", statePrefix, reIdentityID.String()))
		}
		if _, ok := seen[id]; ok {
			errs = append(errs, fmt.Errorf("%s: duplicate state id %q", statePrefix, id))
		}
		if _, ok := metricIDs[strings.ToLower(id)]; ok {
			errs = append(errs, fmt.Errorf("%s: id %q collides with a metric id", statePrefix, id))
		}
		seen[id] = struct{}{}
		if !IsValidResourceProperty(state.Property) {
			errs = append(errs, fmt.Errorf("%s: 'property' must match %q", statePrefix, reResourceProperty.String()))
		}
		if len(state.States) == 0 {
			errs = append(errs, fmt.Errorf("%s: 'states' must contain at least one state", statePrefix))
		}
		seenOptions := map[string]struct{}{}
		seenValues := map[string]struct{}{}
		for j, option := range state.States {
			optionPrefix := fmt.Sprintf("%s.states[%d]", statePrefix, j)
			name := strings.TrimSpace(option.Name)
			if !IsValidIdentityID(name) {
				errs = append(errs, fmt.Errorf("%s: 'name' must match %q", optionPrefix, reIdentityID.String()))
			}
			if _, ok := seenOptions[name]; ok {
				errs = append(errs, fmt.Errorf("%s: duplicate state name %q", optionPrefix, name))
			}
			seenOptions[name] = struct{}{}
			value := strings.ToLower(strings.TrimSpace(option.Value))
			if value == "" {
				errs = append(errs, fmt.Errorf("%s: 'value' is required", optionPrefix))
			} else if _, ok := seenValues[value]; ok {
				errs = append(errs, fmt.Errorf("%s: duplicate state value %q", optionPrefix, option.Value))
			}
			seenValues[value] = struct{}{}
			series := ExportedStateSeriesName("", id, name)
			if _, ok := seriesNames[series]; ok {
				errs = append(errs, fmt.Errorf("%s: series %q collides with another metric or state series", optionPrefix, strings.TrimPrefix(series, ".")))
			}
			seriesNames[series] = struct{}{}
		}
	}
	return errors.Join(errs...)
}

// ExportedStateSeriesName is the series of one state option, valued 1 when the resource is in it.
func ExportedStateSeriesName(profileName, stateID, stateName string) string {
	return strings.TrimSpace(profileName) + "." + strings.TrimSpace(stateID) + "_" + strings.TrimSpace(stateName)
}

// ResourcePropertyColumn is the discovery query column that carries a resource property.
// Property segments cannot contain underscores, so distinct properties never share a column.
func ResourcePropertyColumn(property string) string {
	return "nd_" + strings.ReplaceAll(strings.TrimSpace(property), ".", "_")
}

func IsValidResourceProperty(v string) bool {
	return reResourceProperty.MatchString(strings.TrimSpace(v))
}

func NormalizeResourceLabelValue(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", ResourceLabelValueRaw:
		return ResourceLabelValueRaw
	case ResourceLabelValueARMName:
		return ResourceLabelValueARMName
	default:
		return ""
	}
}

// ResourceProperties lists the resource properties a profile reads during discovery.
func (p Profile) ResourceProperties() []string {
	var properties []string
	for _, label := range p.ResourceLabels {
		properties = append(properties, strings.TrimSpace(label.Property))
	}
	for _, state := range p.ResourceStates {
		properties = append(properties, strings.TrimSpace(state.Property))
	}
	return properties
}

func ExportedSeriesName(profileName, metricID, aggregation string) string {
	return strings.TrimSpace(profileName) + "." + strings.TrimSpace(metricID) + "_" + NormalizeAggregation(aggregation)
}

func NormalizeAggregation(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "average":
		return "average"
	case "minimum", "min":
		return "minimum"
	case "maximum", "max":
		return "maximum"
	case "total", "sum":
		return "total"
	case "count":
		return "count"
	default:
		return ""
	}
}

func NormalizeSeriesKind(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case SeriesKindGauge:
		return SeriesKindGauge
	case SeriesKindCounter:
		return SeriesKindCounter
	default:
		return ""
	}
}

func IsValidIdentityID(v string) bool {
	return reIdentityID.MatchString(strings.TrimSpace(v))
}

func IsValidProfileName(v string) bool {
	return reIdentityID.MatchString(strings.TrimSpace(v))
}

func IsValidResourceType(v string) bool {
	return reResourceType.MatchString(strings.TrimSpace(v))
}

func validateTemplate(prefix, baseName string, profile Profile) error {
	visibleMetrics := visibleMetricsForProfile(baseName, profile)
	visibleList := make([]string, 0, len(visibleMetrics))
	for metric := range visibleMetrics {
		visibleList = append(visibleList, metric)
	}
	sort.Strings(visibleList)

	root := profile.Template
	root.Metrics = visibleList

	spec := charttpl.Spec{
		Version:          charttpl.VersionV1,
		ContextNamespace: "azure_monitor",
		Groups:           []charttpl.Group{root},
	}
	if err := spec.Validate(); err != nil {
		return fmt.Errorf("%s.template: %w", prefix, err)
	}
	return nil
}

func visibleMetricsForProfile(baseName string, profile Profile) map[string]struct{} {
	visible := make(map[string]struct{})
	for _, metric := range profile.Metrics {
		for _, series := range metric.Series {
			visible[ExportedSeriesName(baseName, metric.ID, series.Aggregation)] = struct{}{}
		}
	}
	for _, state := range profile.ResourceStates {
		for _, option := range state.States {
			visible[ExportedStateSeriesName(baseName, state.ID, option.Name)] = struct{}{}
		}
	}
	return visible
}

func normalizeGroupSelectors(baseName string, visible map[string]struct{}, group *charttpl.Group) {
	if group == nil {
		return
	}

	for i := range group.Charts {
		normalizeChartSelectors(baseName, visible, &group.Charts[i])
	}
	for i := range group.Groups {
		normalizeGroupSelectors(baseName, visible, &group.Groups[i])
	}
}

func normalizeChartSelectors(baseName string, visible map[string]struct{}, chart *charttpl.Chart) {
	if chart == nil {
		return
	}

	for i := range chart.Dimensions {
		chart.Dimensions[i].Selector = normalizeSelector(baseName, visible, chart.Dimensions[i].Selector)
	}
}

func normalizeSelector(baseName string, visible map[string]struct{}, selector string) string {
	metricName, suffix, ok := splitSelectorMetric(selector)
	if !ok || strings.Contains(metricName, ".") {
		return selector
	}

	candidate := baseName + "." + metricName
	if _, ok := visible[candidate]; !ok {
		return selector
	}
	return candidate + suffix
}

func splitSelectorMetric(selector string) (metricName, suffix string, ok bool) {
	selector = strings.TrimSpace(selector)
	if selector == "" || strings.HasPrefix(selector, "{") {
		return "", "", false
	}

	if idx := strings.Index(selector, "{"); idx >= 0 {
		metricName = strings.TrimSpace(selector[:idx])
		if metricName == "" {
			return "", "", false
		}
		return metricName, selector[idx:], true
	}

	return selector, "", true
}

func countChartsInGroup(group charttpl.Group) int {
	total := len(group.Charts)
	for _, child := range group.Groups {
		total += countChartsInGroup(child)
	}
	return total
}
