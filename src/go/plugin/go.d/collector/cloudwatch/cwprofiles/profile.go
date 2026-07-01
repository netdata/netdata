// SPDX-License-Identifier: GPL-3.0-or-later

package cwprofiles

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

// VersionV1 is the supported profile schema version.
const VersionV1 = "v1"

// ContextNamespace is the collector-level chart-template namespace, shared by
// per-profile template validation here and the collector's runtime chart
// assembly so the two cannot drift. Each profile Group additionally carries its
// own context_namespace (e.g. ec2).
const ContextNamespace = "cloudwatch"

// minPeriod is the smallest CloudWatch Period the collector supports, maxPeriod
// the largest. CloudWatch standard-resolution metrics use a Period that is a
// positive multiple of 60s, up to one day; every curated profile uses such a
// Period (60, 300, 86400). Bounding it catches typos, keeps GetMetricData
// windows aligned to published buckets, and prevents int32 Period overflow.
const (
	minPeriod = 60
	maxPeriod = 86400
)

// reservedLabels are identity labels the collector provides on every series; a
// profile dimension MUST NOT reuse them or the label set would collide.
var reservedLabels = map[string]struct{}{"account_id": {}, "region": {}}

var (
	reNamespace  = regexp.MustCompile(`^[A-Za-z0-9/._-]+$`)
	reIdentityID = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	rePercentile = regexp.MustCompile(`^p(\d{1,2}(\.\d{1,2})?|100(\.0{1,2})?)$`)
)

// statStrings maps the named statistic tokens to their GetMetricData Stat
// string. Percentile tokens (p<N>) are passed through verbatim.
var statStrings = map[string]string{
	"average":      "Average",
	"minimum":      "Minimum",
	"maximum":      "Maximum",
	"sum":          "Sum",
	"sample_count": "SampleCount",
}

// Profile is one CloudWatch namespace's curated metric+chart definition. The
// schema is additive-only across phases; the YAML decoder is non-strict, so
// unknown keys are ignored (forward-compat for profiles carrying newer fields).
type Profile struct {
	Version     string         `yaml:"version" json:"version,omitempty"`
	DisplayName string         `yaml:"display_name" json:"display_name,omitempty"`
	Namespace   string         `yaml:"namespace" json:"namespace,omitempty"`
	Period      int            `yaml:"period" json:"period,omitempty"`
	Instance    InstanceSpec   `yaml:"instance" json:"instance"`
	Metrics     []Metric       `yaml:"metrics" json:"metrics,omitempty"`
	Template    charttpl.Group `yaml:"template" json:"template"`
	// Disabled excludes the profile from namespaces.mode auto and exact; it is
	// selected only by mode combined (or by a user-dir copy that drops this field).
	// Omitted decodes to false = enabled, so no pointer is needed.
	Disabled bool `yaml:"disabled,omitempty" json:"disabled,omitempty"`
}

// InstanceSpec declares the exact CloudWatch dimension-NAME set that defines an
// instance for this profile. It is both the ListMetrics filter and the instance
// identity key.
type InstanceSpec struct {
	Dimensions []InstanceDimension `yaml:"dimensions" json:"dimensions,omitempty"`
}

// InstanceDimension maps a CloudWatch dimension name to its Netdata label key.
// The mapping is explicit so the public label contract is unambiguous.
type InstanceDimension struct {
	Name  string `yaml:"name" json:"name,omitempty"`   // CloudWatch dimension name (ListMetrics filter + GetMetricData query)
	Label string `yaml:"label" json:"label,omitempty"` // Netdata label key (identity + chart by_labels)
}

// Metric is one CloudWatch MetricName queried at one or more statistics.
type Metric struct {
	ID         string   `yaml:"id" json:"id,omitempty"`
	MetricName string   `yaml:"metric_name" json:"metric_name,omitempty"`
	Statistics []string `yaml:"statistics" json:"statistics,omitempty"`
	// Rate, when true, presents the value as per-second (value / period). The
	// collector injects divisor=period + float on the chart dimension. It
	// requires a sum statistic.
	Rate bool `yaml:"rate,omitempty" json:"rate,omitempty"`
	// Period optionally overrides the profile-level Period for this metric.
	Period int `yaml:"period,omitempty" json:"period,omitempty"`
}

// Normalize rewrites chart-dimension selector shorthand (e.g. cpu_utilization_average)
// to the fully-qualified series name (<baseName>.cpu_utilization_average), so stock
// profiles can be authored without repeating the profile prefix.
func (p *Profile) Normalize(baseName string) error {
	visible := visibleSeriesForProfile(baseName, p.Metrics)
	normalizeGroupSelectors(baseName, visible, &p.Template)
	return nil
}

// Validate performs full structural and semantic validation of a profile.
func (p Profile) Validate(prefix, baseName string) error {
	var errs []error

	if !IsValidProfileName(baseName) {
		errs = append(errs, fmt.Errorf("%s: profile basename must match %q", prefix, reIdentityID.String()))
	}
	if p.Version != VersionV1 {
		errs = append(errs, fmt.Errorf("%s: 'version' must be %q", prefix, VersionV1))
	}
	if strings.TrimSpace(p.DisplayName) == "" {
		errs = append(errs, fmt.Errorf("%s: 'display_name' is required", prefix))
	}
	if !IsValidNamespace(p.Namespace) {
		errs = append(errs, fmt.Errorf("%s: 'namespace' is invalid (must match %q)", prefix, reNamespace.String()))
	}
	if !isValidPeriod(p.Period) {
		errs = append(errs, fmt.Errorf("%s: 'period' must be a positive multiple of %d seconds", prefix, minPeriod))
	}

	errs = append(errs, validateInstanceDimensions(prefix, p.Instance.Dimensions))

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
	seenMetricNames := map[string]struct{}{}
	for i, metric := range p.Metrics {
		if err := metric.validate(prefix, i); err != nil {
			errs = append(errs, err)
		}

		id := strings.ToLower(strings.TrimSpace(metric.ID))
		if id != "" {
			if _, ok := seenMetricIDs[id]; ok {
				errs = append(errs, fmt.Errorf("%s: duplicate metric id %q", prefix, metric.ID))
			}
			seenMetricIDs[id] = struct{}{}
		}

		name := strings.TrimSpace(metric.MetricName)
		if name != "" {
			if _, ok := seenMetricNames[name]; ok {
				errs = append(errs, fmt.Errorf("%s: duplicate metric_name %q", prefix, metric.MetricName))
			}
			seenMetricNames[name] = struct{}{}
		}
	}

	// The template and by_labels checks rely on valid metrics/dimensions.
	if errors.Join(errs...) == nil {
		errs = append(errs, validateByLabels(prefix, p))
		errs = append(errs, validateChartAlgorithms(prefix, p))
		errs = append(errs, validateTemplate(prefix, baseName, p))
	}

	return errors.Join(errs...)
}

func (m Metric) validate(profilePrefix string, idx int) error {
	prefix := fmt.Sprintf("%s.metrics[%d]", profilePrefix, idx)
	var errs []error

	if !IsValidIdentityID(m.ID) {
		errs = append(errs, fmt.Errorf("%s: 'id' must match %q", prefix, reIdentityID.String()))
	}
	if strings.TrimSpace(m.MetricName) == "" {
		errs = append(errs, fmt.Errorf("%s: 'metric_name' is required", prefix))
	}
	if m.Period != 0 && !isValidPeriod(m.Period) {
		errs = append(errs, fmt.Errorf("%s: 'period' override must be a positive multiple of %d seconds", prefix, minPeriod))
	}
	if len(m.Statistics) == 0 {
		errs = append(errs, fmt.Errorf("%s: 'statistics' must contain at least one statistic", prefix))
	}

	seen := map[string]struct{}{}
	hasSum := false
	for i, stat := range m.Statistics {
		token := NormalizeStatistic(stat)
		if token == "" {
			errs = append(errs, fmt.Errorf("%s.statistics[%d]: %q is not a valid statistic (average|minimum|maximum|sum|sample_count|p<N>)", prefix, i, stat))
			continue
		}
		if token == "sum" {
			hasSum = true
		}
		if _, ok := seen[token]; ok {
			errs = append(errs, fmt.Errorf("%s.statistics[%d]: duplicate statistic %q", prefix, i, stat))
			continue
		}
		seen[token] = struct{}{}
	}

	if m.Rate && !hasSum {
		errs = append(errs, fmt.Errorf("%s: 'rate: true' requires a 'sum' statistic (a per-second rate divides a per-period total)", prefix))
	}

	return errors.Join(errs...)
}

func validateInstanceDimensions(profilePrefix string, dims []InstanceDimension) error {
	if len(dims) == 0 {
		return fmt.Errorf("%s: 'instance.dimensions' must contain at least one dimension", profilePrefix)
	}

	var errs []error
	seenNames := map[string]struct{}{}
	seenLabels := map[string]struct{}{}
	for i, d := range dims {
		prefix := fmt.Sprintf("%s.instance.dimensions[%d]", profilePrefix, i)
		name := strings.TrimSpace(d.Name)
		if name == "" {
			errs = append(errs, fmt.Errorf("%s: 'name' is required", prefix))
		} else if _, ok := seenNames[name]; ok {
			errs = append(errs, fmt.Errorf("%s: duplicate dimension name %q", prefix, d.Name))
		} else {
			seenNames[name] = struct{}{}
		}

		switch {
		case !IsValidIdentityID(d.Label):
			errs = append(errs, fmt.Errorf("%s: 'label' must match %q", prefix, reIdentityID.String()))
		case isReservedLabel(d.Label):
			errs = append(errs, fmt.Errorf("%s: 'label' %q is reserved (account_id and region are collector-provided identity labels)", prefix, d.Label))
		default:
			if _, ok := seenLabels[d.Label]; ok {
				errs = append(errs, fmt.Errorf("%s: duplicate dimension label %q", prefix, d.Label))
			} else {
				seenLabels[d.Label] = struct{}{}
			}
		}
	}
	return errors.Join(errs...)
}

func isReservedLabel(label string) bool {
	_, ok := reservedLabels[label]
	return ok
}

// validateByLabels enforces that every chart's effective instance identity
// (by_labels) includes account_id, region, and all of the profile's instance
// dimension labels. chartengine builds instance identity solely from by_labels,
// so omitting any of these would collapse distinct instances.
func validateByLabels(profilePrefix string, p Profile) error {
	required := make([]string, 0, len(p.Instance.Dimensions)+2)
	required = append(required, "account_id", "region")
	for _, d := range p.Instance.Dimensions {
		required = append(required, d.Label)
	}

	var errs []error
	walkGroupByLabels(profilePrefix+".template", p.Template, nil, required, &errs)
	return errors.Join(errs...)
}

func walkGroupByLabels(path string, group charttpl.Group, inherited []string, required []string, errs *[]error) {
	effective := inherited
	if group.ChartDefaults != nil && group.ChartDefaults.Instances != nil && len(group.ChartDefaults.Instances.ByLabels) > 0 {
		effective = group.ChartDefaults.Instances.ByLabels
	}

	for i, chart := range group.Charts {
		chartLabels := effective
		if chart.Instances != nil && len(chart.Instances.ByLabels) > 0 {
			chartLabels = chart.Instances.ByLabels
		}
		if missing := missingLabels(chartLabels, required); len(missing) > 0 {
			*errs = append(*errs, fmt.Errorf("%s.charts[%d] (%s): instances.by_labels must include %v (missing %v)", path, i, chart.Context, required, missing))
		}
	}
	for i := range group.Groups {
		walkGroupByLabels(fmt.Sprintf("%s.groups[%d]", path, i), group.Groups[i], effective, required, errs)
	}
}

// validateChartAlgorithms enforces that every chart sets algorithm: absolute
// explicitly. CloudWatch returns per-period aggregates (never raw cumulative
// counters), so absolute is always correct; requiring it also prevents
// chartengine's suffix-based incremental inference from ever being relied upon.
func validateChartAlgorithms(profilePrefix string, p Profile) error {
	var errs []error
	walkChartAlgorithms(profilePrefix+".template", p.Template, &errs)
	return errors.Join(errs...)
}

func walkChartAlgorithms(path string, group charttpl.Group, errs *[]error) {
	for i, chart := range group.Charts {
		if chart.Algorithm != "absolute" {
			*errs = append(*errs, fmt.Errorf("%s.charts[%d] (%s): 'algorithm' must be 'absolute'", path, i, chart.Context))
		}
	}
	for i := range group.Groups {
		walkChartAlgorithms(fmt.Sprintf("%s.groups[%d]", path, i), group.Groups[i], errs)
	}
}

func missingLabels(byLabels []string, required []string) []string {
	have := make(map[string]struct{}, len(byLabels))
	excluded := make(map[string]struct{})
	wildcard := false
	for _, l := range byLabels {
		l = strings.TrimSpace(l)
		switch {
		case l == "*":
			wildcard = true
		case strings.HasPrefix(l, "!"):
			excluded[strings.TrimPrefix(l, "!")] = struct{}{}
		default:
			have[l] = struct{}{}
		}
	}

	var missing []string
	for _, r := range required {
		// A required identity label is present unless it is explicitly excluded;
		// the wildcard covers everything else.
		if _, isExcluded := excluded[r]; isExcluded {
			missing = append(missing, r)
			continue
		}
		if wildcard {
			continue
		}
		if _, ok := have[r]; !ok {
			missing = append(missing, r)
		}
	}
	return missing
}

// ExportedSeriesName is the fully-qualified metrix series name for one
// (metric, statistic): <profile>.<metric_id>_<statistic-token>. It is both the
// store instrument name and the chart selector target. statistic MUST already be
// a canonical token (see NormalizeStatistic); every caller normalizes first.
func ExportedSeriesName(profileName, metricID, statistic string) string {
	return strings.TrimSpace(profileName) + "." + strings.TrimSpace(metricID) + "_" + statistic
}

// NormalizeStatistic returns the canonical lower-case statistic token, or ""
// if the input is not a supported statistic.
func NormalizeStatistic(v string) string {
	token := strings.ToLower(strings.TrimSpace(v))
	if token == "" {
		return ""
	}
	if _, ok := statStrings[token]; ok {
		return token
	}
	if rePercentile.MatchString(token) {
		return token
	}
	return ""
}

// StatString maps a statistic token to its GetMetricData Stat string. Named
// tokens map to their CamelCase Stat; any other token (a percentile such as
// p90) is returned verbatim. Callers pass tokens already validated by
// NormalizeStatistic.
func StatString(token string) string {
	if s, ok := statStrings[token]; ok {
		return s
	}
	return token // percentile tokens (p<N>) are used verbatim
}

// EffectivePeriod returns the metric's per-metric period override when set,
// else the profile-level period.
func (p Profile) EffectivePeriod(m Metric) int {
	if m.Period > 0 {
		return m.Period
	}
	return p.Period
}

// MaxEffectivePeriod is the largest effective period across the profile's
// metrics. It drives the period-aware RecentlyActive decision (a profile with
// any long-period metric must not be pruned by ListMetrics RecentlyActive=PT3H).
func (p Profile) MaxEffectivePeriod() int {
	largest := p.Period
	for _, m := range p.Metrics {
		if ep := p.EffectivePeriod(m); ep > largest {
			largest = ep
		}
	}
	return largest
}

// DimensionNames returns the profile's instance dimension CloudWatch names in
// declared order. This order is the canonical ordering for an instance's
// dimension values.
func (p Profile) DimensionNames() []string {
	names := make([]string, len(p.Instance.Dimensions))
	for i, d := range p.Instance.Dimensions {
		names[i] = d.Name
	}
	return names
}

func IsValidIdentityID(v string) bool {
	return reIdentityID.MatchString(strings.TrimSpace(v))
}

func IsValidProfileName(v string) bool {
	return reIdentityID.MatchString(strings.TrimSpace(v))
}

func IsValidNamespace(v string) bool {
	return reNamespace.MatchString(strings.TrimSpace(v))
}

func isValidPeriod(p int) bool {
	return p >= minPeriod && p <= maxPeriod && p%minPeriod == 0
}

func validateTemplate(prefix, baseName string, profile Profile) error {
	visible := visibleSeriesForProfile(baseName, profile.Metrics)
	visibleList := slices.Sorted(maps.Keys(visible))

	root := profile.Template
	root.Metrics = visibleList

	spec := charttpl.Spec{
		Version:          charttpl.VersionV1,
		ContextNamespace: ContextNamespace,
		Groups:           []charttpl.Group{root},
	}
	if err := spec.Validate(); err != nil {
		return fmt.Errorf("%s.template: %w", prefix, err)
	}
	return nil
}

// visibleSeriesForProfile is the set of fully-qualified series names a profile
// exports: one per (metric, statistic).
func visibleSeriesForProfile(baseName string, metrics []Metric) map[string]struct{} {
	visible := make(map[string]struct{})
	for _, metric := range metrics {
		for _, stat := range metric.Statistics {
			token := NormalizeStatistic(stat)
			if token == "" {
				continue
			}
			visible[ExportedSeriesName(baseName, metric.ID, token)] = struct{}{}
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
	seriesName, suffix, ok := splitSelectorSeries(selector)
	if !ok || strings.Contains(seriesName, ".") {
		return selector
	}

	candidate := baseName + "." + seriesName
	if _, ok := visible[candidate]; !ok {
		return selector
	}
	return candidate + suffix
}

func splitSelectorSeries(selector string) (seriesName, suffix string, ok bool) {
	selector = strings.TrimSpace(selector)
	if selector == "" || strings.HasPrefix(selector, "{") {
		return "", "", false
	}
	if idx := strings.Index(selector, "{"); idx >= 0 {
		seriesName = strings.TrimSpace(selector[:idx])
		if seriesName == "" {
			return "", "", false
		}
		return seriesName, selector[idx:], true
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
