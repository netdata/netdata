// SPDX-License-Identifier: GPL-3.0-or-later

package cwprofiles

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsregion"
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

// Profile is one CloudWatch namespace's curated metric and chart definition.
type Profile struct {
	Version     string `yaml:"version" json:"version,omitempty"`
	DisplayName string `yaml:"display_name" json:"display_name,omitempty"`
	Namespace   string `yaml:"namespace" json:"namespace,omitempty"`
	// SupportedRegions restricts profiles for services whose CloudWatch metrics
	// are published only in specific regions. Nil means unrestricted.
	SupportedRegions []string `yaml:"supported_regions,omitempty" json:"supported_regions,omitempty"`
	Period           int      `yaml:"period" json:"period,omitempty"`
	// PublicationDelay is the profile-level settling delay for metrics that are
	// published after their aggregation period closes. Omitted profiles inherit
	// the collector's built-in fallback.
	PublicationDelay *confopt.LongDuration `yaml:"publication_delay,omitempty" json:"publication_delay,omitempty"`
	Instance         InstanceSpec          `yaml:"instance" json:"instance"`
	Metrics          []Metric              `yaml:"metrics" json:"metrics,omitempty"`
	Template         charttpl.Group        `yaml:"template" json:"template"`
	// Disabled excludes the profile from the default-enabled set. A collection rule
	// can still include it explicitly by profile name. Omitted decodes to false.
	Disabled bool `yaml:"disabled,omitempty" json:"disabled,omitempty"`
}

func (p Profile) SupportsRegion(region string) bool {
	if len(p.SupportedRegions) == 0 {
		return true
	}
	return slices.Contains(p.SupportedRegions, awsregion.Normalize(region))
}

// InstanceSpec declares the exact CloudWatch dimension-NAME set that defines an
// instance for this profile. The set is the ListMetrics filter and the
// GetMetricData query dimensions; identifying dimensions (those with a label)
// form the Netdata instance identity, while constant match-and-query-only
// dimensions do not.
type InstanceSpec struct {
	Dimensions []InstanceDimension `yaml:"dimensions" json:"dimensions,omitempty"`
}

// InstanceDimension is one CloudWatch dimension of a profile's instance identity.
// It is either:
//   - an IDENTIFYING dimension (Label set): its value is emitted as a Netdata
//     label and is required in chart by_labels; or
//   - a MATCH-AND-QUERY-ONLY dimension (Constant set): still part of the exact
//     dimension-NAME-set match and the GetMetricData query, but discovery keeps
//     only metrics whose value for it equals Constant (fail-closed), and it is NOT
//     emitted as a label / NOT required in by_labels.
//
// Use Constant for a dimension that is constant across instances and so carries no
// Netdata identity (e.g. CloudFront's Region="Global"). Exactly one of Label or
// Constant must be set.
type InstanceDimension struct {
	Name     string  `yaml:"name" json:"name,omitempty"`
	Label    string  `yaml:"label" json:"label,omitempty"`
	Constant *string `yaml:"constant,omitempty" json:"constant,omitempty"`
}

// IsConstant reports whether the dimension is match-and-query-only (a pinned
// constant value) rather than an identifying label.
func (d InstanceDimension) IsConstant() bool { return d.Constant != nil }

// Metric is one CloudWatch MetricName queried at one or more statistics.
type Metric struct {
	ID         string   `yaml:"id" json:"id,omitempty"`
	MetricName string   `yaml:"metric_name" json:"metric_name,omitempty"`
	Statistics []string `yaml:"statistics" json:"statistics,omitempty"`
	// Rate, when true, presents each per-period total as a per-second value using
	// the effective rule period. It requires a sum or sample_count statistic.
	Rate bool `yaml:"rate,omitempty" json:"rate,omitempty"`
	// Period optionally overrides the profile-level Period for this metric.
	Period int `yaml:"period,omitempty" json:"period,omitempty"`
	// NilAsZero controls how a query that returns no datapoint is recorded: as 0
	// (true) or as a gap (false). Unset defaults to Rate — sum/count metrics read
	// no-data as 0 ("no activity"), gauges gap — and an explicit value overrides
	// it. Pointer so "unset" is distinguishable from an explicit false.
	NilAsZero *bool `yaml:"nil_as_zero,omitempty" json:"nil_as_zero,omitempty"`
}

// IsPerPeriodTotal reports whether a statistic is a per-period total (sum or
// sample_count) — the statistics that divide by the period to form a per-second
// rate, and that read a no-datapoint result as 0 rather than a gap.
func IsPerPeriodTotal(token string) bool {
	return token == "sum" || token == "sample_count"
}

// EmitZeroOnNoData reports whether a query returning no datapoint for this metric
// at the given statistic should be recorded as 0 (true) or left as a gap (false).
// An explicit NilAsZero applies to every statistic; otherwise the default is
// per-statistic: a rate metric's per-period totals (sum/sample_count) read no-data
// as 0 ("no activity"), while gauges and per-observation aggregates (average,
// maximum, percentiles) gap.
func (m Metric) EmitZeroOnNoData(token string) bool {
	if m.NilAsZero != nil {
		return *m.NilAsZero
	}
	return m.Rate && IsPerPeriodTotal(token)
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
	if p.SupportedRegions != nil {
		if len(p.SupportedRegions) == 0 {
			errs = append(errs, fmt.Errorf("%s: 'supported_regions' must not be empty when set", prefix))
		}
		seen := make(map[string]struct{}, len(p.SupportedRegions))
		for i, region := range p.SupportedRegions {
			if canonical := awsregion.Normalize(region); region != canonical {
				errs = append(errs, fmt.Errorf("%s.supported_regions[%d]: %q is not canonical; use %q", prefix, i, region, canonical))
				continue
			}
			if !awsregion.Valid(region) {
				errs = append(errs, fmt.Errorf("%s.supported_regions[%d]: %q is not a valid AWS region", prefix, i, region))
				continue
			}
			if _, ok := seen[region]; ok {
				errs = append(errs, fmt.Errorf("%s: duplicate supported region %q", prefix, region))
				continue
			}
			seen[region] = struct{}{}
		}
	}
	if !isValidPeriod(p.Period) {
		errs = append(errs, fmt.Errorf("%s: 'period' must be a positive multiple of %d seconds", prefix, minPeriod))
	}
	if p.PublicationDelay != nil && p.PublicationDelay.Duration() < 0 {
		errs = append(errs, fmt.Errorf("%s: 'publication_delay' must not be negative", prefix))
	}
	if p.PublicationDelay != nil && p.PublicationDelay.Duration()%time.Second != 0 {
		errs = append(errs, fmt.Errorf("%s: 'publication_delay' must use whole seconds", prefix))
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
	} else if m.MetricName != strings.TrimSpace(m.MetricName) {
		errs = append(errs, fmt.Errorf("%s: 'metric_name' must not have surrounding whitespace", prefix))
	}
	if m.Period != 0 && !isValidPeriod(m.Period) {
		errs = append(errs, fmt.Errorf("%s: 'period' override must be a positive multiple of %d seconds", prefix, minPeriod))
	}
	if len(m.Statistics) == 0 {
		errs = append(errs, fmt.Errorf("%s: 'statistics' must contain at least one statistic", prefix))
	}

	seen := map[string]struct{}{}
	hasSum, hasSampleCount := false, false
	for i, stat := range m.Statistics {
		token := NormalizeStatistic(stat)
		if token == "" {
			errs = append(errs, fmt.Errorf("%s.statistics[%d]: %q is not a valid statistic (average|minimum|maximum|sum|sample_count|p<N>)", prefix, i, stat))
			continue
		}
		switch token {
		case "sum":
			hasSum = true
		case "sample_count":
			hasSampleCount = true
		}
		if _, ok := seen[token]; ok {
			errs = append(errs, fmt.Errorf("%s.statistics[%d]: duplicate statistic %q", prefix, i, stat))
			continue
		}
		seen[token] = struct{}{}
	}

	if m.Rate && !hasSum && !hasSampleCount {
		errs = append(errs, fmt.Errorf("%s: 'rate: true' requires a 'sum' or 'sample_count' statistic (a per-second rate divides a per-period total)", prefix))
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
		// Name is matched against CloudWatch dimension names verbatim (DimensionNames),
		// so a stray leading/trailing space would silently match nothing — reject it
		// (internal spaces are legitimate, e.g. MSK's "Cluster Name").
		if d.Name == "" || d.Name != strings.TrimSpace(d.Name) {
			errs = append(errs, fmt.Errorf("%s: 'name' must be non-empty with no leading/trailing whitespace (it is matched against CloudWatch dimension names verbatim)", prefix))
		} else if _, ok := seenNames[d.Name]; ok {
			errs = append(errs, fmt.Errorf("%s: duplicate dimension name %q", prefix, d.Name))
		} else {
			seenNames[d.Name] = struct{}{}
		}

		switch {
		case d.Constant != nil && d.Label != "":
			errs = append(errs, fmt.Errorf("%s: set exactly one of 'label' or 'constant', not both", prefix))
		case d.Constant != nil:
			// Match-and-query-only dimension: it has no label, so the label rules
			// (charset, reserved, uniqueness) do not apply. It is compared verbatim
			// to the CloudWatch dimension value at discovery, so a stray space would
			// silently match nothing — reject empty or whitespace-padded values.
			if c := *d.Constant; c == "" || c != strings.TrimSpace(c) {
				errs = append(errs, fmt.Errorf("%s: 'constant' must be non-empty with no leading/trailing whitespace (it is matched against the CloudWatch dimension value verbatim)", prefix))
			}
		case !IsValidIdentityID(d.Label):
			errs = append(errs, fmt.Errorf("%s: 'label' must match %q (or set 'constant' for a match-and-query-only dimension)", prefix, reIdentityID.String()))
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
// (by_labels) includes account_id, region, and all of the profile's identifying
// dimension labels (constant match-and-query-only dimensions are excluded — they
// carry no identity). chartengine builds instance identity solely from by_labels,
// so omitting any required label would collapse distinct instances.
func validateByLabels(profilePrefix string, p Profile) error {
	required := make([]string, 0, len(p.Instance.Dimensions)+2)
	required = append(required, "account_id", "region")
	for _, d := range p.Instance.Dimensions {
		if !d.IsConstant() { // constant dims are not labeled, so not required in by_labels
			required = append(required, d.Label)
		}
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

// IsValidNamespace validates the namespace verbatim (no trimming). Namespace is
// used as-is as the ListMetrics filter, so surrounding whitespace must fail
// validation as a typo rather than pass here and then silently match nothing.
func IsValidNamespace(v string) bool {
	return reNamespace.MatchString(v)
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

// chartIDs returns every chart id declared in a template group, descending into
// nested groups. Chart ids must be globally unique across the active profile set:
// chartengine keys charts by id and silently drops a cross-template id collision.
func chartIDs(g charttpl.Group) []string {
	var ids []string
	for _, ch := range g.Charts {
		if id := strings.TrimSpace(ch.ID); id != "" {
			ids = append(ids, id)
		}
	}
	for i := range g.Groups {
		ids = append(ids, chartIDs(g.Groups[i])...)
	}
	return ids
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
	if !ok {
		return selector
	}

	// Prefix the profile basename only when the result is one of this profile's
	// visible series. An already-qualified selector produces a double-prefixed
	// candidate that is not visible, so it is left unchanged. Relying on the visible
	// lookup rather than a "series name contains a dot" heuristic also lets a
	// decimal-percentile shorthand (e.g. latency_p99.9, whose statistic token carries
	// a dot) normalize correctly instead of being mistaken for already-qualified.
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
