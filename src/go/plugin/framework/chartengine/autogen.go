// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
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
	contextName       string
	staticDimension   bool
}

type autogenSourceBuilder func(
	metricName string,
	labels metrix.LabelView,
	meta metrix.SeriesMeta,
	policy AutogenPolicy,
) (autogenRoute, bool, error)

type autogenRoleBuilder func(
	metricName string,
	labels metrix.LabelView,
	policy AutogenPolicy,
) (autogenRoute, bool, error)

var autogenSourceBuilders = map[metrix.MetricKind]autogenSourceBuilder{
	metrix.MetricKindHistogram: buildHistogramAutogenRoute,
	metrix.MetricKindSummary:   buildSummaryAutogenRoute,
	metrix.MetricKindStateSet:  buildStateSetAutogenRoute,
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

	route, ok, err := buildAutogenRoute(metricName, labels, meta, policy)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	if metricMeta, ok := autogenMetricMeta(reader, metricName, meta); ok {
		route = applyAutogenMetricMeta(route, metricMeta, meta)
	}
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
			Static:            route.staticDimension,
			Inferred:          false,
			Autogen:           true,
			Meta: program.ChartMeta{
				Title:     title,
				Family:    route.family,
				Context:   getAutogenChartContext(route.contextName),
				Units:     route.units,
				Algorithm: route.algorithm,
				Type:      route.chartType,
				Priority:  0,
			},
			Lifecycle: autogenLifecyclePolicy(policy),
		},
	}, true, nil
}

func buildAutogenRoute(
	metricName string,
	labels metrix.LabelView,
	meta metrix.SeriesMeta,
	policy AutogenPolicy,
) (autogenRoute, bool, error) {
	if strings.TrimSpace(metricName) == "" {
		return autogenRoute{}, false, nil
	}
	builder := buildScalarAutogenRoute
	if knownBuilder, ok := autogenSourceBuilders[meta.SourceKind]; ok {
		builder = knownBuilder
	}
	return builder(metricName, labels, meta, policy)
}

func autogenMetricMeta(reader metrix.Reader, metricName string, meta metrix.SeriesMeta) (metrix.MetricMeta, bool) {
	if reader == nil {
		return metrix.MetricMeta{}, false
	}
	for _, name := range sourceMetricNames(metricName, meta) {
		if name == "" {
			continue
		}
		if metricMeta, ok := reader.MetricMeta(name); ok {
			return metricMeta, true
		}
	}
	return metrix.MetricMeta{}, false
}

func sourceMetricNames(metricName string, meta metrix.SeriesMeta) []string {
	source := sourceMetricName(metricName, meta)
	if source == "" || source == metricName {
		return []string{metricName}
	}
	return []string{source, metricName}
}

func sourceMetricName(metricName string, meta metrix.SeriesMeta) string {
	switch meta.SourceKind {
	case metrix.MetricKindHistogram:
		switch meta.FlattenRole {
		case metrix.FlattenRoleHistogramBucket:
			return strings.TrimSuffix(metricName, "_bucket")
		case metrix.FlattenRoleHistogramCount:
			return strings.TrimSuffix(metricName, "_count")
		case metrix.FlattenRoleHistogramSum:
			return strings.TrimSuffix(metricName, "_sum")
		}
	case metrix.MetricKindSummary:
		switch meta.FlattenRole {
		case metrix.FlattenRoleSummaryCount:
			return strings.TrimSuffix(metricName, "_count")
		case metrix.FlattenRoleSummarySum:
			return strings.TrimSuffix(metricName, "_sum")
		}
	}
	return metricName
}

func applyAutogenMetricMeta(route autogenRoute, meta metrix.MetricMeta, seriesMeta metrix.SeriesMeta) autogenRoute {
	if title := strings.TrimSpace(meta.Description); title != "" {
		route.title = title
	}
	if family := strings.TrimSpace(meta.ChartFamily); family != "" {
		route.family = family
	}
	if unit := strings.TrimSpace(meta.Unit); unit != "" && allowAutogenUnitOverride(seriesMeta) {
		route.units = normalizeAutogenUnitByAlgorithm(unit, route.algorithm)
		route.chartType = chartTypeFromUnits(route.units)
	}
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
	metricName string,
	labels metrix.LabelView,
	meta metrix.SeriesMeta,
	policy AutogenPolicy,
) (autogenRoute, bool, error) {
	builder, ok := histogramRoleBuilders[meta.FlattenRole]
	if !ok {
		return autogenRoute{}, false, nil
	}
	return builder(metricName, labels, policy)
}

func buildSummaryAutogenRoute(
	metricName string,
	labels metrix.LabelView,
	meta metrix.SeriesMeta,
	policy AutogenPolicy,
) (autogenRoute, bool, error) {
	builder, ok := summaryRoleBuilders[meta.FlattenRole]
	if !ok {
		return autogenRoute{}, false, nil
	}
	return builder(metricName, labels, policy)
}

func buildHistogramBucketAutogenRoute(
	metricName string,
	labels metrix.LabelView,
	policy AutogenPolicy,
) (autogenRoute, bool, error) {
	baseName := strings.TrimSuffix(metricName, "_bucket")
	if baseName == "" {
		baseName = metricName
	}
	upperBound, ok := labels.Get(histogramBucketLabel)
	if !ok || strings.TrimSpace(upperBound) == "" {
		return autogenRoute{}, false, nil
	}
	chartID := buildJoinedLabelAutogenID(baseName, labels, map[string]struct{}{
		histogramBucketLabel: {},
	})
	if !fitsTypeIDBudget(policy, chartID) {
		return autogenRoute{}, false, nil
	}
	return autogenRoute{
		chartID:           chartID,
		chartName:         baseName,
		dimensionName:     "bucket_" + upperBound,
		dimensionKeyLabel: histogramBucketLabel,
		algorithm:         program.AlgorithmIncremental,
		units:             "observations/s",
		chartType:         program.ChartTypeLine,
		family:            getAutogenChartFamily(baseName),
		contextName:       baseName,
		staticDimension:   false,
	}, true, nil
}

func buildHistogramCountAutogenRoute(
	metricName string,
	labels metrix.LabelView,
	policy AutogenPolicy,
) (autogenRoute, bool, error) {
	baseName := strings.TrimSuffix(metricName, "_count")
	if baseName == "" {
		baseName = metricName
	}
	return buildCounterComponentAutogenRoute(baseName, "_count", labels, policy, "events/s")
}

func buildHistogramSumAutogenRoute(
	metricName string,
	labels metrix.LabelView,
	policy AutogenPolicy,
) (autogenRoute, bool, error) {
	baseName := strings.TrimSuffix(metricName, "_sum")
	if baseName == "" {
		baseName = metricName
	}
	return buildCounterComponentAutogenRoute(baseName, "_sum", labels, policy, getAutogenCounterUnits(baseName))
}

func buildSummaryQuantileAutogenRoute(
	metricName string,
	labels metrix.LabelView,
	policy AutogenPolicy,
) (autogenRoute, bool, error) {
	quantile, ok := labels.Get(summaryQuantileLabel)
	if !ok || strings.TrimSpace(quantile) == "" {
		return autogenRoute{}, false, nil
	}
	chartID := buildJoinedLabelAutogenID(metricName, labels, map[string]struct{}{
		summaryQuantileLabel: {},
	})
	if !fitsTypeIDBudget(policy, chartID) {
		return autogenRoute{}, false, nil
	}
	units := getAutogenSummaryUnits(metricName)
	return autogenRoute{
		chartID:           chartID,
		chartName:         metricName,
		dimensionName:     "quantile_" + quantile,
		dimensionKeyLabel: summaryQuantileLabel,
		algorithm:         program.AlgorithmAbsolute,
		units:             units,
		chartType:         chartTypeFromUnits(units),
		family:            getAutogenChartFamily(metricName),
		contextName:       metricName,
		staticDimension:   false,
	}, true, nil
}

func buildSummaryCountAutogenRoute(
	metricName string,
	labels metrix.LabelView,
	policy AutogenPolicy,
) (autogenRoute, bool, error) {
	baseName := strings.TrimSuffix(metricName, "_count")
	if baseName == "" {
		baseName = metricName
	}
	return buildCounterComponentAutogenRoute(baseName, "_count", labels, policy, "events/s")
}

func buildSummarySumAutogenRoute(
	metricName string,
	labels metrix.LabelView,
	policy AutogenPolicy,
) (autogenRoute, bool, error) {
	baseName := strings.TrimSuffix(metricName, "_sum")
	if baseName == "" {
		baseName = metricName
	}
	return buildCounterComponentAutogenRoute(baseName, "_sum", labels, policy, getAutogenCounterUnits(baseName))
}

func buildCounterComponentAutogenRoute(
	baseName string,
	suffix string,
	labels metrix.LabelView,
	policy AutogenPolicy,
	units string,
) (autogenRoute, bool, error) {
	chartName := baseName + suffix
	chartID := buildJoinedLabelAutogenID(chartName, labels, nil)
	if !fitsTypeIDBudget(policy, chartID) {
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
	metricName string,
	labels metrix.LabelView,
	meta metrix.SeriesMeta,
	policy AutogenPolicy,
) (autogenRoute, bool, error) {
	if meta.FlattenRole != metrix.FlattenRoleStateSetState {
		return autogenRoute{}, false, nil
	}
	state, ok := labels.Get(metricName)
	if !ok || strings.TrimSpace(state) == "" {
		return autogenRoute{}, false, nil
	}
	chartID := buildJoinedLabelAutogenID(metricName, labels, map[string]struct{}{
		metricName: {},
	})
	if !fitsTypeIDBudget(policy, chartID) {
		return autogenRoute{}, false, nil
	}
	return autogenRoute{
		chartID:           chartID,
		chartName:         metricName,
		dimensionName:     state,
		dimensionKeyLabel: metricName,
		algorithm:         program.AlgorithmAbsolute,
		units:             "state",
		chartType:         program.ChartTypeLine,
		family:            getAutogenChartFamily(metricName),
		contextName:       metricName,
		staticDimension:   false,
	}, true, nil
}

func buildScalarAutogenRoute(
	metricName string,
	labels metrix.LabelView,
	meta metrix.SeriesMeta,
	policy AutogenPolicy,
) (autogenRoute, bool, error) {
	chartID := buildJoinedLabelAutogenID(metricName, labels, nil)
	if !fitsTypeIDBudget(policy, chartID) {
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

func fitsTypeIDBudget(policy AutogenPolicy, chartID string) bool {
	maxLen := policy.MaxTypeIDLen
	if maxLen <= 0 {
		maxLen = defaultMaxTypeIDLen
	}
	if strings.TrimSpace(policy.TypeID) == "" {
		return len(chartID) <= maxLen
	}
	return len(policy.TypeID)+1+len(chartID) <= maxLen
}

func buildJoinedLabelAutogenID(metricName string, labels metrix.LabelView, exclude map[string]struct{}) string {
	var b strings.Builder
	b.Grow(len(metricName) + labels.Len()*8)
	b.WriteString(metricName)
	labels.Range(func(key, value string) bool {
		if key == "" || value == "" {
			return true
		}
		if _, skip := exclude[key]; skip {
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
	if strings.IndexByte(value, ' ') != -1 {
		value = strings.ReplaceAll(value, " ", "_")
	}
	if strings.IndexByte(value, '\\') != -1 {
		if v := decodeAutogenLabelValue(value); strings.IndexByte(v, '\\') != -1 {
			value = strings.ReplaceAll(v, `\`, "_")
		} else {
			value = v
		}
	}
	if strings.IndexByte(value, '\'') != -1 {
		value = strings.ReplaceAll(value, "'", "")
	}
	return value
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

func getAutogenChartContext(metricName string) string {
	metricName = strings.TrimSpace(metricName)
	if metricName == "" {
		return "metric"
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
	return getAutogenCounterUnits(metric)
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
