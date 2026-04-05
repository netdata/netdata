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

var (
	reResourceType = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
	reIdentityID   = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

var SupportedTimeGrains = map[string]time.Duration{
	"PT1M":  time.Minute,
	"PT5M":  5 * time.Minute,
	"PT15M": 15 * time.Minute,
	"PT30M": 30 * time.Minute,
	"PT1H":  time.Hour,
}

type Profile struct {
	DisplayName     string         `yaml:"display_name" json:"display_name,omitempty"`
	ResourceType    string         `yaml:"resource_type" json:"resource_type,omitempty"`
	MetricNamespace string         `yaml:"metric_namespace,omitempty" json:"metric_namespace,omitempty"`
	Metrics         []Metric       `yaml:"metrics" json:"metrics,omitempty"`
	Template        charttpl.Group `yaml:"template" json:"template"`
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
	visibleMetrics := visibleMetricsForProfile(baseName, p.Metrics)
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

	if len(errs) == 0 {
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
	visibleMetrics := visibleMetricsForProfile(baseName, profile.Metrics)
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

func visibleMetricsForProfile(baseName string, metrics []Metric) map[string]struct{} {
	visible := make(map[string]struct{})
	for _, metric := range metrics {
		for _, series := range metric.Series {
			visible[ExportedSeriesName(baseName, metric.ID, series.Aggregation)] = struct{}{}
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
