// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

const (
	autogenTemplatePrefix = "__autogen__:"
)

type autogenRoute struct {
	chartID           string
	chartName         string
	title             string
	dimensionName     string
	dimensionKeyLabel string
	algorithm         program.Algorithm
	units             string
	chartType         program.ChartType
	family            string
	priority          int
	contextName       string
	staticDimension   bool
	float             bool
}

type autogenSource struct {
	seriesName   string
	familyName   string
	measureField string
}

type autogenSourceBuilder func(
	source autogenSource,
	labels metrix.LabelView,
	meta metrix.SeriesMeta,
	policy AutogenPolicy,
	typeIDPrefix string,
) (autogenRoute, bool, error)

type autogenRoleBuilder func(
	source autogenSource,
	labels metrix.LabelView,
	policy AutogenPolicy,
	typeIDPrefix string,
) (autogenRoute, bool, error)

var autogenSourceBuilders = map[metrix.MetricKind]autogenSourceBuilder{
	metrix.MetricKindHistogram:  buildHistogramAutogenRoute,
	metrix.MetricKindSummary:    buildSummaryAutogenRoute,
	metrix.MetricKindStateSet:   buildStateSetAutogenRoute,
	metrix.MetricKindMeasureSet: buildMeasureSetAutogenRoute,
}

var histogramRoleBuilders = map[metrix.FlattenRole]autogenRoleBuilder{
	metrix.FlattenRoleHistogramBucket: buildHistogramBucketAutogenRoute,
	metrix.FlattenRoleHistogramCount:  buildHistogramCountAutogenRoute,
	metrix.FlattenRoleHistogramSum:    buildHistogramSumAutogenRoute,
}

var summaryRoleBuilders = map[metrix.FlattenRole]autogenRoleBuilder{
	metrix.FlattenRoleSummaryQuantile: buildSummaryQuantileAutogenRoute,
	metrix.FlattenRoleSummaryCount:    buildSummaryCountAutogenRoute,
	metrix.FlattenRoleSummarySum:      buildSummarySumAutogenRoute,
}

func (e *Engine) resolveAutogenRoute(
	reader metrix.Reader,
	metricName string,
	labels metrix.LabelView,
	meta metrix.SeriesMeta,
) ([]routeBinding, bool, error) {
	if e == nil {
		return nil, false, fmt.Errorf("chartengine: nil engine")
	}
	policy := e.state.cfg.autogen
	if !policy.Enabled {
		return nil, false, nil
	}

	source, ok := resolveAutogenSource(metricName, labels, meta)
	if !ok {
		return nil, false, nil
	}
	if !autogenRulesSelect(e.state.cfg.autogenRules, source.familyName, labels) {
		return nil, false, nil
	}

	namespace := e.state.cfg.autogenContextNamespace

	route, ok, err := buildAutogenRoute(source, labels, meta, policy, e.state.cfg.autogenTypeID)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	if metricMeta, ok := autogenMetricMeta(reader, source); ok {
		route = applyAutogenMetricMeta(route, metricMeta, meta)
	}
	route.priority = effectiveChartPriority(route.priority)
	title := route.title
	if title == "" {
		title = getAutogenChartTitle(route.chartName)
	}
	return []routeBinding{
		{
			ChartTemplateID:   autogenTemplatePrefix + route.chartID,
			ChartID:           route.chartID,
			DimensionIndex:    0,
			DimensionName:     route.dimensionName,
			DimensionKeyLabel: route.dimensionKeyLabel,
			Algorithm:         route.algorithm,
			Hidden:            false,
			Multiplier:        1,
			Divisor:           1,
			Float:             route.float,
			Static:            route.staticDimension,
			Inferred:          false,
			Autogen:           true,
			Meta: program.ChartMeta{
				Title:     title,
				Family:    route.family,
				Context:   getAutogenChartContext(namespace, route.contextName),
				Units:     route.units,
				Algorithm: route.algorithm,
				Type:      route.chartType,
				Priority:  route.priority,
			},
			Lifecycle: autogenLifecyclePolicy(policy),
		},
	}, true, nil
}

func autogenRulesSelect(rules []charttpl.ValidatedAutogenRule, metricName string, labels metrix.LabelView) bool {
	for _, rule := range rules {
		if rule.ScopeMatches(metricName) && !rule.Selects(metricName, labels) {
			return false
		}
	}
	return true
}

func buildAutogenRoute(
	source autogenSource,
	labels metrix.LabelView,
	meta metrix.SeriesMeta,
	policy AutogenPolicy,
	typeIDPrefix string,
) (autogenRoute, bool, error) {
	if strings.TrimSpace(source.seriesName) == "" {
		return autogenRoute{}, false, nil
	}
	if builder, ok := autogenSourceBuilders[meta.SourceKind]; ok {
		return builder(source, labels, meta, policy, typeIDPrefix)
	}
	return buildScalarAutogenRoute(source.seriesName, labels, meta, policy, typeIDPrefix)
}

func autogenMetricMeta(reader metrix.Reader, source autogenSource) (metrix.MetricMeta, bool) {
	if reader == nil {
		return metrix.MetricMeta{}, false
	}
	for _, name := range sourceMetricNames(source) {
		if name == "" {
			continue
		}
		if metricMeta, ok := reader.MetricMeta(name); ok {
			return metricMeta, true
		}
	}
	return metrix.MetricMeta{}, false
}

func sourceMetricNames(source autogenSource) []string {
	if source.familyName == "" || source.familyName == source.seriesName {
		return []string{source.seriesName}
	}
	return []string{source.familyName, source.seriesName}
}

func resolveAutogenSource(metricName string, labels metrix.LabelView, meta metrix.SeriesMeta) (autogenSource, bool) {
	if strings.TrimSpace(metricName) == "" {
		return autogenSource{}, false
	}

	source := autogenSource{
		seriesName: metricName,
		familyName: sourceMetricName(metricName, meta),
	}
	if meta.SourceKind != metrix.MetricKindMeasureSet {
		return source, true
	}

	familyName, fieldName, ok := resolveMeasureSetAutogenSource(metricName, labels)
	if !ok {
		return autogenSource{}, false
	}
	source.familyName = familyName
	source.measureField = fieldName
	return source, true
}

func sourceMetricName(metricName string, meta metrix.SeriesMeta) string {
	switch meta.SourceKind {
	case metrix.MetricKindHistogram:
		switch meta.FlattenRole {
		case metrix.FlattenRoleHistogramBucket:
			return trimAutogenSourceSuffix(metricName, "_bucket")
		case metrix.FlattenRoleHistogramCount:
			return trimAutogenSourceSuffix(metricName, "_count")
		case metrix.FlattenRoleHistogramSum:
			return trimAutogenSourceSuffix(metricName, "_sum")
		}
	case metrix.MetricKindSummary:
		switch meta.FlattenRole {
		case metrix.FlattenRoleSummaryCount:
			return trimAutogenSourceSuffix(metricName, "_count")
		case metrix.FlattenRoleSummarySum:
			return trimAutogenSourceSuffix(metricName, "_sum")
		}
	}
	return metricName
}

func trimAutogenSourceSuffix(metricName, suffix string) string {
	sourceName := strings.TrimSuffix(metricName, suffix)
	if sourceName == "" {
		return metricName
	}
	return sourceName
}

func applyAutogenMetricMeta(route autogenRoute, meta metrix.MetricMeta, seriesMeta metrix.SeriesMeta) autogenRoute {
	if title := strings.TrimSpace(meta.Description); title != "" {
		route.title = title
	}
	if family := strings.TrimSpace(meta.ChartFamily); family != "" {
		route.family = family
	}
	if meta.ChartPriority > 0 {
		route.priority = meta.ChartPriority
	}
	if unit := strings.TrimSpace(meta.Unit); unit != "" && allowAutogenUnitOverride(seriesMeta) {
		route.units = normalizeAutogenUnitByAlgorithm(unit, route.algorithm)
		route.chartType = chartTypeFromUnits(route.units)
	}
	route.float = meta.Float
	return route
}

func allowAutogenUnitOverride(meta metrix.SeriesMeta) bool {
	if meta.SourceKind == metrix.MetricKindStateSet {
		return false
	}
	if meta.SourceKind == metrix.MetricKindHistogram {
		return meta.FlattenRole != metrix.FlattenRoleHistogramBucket &&
			meta.FlattenRole != metrix.FlattenRoleHistogramCount
	}
	if meta.SourceKind == metrix.MetricKindSummary {
		return meta.FlattenRole != metrix.FlattenRoleSummaryCount
	}
	return true
}

func normalizeAutogenUnitByAlgorithm(unit string, alg program.Algorithm) string {
	unit = strings.TrimSpace(unit)
	if unit == "" {
		return unit
	}
	if alg != program.AlgorithmIncremental {
		return unit
	}
	switch unit {
	case "seconds", "time":
		return unit
	}
	if strings.HasSuffix(unit, "/s") {
		return unit
	}
	return unit + "/s"
}

func buildHistogramAutogenRoute(
	source autogenSource,
	labels metrix.LabelView,
	meta metrix.SeriesMeta,
	policy AutogenPolicy,
	typeIDPrefix string,
) (autogenRoute, bool, error) {
	builder, ok := histogramRoleBuilders[meta.FlattenRole]
	if !ok {
		return autogenRoute{}, false, nil
	}
	return builder(source, labels, policy, typeIDPrefix)
}

func buildSummaryAutogenRoute(
	source autogenSource,
	labels metrix.LabelView,
	meta metrix.SeriesMeta,
	policy AutogenPolicy,
	typeIDPrefix string,
) (autogenRoute, bool, error) {
	builder, ok := summaryRoleBuilders[meta.FlattenRole]
	if !ok {
		return autogenRoute{}, false, nil
	}
	return builder(source, labels, policy, typeIDPrefix)
}

func buildHistogramBucketAutogenRoute(
	source autogenSource,
	labels metrix.LabelView,
	policy AutogenPolicy,
	typeIDPrefix string,
) (autogenRoute, bool, error) {
	baseName := source.familyName
	upperBound, ok := labels.Get(metrix.HistogramBucketLabel)
	if !ok || strings.TrimSpace(upperBound) == "" {
		return autogenRoute{}, false, nil
	}
	chartID := buildJoinedLabelAutogenID(baseName, labels, map[string]struct{}{
		metrix.HistogramBucketLabel: {},
	})
	if !fitsTypeIDBudget(policy.MaxTypeIDLen, typeIDPrefix, chartID) {
		return autogenRoute{}, false, nil
	}
	return autogenRoute{
		chartID:           chartID,
		chartName:         baseName,
		dimensionName:     upperBound,
		dimensionKeyLabel: metrix.HistogramBucketLabel,
		algorithm:         program.AlgorithmIncremental,
		units:             "observations/s",
		chartType:         program.ChartTypeHeatmap,
		family:            getAutogenChartFamily(baseName),
		contextName:       baseName,
		staticDimension:   false,
	}, true, nil
}

func buildHistogramCountAutogenRoute(
	source autogenSource,
	labels metrix.LabelView,
	policy AutogenPolicy,
	typeIDPrefix string,
) (autogenRoute, bool, error) {
	return buildCounterComponentAutogenRoute(source.familyName, "_count", labels, policy, typeIDPrefix, "events/s")
}

func buildHistogramSumAutogenRoute(
	source autogenSource,
	labels metrix.LabelView,
	policy AutogenPolicy,
	typeIDPrefix string,
) (autogenRoute, bool, error) {
	return buildCounterComponentAutogenRoute(
		source.familyName,
		"_sum",
		labels,
		policy,
		typeIDPrefix,
		getAutogenCounterUnits(source.familyName),
	)
}

func buildSummaryQuantileAutogenRoute(
	source autogenSource,
	labels metrix.LabelView,
	policy AutogenPolicy,
	typeIDPrefix string,
) (autogenRoute, bool, error) {
	quantile, ok := labels.Get(metrix.SummaryQuantileLabel)
	if !ok || strings.TrimSpace(quantile) == "" {
		return autogenRoute{}, false, nil
	}
	chartID := buildJoinedLabelAutogenID(source.familyName, labels, map[string]struct{}{
		metrix.SummaryQuantileLabel: {},
	})
	if !fitsTypeIDBudget(policy.MaxTypeIDLen, typeIDPrefix, chartID) {
		return autogenRoute{}, false, nil
	}
	units := getAutogenSummaryUnits(source.familyName)
	return autogenRoute{
		chartID:           chartID,
		chartName:         source.familyName,
		dimensionName:     "quantile_" + quantile,
		dimensionKeyLabel: metrix.SummaryQuantileLabel,
		algorithm:         program.AlgorithmAbsolute,
		units:             units,
		chartType:         chartTypeFromUnits(units),
		family:            getAutogenChartFamily(source.familyName),
		contextName:       source.familyName,
		staticDimension:   false,
	}, true, nil
}

func buildSummaryCountAutogenRoute(
	source autogenSource,
	labels metrix.LabelView,
	policy AutogenPolicy,
	typeIDPrefix string,
) (autogenRoute, bool, error) {
	return buildCounterComponentAutogenRoute(source.familyName, "_count", labels, policy, typeIDPrefix, "events/s")
}

func buildSummarySumAutogenRoute(
	source autogenSource,
	labels metrix.LabelView,
	policy AutogenPolicy,
	typeIDPrefix string,
) (autogenRoute, bool, error) {
	return buildCounterComponentAutogenRoute(
		source.familyName,
		"_sum",
		labels,
		policy,
		typeIDPrefix,
		getAutogenCounterUnits(source.familyName),
	)
}

func buildCounterComponentAutogenRoute(
	baseName string,
	suffix string,
	labels metrix.LabelView,
	policy AutogenPolicy,
	typeIDPrefix string,
	units string,
) (autogenRoute, bool, error) {
	chartName := baseName + suffix
	chartID := buildJoinedLabelAutogenID(chartName, labels, nil)
	if !fitsTypeIDBudget(policy.MaxTypeIDLen, typeIDPrefix, chartID) {
		return autogenRoute{}, false, nil
	}
	return autogenRoute{
		chartID:         chartID,
		chartName:       baseName,
		dimensionName:   autogenDimensionName(chartName),
		algorithm:       program.AlgorithmIncremental,
		units:           units,
		chartType:       chartTypeFromUnits(units),
		family:          getAutogenChartFamily(baseName),
		contextName:     chartName,
		staticDimension: true,
	}, true, nil
}

func buildStateSetAutogenRoute(
	source autogenSource,
	labels metrix.LabelView,
	meta metrix.SeriesMeta,
	policy AutogenPolicy,
	typeIDPrefix string,
) (autogenRoute, bool, error) {
	if meta.FlattenRole != metrix.FlattenRoleStateSetState {
		return autogenRoute{}, false, nil
	}
	state, ok := labels.Get(source.familyName)
	if !ok || strings.TrimSpace(state) == "" {
		return autogenRoute{}, false, nil
	}
	chartID := buildJoinedLabelAutogenID(source.familyName, labels, map[string]struct{}{
		source.familyName: {},
	})
	if !fitsTypeIDBudget(policy.MaxTypeIDLen, typeIDPrefix, chartID) {
		return autogenRoute{}, false, nil
	}
	return autogenRoute{
		chartID:           chartID,
		chartName:         source.familyName,
		dimensionName:     state,
		dimensionKeyLabel: source.familyName,
		algorithm:         program.AlgorithmAbsolute,
		units:             "state",
		chartType:         program.ChartTypeLine,
		family:            getAutogenChartFamily(source.familyName),
		contextName:       source.familyName,
		staticDimension:   false,
	}, true, nil
}

func buildMeasureSetAutogenRoute(
	source autogenSource,
	labels metrix.LabelView,
	meta metrix.SeriesMeta,
	policy AutogenPolicy,
	typeIDPrefix string,
) (autogenRoute, bool, error) {
	if meta.FlattenRole != metrix.FlattenRoleMeasureSetField {
		return autogenRoute{}, false, nil
	}
	if source.familyName == "" || source.measureField == "" {
		return autogenRoute{}, false, nil
	}
	chartID := buildJoinedLabelAutogenID(source.familyName, labels, map[string]struct{}{
		metrix.MeasureSetFieldLabel: {},
	})
	if !fitsTypeIDBudget(policy.MaxTypeIDLen, typeIDPrefix, chartID) {
		return autogenRoute{}, false, nil
	}
	algorithm := program.AlgorithmAbsolute
	units := getAutogenGaugeUnits(source.familyName)
	if meta.Kind == metrix.MetricKindCounter {
		algorithm = program.AlgorithmIncremental
		units = getAutogenCounterUnits(source.familyName)
	}
	return autogenRoute{
		chartID:           chartID,
		chartName:         source.familyName,
		dimensionName:     source.measureField,
		dimensionKeyLabel: metrix.MeasureSetFieldLabel,
		algorithm:         algorithm,
		units:             units,
		chartType:         chartTypeFromUnits(units),
		family:            getAutogenChartFamily(source.familyName),
		contextName:       source.familyName,
		staticDimension:   false,
	}, true, nil
}

func resolveMeasureSetAutogenSource(metricName string, labels metrix.LabelView) (string, string, bool) {
	if strings.TrimSpace(metricName) == "" {
		return "", "", false
	}
	if labels == nil {
		return "", "", false
	}

	fieldName, ok := labels.Get(metrix.MeasureSetFieldLabel)
	if !ok || strings.TrimSpace(fieldName) == "" {
		return "", "", false
	}

	suffix := "_" + fieldName
	if !strings.HasSuffix(metricName, suffix) {
		return "", "", false
	}

	sourceName := strings.TrimSuffix(metricName, suffix)
	if strings.TrimSpace(sourceName) == "" {
		return "", "", false
	}
	return sourceName, fieldName, true
}

func buildScalarAutogenRoute(
	metricName string,
	labels metrix.LabelView,
	meta metrix.SeriesMeta,
	policy AutogenPolicy,
	typeIDPrefix string,
) (autogenRoute, bool, error) {
	chartID := buildJoinedLabelAutogenID(metricName, labels, nil)
	if !fitsTypeIDBudget(policy.MaxTypeIDLen, typeIDPrefix, chartID) {
		return autogenRoute{}, false, nil
	}
	algorithm := program.AlgorithmAbsolute
	units := getAutogenGaugeUnits(metricName)
	if meta.Kind == metrix.MetricKindCounter {
		algorithm = program.AlgorithmIncremental
		units = getAutogenCounterUnits(metricName)
	}
	return autogenRoute{
		chartID:         chartID,
		chartName:       metricName,
		dimensionName:   autogenDimensionName(metricName),
		algorithm:       algorithm,
		units:           units,
		chartType:       chartTypeFromUnits(units),
		family:          getAutogenChartFamily(metricName),
		contextName:     metricName,
		staticDimension: true,
	}, true, nil
}

func fitsTypeIDBudget(maxLen int, typeIDPrefix, chartID string) bool {
	if maxLen <= 0 {
		maxLen = defaultMaxTypeIDLen
	}
	if strings.TrimSpace(typeIDPrefix) == "" {
		return len(chartID) <= maxLen
	}
	return len(typeIDPrefix)+1+len(chartID) <= maxLen
}

func buildJoinedLabelAutogenID(metricName string, labels metrix.LabelView, skipKeys map[string]struct{}) string {
	var b strings.Builder
	b.Grow(len(metricName) + labels.Len()*8)
	b.WriteString(metricName)
	labels.Range(func(key, value string) bool {
		if key == "" || value == "" {
			return true
		}
		if _, skip := skipKeys[key]; skip {
			return true
		}
		b.WriteByte('-')
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(normalizeAutogenLabelValue(value))
		return true
	})
	return b.String()
}

func normalizeAutogenLabelValue(value string) string {
	if strings.IndexByte(value, '\\') != -1 {
		v := decodeAutogenLabelValue(value)
		switch {
		case strings.IndexByte(v, '\\') != -1:
			value = strings.ReplaceAll(v, `\`, "_")
		case hasControlChars(v):
			// Keep raw when unquoting introduces controls (e.g. \b => backspace).
		default:
			value = v
		}
	}
	return sanitizeChartIDLabelValue(value)
}

func hasControlChars(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func decodeAutogenLabelValue(value string) string {
	v, err := strconv.Unquote("\"" + value + "\"")
	if err != nil {
		return value
	}
	return v
}

func getAutogenChartTitle(metricName string) string {
	return fmt.Sprintf("Metric \"%s\"", metricName)
}

// getAutogenChartContext builds an autogen chart context, prefixed by the spec's root
// context_namespace when set ("prometheus" + "foo" -> "prometheus.foo"), matching the
// template compiler's "." join. Empty namespace -> the bare metric name (unchanged).
func getAutogenChartContext(namespace, metricName string) string {
	metricName = strings.TrimSpace(metricName)
	if metricName == "" {
		metricName = "metric"
	}
	if namespace = strings.TrimSpace(namespace); namespace != "" {
		return namespace + "." + metricName
	}
	return metricName
}

func autogenDimensionName(metricName string) string {
	metricName = strings.TrimSpace(metricName)
	if metricName == "" {
		return metricName
	}
	idx := strings.LastIndexByte(metricName, '.')
	if idx == -1 || idx == len(metricName)-1 {
		return metricName
	}
	return metricName[idx+1:]
}

func getAutogenChartFamily(metric string) string {
	if strings.HasPrefix(metric, "go_") {
		return "go"
	}
	if strings.HasPrefix(metric, "process_") {
		return "process"
	}
	parts := strings.SplitN(metric, "_", 3)
	family := metric
	if len(parts) >= 3 {
		family = parts[0] + "_" + parts[1]
	}
	i := len(family) - 1
	for i >= 0 && family[i] >= '0' && family[i] <= '9' {
		i--
	}
	if i > 0 {
		return family[:i+1]
	}
	return family
}

func getAutogenGaugeUnits(metric string) string {
	return getAutogenMetricUnits(metric)
}

func getAutogenCounterUnits(metric string) string {
	units := getAutogenMetricUnits(metric)
	switch units {
	case "seconds", "time":
	default:
		units += "/s"
	}
	return units
}

func getAutogenSummaryUnits(metric string) string {
	return getAutogenGaugeUnits(metric)
}

func getAutogenMetricUnits(metric string) string {
	idx := strings.LastIndexByte(metric, '_')
	if idx == -1 {
		return "events"
	}
	switch suffix := metric[idx:]; suffix {
	case "_total", "_sum", "_count", "_ratio":
		return getAutogenMetricUnits(metric[:idx])
	}
	switch units := metric[idx+1:]; units {
	case "hertz":
		return "Hz"
	default:
		return units
	}
}

func chartTypeFromUnits(units string) program.ChartType {
	if strings.HasSuffix(units, "bytes") || strings.HasSuffix(units, "bytes/s") {
		return program.ChartTypeArea
	}
	return program.ChartTypeLine
}

func autogenLifecyclePolicy(policy AutogenPolicy) program.LifecyclePolicy {
	expire := 0
	if policy.ExpireAfterSuccessCycles > 0 {
		expire = int(policy.ExpireAfterSuccessCycles)
	}
	return program.LifecyclePolicy{
		ExpireAfterCycles: expire,
		Dimensions: program.DimensionLifecyclePolicy{
			ExpireAfterCycles: expire,
		},
	}
}
