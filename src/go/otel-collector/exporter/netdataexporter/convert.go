package netdataexporter

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

type ChartDefinition struct {
	ID         string // must be globally unique
	Title      string
	Units      string
	Family     string
	Context    string // metric name
	Type       string // line, area, stacked
	Options    string // set to "obsolete" to delete the chart, otherwise empty string
	Labels     []LabelDefinition
	Dimensions []DimensionDefinition
	IsNew      bool // Indicates if this chart is new and needs to be sent to Netdata
}

// LabelDefinition represents a Netdata chart label
type LabelDefinition struct {
	Name  string
	Value string
}

// DimensionDefinition represents a Netdata chart dimension
type DimensionDefinition struct {
	ID    string // must be unique within the chart
	Name  string // replaces ID in UI
	Algo  string // absolute (Gauge), incremental (Counter)
	Value float64
}

// Maximum length for chart IDs to prevent excessively long IDs
const maxChartIDLength = 1000

// Convert transforms OTLP metrics into Netdata charts
func (e *netdataExporter) convert(pms pmetric.Metrics) map[string]*ChartDefinition {
	// Track new and updated charts in this batch
	currentCharts := make(map[string]bool, len(e.charts))

	for _, rm := range pms.ResourceMetrics().All() {
		resAttrs := rm.Resource().Attributes()

		for _, sm := range rm.ScopeMetrics().All() {
			for _, metric := range sm.Metrics().All() {
				switch metric.Type() {
				case pmetric.MetricTypeGauge:
					e.processGauge(metric, resAttrs, currentCharts)
				case pmetric.MetricTypeSum:
					e.processSum(metric, resAttrs, currentCharts)
				case pmetric.MetricTypeHistogram:
					e.processHistogram(metric, resAttrs, currentCharts)
				case pmetric.MetricTypeExponentialHistogram:
					e.processExponentialHistogram(metric, resAttrs, currentCharts)
				case pmetric.MetricTypeSummary:
					e.processSummary(metric, resAttrs, currentCharts)
				default:
				}
			}
		}
	}

	for id, chart := range e.charts {
		if !currentCharts[id] {
			chart.IsNew = false
		}
	}

	return e.charts
}

// processGauge handles gauge metric types
func (e *netdataExporter) processGauge(metric pmetric.Metric, resourceAttrs pcommon.Map, currentCharts map[string]bool) {
	gauge := metric.Gauge()
	metricName := metric.Name()

	// Group data points by their attributes to create separate charts
	attributeGroups := groupDataPointsByAttributes(gauge.DataPoints())

	for attrHash, points := range attributeGroups {
		// Generate a unique chart ID
		chartID := generateChartID(metricName, attrHash)
		currentCharts[chartID] = true

		// Get or create chart
		chart, exists := e.charts[chartID]
		if !exists {
			chart = &ChartDefinition{
				ID:      chartID,
				Title:   getTitle(metric),
				Units:   getUnits(metric),
				Family:  getFamily(metricName, resourceAttrs),
				Context: metricName,
				Type:    "line", // Default to line chart
				Labels:  make([]LabelDefinition, 0, resourceAttrs.Len()),
				IsNew:   true,
			}
			e.charts[chartID] = chart
		}

		// Update chart with resource attributes as labels
		updateChartLabels(chart, resourceAttrs)

		// Process data points
		dimensions := make([]DimensionDefinition, 0, len(points))

		for _, dp := range points {
			if dp.Flags().NoRecordedValue() {
				continue
			}

			dimID := generateDimensionID(dp.Attributes())
			dimName := getDimensionName(dp.Attributes())

			value := 0.0
			if dp.ValueType() == pmetric.NumberDataPointValueTypeDouble {
				value = dp.DoubleValue()
			} else {
				value = float64(dp.IntValue())
			}

			dimensions = append(dimensions, DimensionDefinition{
				ID:    dimID,
				Name:  dimName,
				Algo:  "absolute", // Gauge uses absolute algorithm
				Value: value,
			})

			// Add data point attributes as labels
			addDataPointAttributesAsLabels(chart, dp.Attributes())
		}

		chart.Dimensions = append(chart.Dimensions[:0], dimensions...)
	}
}

// processSum handles sum metric types
func (e *netdataExporter) processSum(metric pmetric.Metric, resourceAttrs pcommon.Map, currentCharts map[string]bool) {
	sum := metric.Sum()
	metricName := metric.Name()

	// Determine algorithm based on aggregation temporality
	algo := "absolute" // Default for CUMULATIVE and UNSPECIFIED
	if sum.AggregationTemporality() == pmetric.AggregationTemporalityDelta {
		algo = "incremental"
	}

	// Group data points by their attributes to create separate charts
	attributeGroups := groupDataPointsByAttributes(sum.DataPoints())

	for attrHash, points := range attributeGroups {
		// Generate a unique chart ID
		chartID := generateChartID(metricName, attrHash)
		currentCharts[chartID] = true

		// Get or create chart
		chart, exists := e.charts[chartID]
		if !exists {
			chart = &ChartDefinition{
				ID:      chartID,
				Title:   getTitle(metric),
				Units:   getUnits(metric),
				Family:  getFamily(metricName, resourceAttrs),
				Context: metricName,
				Type:    "line", // Default to line chart
				Labels:  make([]LabelDefinition, 0, resourceAttrs.Len()),
				IsNew:   true,
			}
			e.charts[chartID] = chart
		}

		// Update chart with resource attributes as labels
		updateChartLabels(chart, resourceAttrs)

		// Process data points
		dimensions := make([]DimensionDefinition, 0, len(points))

		for _, dp := range points {
			if dp.Flags().NoRecordedValue() {
				continue
			}

			dimID := generateDimensionID(dp.Attributes())
			dimName := getDimensionName(dp.Attributes())

			value := 0.0
			if dp.ValueType() == pmetric.NumberDataPointValueTypeDouble {
				value = dp.DoubleValue()
			} else {
				value = float64(dp.IntValue())
			}

			dimensions = append(dimensions, DimensionDefinition{
				ID:    dimID,
				Name:  dimName,
				Algo:  algo,
				Value: value,
			})

			// Add data point attributes as labels
			addDataPointAttributesAsLabels(chart, dp.Attributes())
		}

		chart.Dimensions = dimensions
	}
}

// processHistogram handles histogram metric types
func (e *netdataExporter) processHistogram(metric pmetric.Metric, resourceAttrs pcommon.Map, currentCharts map[string]bool) {
	histogram := metric.Histogram()
	metricName := metric.Name()

	// Determine algorithm based on aggregation temporality
	algo := "absolute" // Default for CUMULATIVE and UNSPECIFIED
	if histogram.AggregationTemporality() == pmetric.AggregationTemporalityDelta {
		algo = "incremental"
	}

	for _, dp := range histogram.DataPoints().All() {
		if dp.Flags().NoRecordedValue() {
			continue
		}

		attrHash := attributesHash(dp.Attributes())
		dimID := generateDimensionID(dp.Attributes())
		dimName := getDimensionName(dp.Attributes())

		// 1. Count chart
		countChartID := generateChartID(metricName+".count", attrHash)
		currentCharts[countChartID] = true

		countChart, exists := e.charts[countChartID]
		if !exists {
			countChart = &ChartDefinition{
				ID:      countChartID,
				Title:   getTitle(metric) + " (Count)",
				Units:   "count",
				Family:  getFamily(metricName, resourceAttrs),
				Context: metricName + ".count",
				Type:    "line",
				Labels:  make([]LabelDefinition, 0, resourceAttrs.Len()),
				IsNew:   true,
			}
			e.charts[countChartID] = countChart
		}

		updateChartLabels(countChart, resourceAttrs)
		addDataPointAttributesAsLabels(countChart, dp.Attributes())

		countChart.Dimensions = []DimensionDefinition{
			{
				ID:    dimID,
				Name:  dimName,
				Algo:  algo,
				Value: float64(dp.Count()),
			},
		}

		// 2. Sum chart (if sum exists)
		if dp.HasSum() {
			sumChartID := generateChartID(metricName+".sum", attrHash)
			currentCharts[sumChartID] = true

			sumChart, exists := e.charts[sumChartID]
			if !exists {
				sumChart = &ChartDefinition{
					ID:      sumChartID,
					Title:   getTitle(metric) + " (Sum)",
					Units:   getUnits(metric),
					Family:  getFamily(metricName, resourceAttrs),
					Context: metricName + ".sum",
					Type:    "line",
					Labels:  make([]LabelDefinition, 0, resourceAttrs.Len()),
					IsNew:   true,
				}
				e.charts[sumChartID] = sumChart
			}

			updateChartLabels(sumChart, resourceAttrs)
			addDataPointAttributesAsLabels(sumChart, dp.Attributes())

			sumChart.Dimensions = []DimensionDefinition{
				{
					ID:    dimID,
					Name:  dimName,
					Algo:  algo,
					Value: dp.Sum(),
				},
			}
		}

		// 3. Buckets chart (if buckets exist)
		bucketCounts := dp.BucketCounts()
		explicitBounds := dp.ExplicitBounds()

		if bucketCounts.Len() > 0 && explicitBounds.Len() > 0 {
			bucketsChartID := generateChartID(metricName+".buckets", attrHash)
			currentCharts[bucketsChartID] = true

			bucketsChart, exists := e.charts[bucketsChartID]
			if !exists {
				bucketsChart = &ChartDefinition{
					ID:      bucketsChartID,
					Title:   getTitle(metric) + " (Buckets)",
					Units:   "count",
					Family:  getFamily(metricName, resourceAttrs),
					Context: metricName + ".buckets",
					Type:    "stacked", // Histogram buckets should be stacked
					Labels:  make([]LabelDefinition, 0, resourceAttrs.Len()),
					IsNew:   true,
				}
				e.charts[bucketsChartID] = bucketsChart
			}

			updateChartLabels(bucketsChart, resourceAttrs)
			addDataPointAttributesAsLabels(bucketsChart, dp.Attributes())

			dimensions := make([]DimensionDefinition, 0, bucketCounts.Len())

			// Add dimensions for each bucket
			for j := 0; j < bucketCounts.Len(); j++ {
				var upperBound string
				if j < explicitBounds.Len() {
					upperBound = fmt.Sprintf("%.2f", explicitBounds.At(j))
				} else {
					upperBound = "inf"
				}

				bucketDimID := dimID + "_bucket_" + strconv.Itoa(j)
				bucketDimName := "≤ " + upperBound

				dimensions = append(dimensions, DimensionDefinition{
					ID:    bucketDimID,
					Name:  bucketDimName,
					Algo:  algo,
					Value: float64(bucketCounts.At(j)),
				})
			}

			bucketsChart.Dimensions = dimensions
		}
	}
}

// processExponentialHistogram handles exponential histogram metric types
func (e *netdataExporter) processExponentialHistogram(metric pmetric.Metric, resourceAttrs pcommon.Map, currentCharts map[string]bool) {
	expHistogram := metric.ExponentialHistogram()
	metricName := metric.Name()

	// Determine algorithm based on aggregation temporality
	algo := "absolute" // Default for CUMULATIVE and UNSPECIFIED
	if expHistogram.AggregationTemporality() == pmetric.AggregationTemporalityDelta {
		algo = "incremental"
	}

	for _, dp := range expHistogram.DataPoints().All() {
		if dp.Flags().NoRecordedValue() {
			continue
		}

		attrHash := attributesHash(dp.Attributes())
		dimID := generateDimensionID(dp.Attributes())
		dimName := getDimensionName(dp.Attributes())

		// 1. Count chart
		countChartID := generateChartID(metricName+".count", attrHash)
		currentCharts[countChartID] = true

		countChart, exists := e.charts[countChartID]
		if !exists {
			countChart = &ChartDefinition{
				ID:      countChartID,
				Title:   getTitle(metric) + " (Count)",
				Units:   "count",
				Family:  getFamily(metricName, resourceAttrs),
				Context: metricName + ".count",
				Type:    "line",
				Labels:  make([]LabelDefinition, 0, resourceAttrs.Len()),
				IsNew:   true,
			}
			e.charts[countChartID] = countChart
		}

		updateChartLabels(countChart, resourceAttrs)
		addDataPointAttributesAsLabels(countChart, dp.Attributes())

		countChart.Dimensions = []DimensionDefinition{
			{
				ID:    dimID,
				Name:  dimName,
				Algo:  algo,
				Value: float64(dp.Count()),
			},
		}

		// 2. Sum chart (if sum exists)
		if dp.HasSum() {
			sumChartID := generateChartID(metricName+".sum", attrHash)
			currentCharts[sumChartID] = true

			sumChart, exists := e.charts[sumChartID]
			if !exists {
				sumChart = &ChartDefinition{
					ID:      sumChartID,
					Title:   getTitle(metric) + " (Sum)",
					Units:   getUnits(metric),
					Family:  getFamily(metricName, resourceAttrs),
					Context: metricName + ".sum",
					Type:    "line",
					Labels:  make([]LabelDefinition, 0, resourceAttrs.Len()),
					IsNew:   true,
				}
				e.charts[sumChartID] = sumChart
			}

			updateChartLabels(sumChart, resourceAttrs)
			addDataPointAttributesAsLabels(sumChart, dp.Attributes())

			sumChart.Dimensions = []DimensionDefinition{
				{
					ID:    dimID,
					Name:  dimName,
					Algo:  algo,
					Value: dp.Sum(),
				},
			}
		}

		// 3. Simplified histogram buckets chart
		// For exponential histograms, we'll create a simplified view with positive, zero, and negative counts
		bucketsChartID := generateChartID(metricName+".exp_buckets", attrHash)
		currentCharts[bucketsChartID] = true

		bucketsChart, exists := e.charts[bucketsChartID]
		if !exists {
			bucketsChart = &ChartDefinition{
				ID:      bucketsChartID,
				Title:   getTitle(metric) + " (Distribution)",
				Units:   "count",
				Family:  getFamily(metricName, resourceAttrs),
				Context: metricName + ".distribution",
				Type:    "stacked",
				Labels:  make([]LabelDefinition, 0, resourceAttrs.Len()),
				IsNew:   true,
			}
			e.charts[bucketsChartID] = bucketsChart
		}

		updateChartLabels(bucketsChart, resourceAttrs)
		addDataPointAttributesAsLabels(bucketsChart, dp.Attributes())

		// Add dimensions for positive, negative, and zero buckets
		bucketsChart.Dimensions = []DimensionDefinition{
			{
				ID:    dimID + "_positive",
				Name:  "Positive Values",
				Algo:  algo,
				Value: getTotalCountFromBuckets(dp.Positive()),
			},
			{
				ID:    dimID + "_zero",
				Name:  "Zero Values",
				Algo:  algo,
				Value: float64(dp.ZeroCount()),
			},
			{
				ID:    dimID + "_negative",
				Name:  "Negative Values",
				Algo:  algo,
				Value: getTotalCountFromBuckets(dp.Negative()),
			},
		}
	}
}

// processSummary handles summary metric types
func (e *netdataExporter) processSummary(metric pmetric.Metric, resourceAttrs pcommon.Map, currentCharts map[string]bool) {
	summary := metric.Summary()
	metricName := metric.Name()

	for _, dp := range summary.DataPoints().All() {
		if dp.Flags().NoRecordedValue() {
			continue
		}

		attrHash := attributesHash(dp.Attributes())
		dimID := generateDimensionID(dp.Attributes())
		dimName := getDimensionName(dp.Attributes())

		// 1. Count chart
		countChartID := generateChartID(metricName+".count", attrHash)
		currentCharts[countChartID] = true

		countChart, exists := e.charts[countChartID]
		if !exists {
			countChart = &ChartDefinition{
				ID:      countChartID,
				Title:   getTitle(metric) + " (Count)",
				Units:   "count",
				Family:  getFamily(metricName, resourceAttrs),
				Context: metricName + ".count",
				Type:    "line",
				Labels:  make([]LabelDefinition, 0, resourceAttrs.Len()),
				IsNew:   true,
			}
			e.charts[countChartID] = countChart
		}

		updateChartLabels(countChart, resourceAttrs)
		addDataPointAttributesAsLabels(countChart, dp.Attributes())

		countChart.Dimensions = []DimensionDefinition{
			{
				ID:    dimID,
				Name:  dimName,
				Algo:  "absolute", // Summary count should be absolute
				Value: float64(dp.Count()),
			},
		}

		// 2. Sum chart
		sumChartID := generateChartID(metricName+".sum", attrHash)
		currentCharts[sumChartID] = true

		sumChart, exists := e.charts[sumChartID]
		if !exists {
			sumChart = &ChartDefinition{
				ID:      sumChartID,
				Title:   getTitle(metric) + " (Sum)",
				Units:   getUnits(metric),
				Family:  getFamily(metricName, resourceAttrs),
				Context: metricName + ".sum",
				Type:    "line",
				Labels:  make([]LabelDefinition, 0, resourceAttrs.Len()),
				IsNew:   true,
			}
			e.charts[sumChartID] = sumChart
		}

		updateChartLabels(sumChart, resourceAttrs)
		addDataPointAttributesAsLabels(sumChart, dp.Attributes())

		sumChart.Dimensions = []DimensionDefinition{
			{
				ID:    dimID,
				Name:  dimName,
				Algo:  "absolute", // Summary sum should be absolute
				Value: dp.Sum(),
			},
		}

		// 3. Quantiles chart
		qvs := dp.QuantileValues()
		if qvs.Len() > 0 {
			quantilesChartID := generateChartID(metricName+".quantiles", attrHash)
			currentCharts[quantilesChartID] = true

			quantilesChart, exists := e.charts[quantilesChartID]
			if !exists {
				quantilesChart = &ChartDefinition{
					ID:      quantilesChartID,
					Title:   getTitle(metric) + " (Quantiles)",
					Units:   getUnits(metric),
					Family:  getFamily(metricName, resourceAttrs),
					Context: metricName + ".quantiles",
					Type:    "line",
					Labels:  make([]LabelDefinition, 0, resourceAttrs.Len()),
					IsNew:   true,
				}
				e.charts[quantilesChartID] = quantilesChart
			}

			updateChartLabels(quantilesChart, resourceAttrs)
			addDataPointAttributesAsLabels(quantilesChart, dp.Attributes())

			dimensions := make([]DimensionDefinition, 0, qvs.Len())

			// Add dimensions for each quantile
			for _, qv := range qvs.All() {
				quantileStr := fmt.Sprintf("%.2f", qv.Quantile())

				// Special names for min/max
				var quantileName string
				if qv.Quantile() == 0 {
					quantileName = "min"
				} else if qv.Quantile() == 1 {
					quantileName = "max"
				} else {
					quantileName = "p" + strings.ReplaceAll(quantileStr, "0.", "")
				}

				dimensions = append(dimensions, DimensionDefinition{
					ID:    dimID + "_q" + strings.ReplaceAll(quantileStr, ".", "_"),
					Name:  quantileName,
					Algo:  "absolute", // Quantiles are absolute values
					Value: qv.Value(),
				})
			}

			quantilesChart.Dimensions = dimensions
		}
	}
}

// groupDataPointsByAttributes groups data points by attributes to create separate charts
func groupDataPointsByAttributes(dataPoints pmetric.NumberDataPointSlice) map[string][]pmetric.NumberDataPoint {
	groups := make(map[string][]pmetric.NumberDataPoint)

	for _, dp := range dataPoints.All() {
		hash := attributesHash(dp.Attributes())
		groups[hash] = append(groups[hash], dp)
	}

	return groups
}

func generateChartID(metricName, attrHash string) string {
	// Sanitize and limit the length of the metric name to prevent overly long IDs
	sanitizedName := sanitizeID(metricName)
	if len(sanitizedName) > maxChartIDLength-1-len(attrHash) {
		sanitizedName = sanitizedName[:maxChartIDLength-1-len(attrHash)]
	}
	return sanitizedName + "_" + attrHash
}

func generateDimensionID(attrs pcommon.Map) string {
	if attrs.Len() == 0 {
		return "value"
	}

	return "value_" + attributesHash(attrs)
}

func getDimensionName(attrs pcommon.Map) string {
	if attrs.Len() == 0 {
		return "value"
	}

	// Try to find a good name from the attributes
	nameAttrs := []string{"name", "id", "key"}
	for _, nameAttr := range nameAttrs {
		if val, ok := attrs.Get(nameAttr); ok {
			return val.AsString()
		}
	}

	return "value"
}

func getTitle(metric pmetric.Metric) string {
	if metric.Description() != "" {
		return metric.Description()
	}
	return metric.Name()
}

func getUnits(metric pmetric.Metric) string {
	if metric.Unit() != "" {
		return metric.Unit()
	}
	return "count" // Default unit
}

func getFamily(metricName string, resourceAttrs pcommon.Map) string {
	var sb strings.Builder

	if val, ok := resourceAttrs.Get("service.name"); ok {
		sb.WriteString(sanitizeID(val.AsString()))
	} else if val, ok := resourceAttrs.Get("service.namespace"); ok {
		sb.WriteString(sanitizeID(val.AsString()))
	}

	if sb.Len() > 0 {
		sb.WriteString("/")
	}

	// Replace dots with slashes for hierarchical grouping
	sb.WriteString(strings.ReplaceAll(metricName, ".", "/"))

	return sb.String()
}

func updateChartLabels(chart *ChartDefinition, resourceAttrs pcommon.Map) {
	if len(chart.Labels) == resourceAttrs.Len() {
		same := true
		resourceAttrs.Range(func(key string, value pcommon.Value) bool {
			found := false
			for _, label := range chart.Labels {
				if label.Name == key && label.Value == value.AsString() {
					found = true
					break
				}
			}
			if !found {
				same = false
				return false
			}
			return true
		})
		if same {
			return
		}
	}

	chart.Labels = make([]LabelDefinition, 0, resourceAttrs.Len())

	resourceAttrs.Range(func(key string, value pcommon.Value) bool {
		chart.Labels = append(chart.Labels, LabelDefinition{
			Name:  key,
			Value: value.AsString(),
		})
		return true
	})

	sort.Slice(chart.Labels, func(i, j int) bool {
		return chart.Labels[i].Name < chart.Labels[j].Name
	})
}

func addDataPointAttributesAsLabels(chart *ChartDefinition, attrs pcommon.Map) {
	// Use a map to track existing labels by name for O(1) lookup
	existingLabels := make(map[string]bool, len(chart.Labels))
	for _, label := range chart.Labels {
		existingLabels[label.Name] = true
	}

	var newLabels []LabelDefinition
	attrs.Range(func(key string, value pcommon.Value) bool {
		if !existingLabels[key] {
			newLabels = append(newLabels, LabelDefinition{
				Name:  key,
				Value: value.AsString(),
			})
			existingLabels[key] = true
		}
		return true
	})

	if len(newLabels) > 0 {
		chart.Labels = append(chart.Labels, newLabels...)

		// Sort labels by name for consistent output
		sort.Slice(chart.Labels, func(i, j int) bool {
			return chart.Labels[i].Name < chart.Labels[j].Name
		})
	}
}

func attributesHash(attrs pcommon.Map) string {
	if attrs.Len() == 0 {
		return "default"
	}

	hash := fnv.New64a()

	// Sort keys for consistent hashing
	keys := make([]string, 0, attrs.Len())
	attrs.Range(func(k string, v pcommon.Value) bool {
		keys = append(keys, k)
		return true
	})
	sort.Strings(keys)

	for _, k := range keys {
		v, _ := attrs.Get(k)
		_, _ = hash.Write([]byte(k))
		_, _ = hash.Write([]byte(":"))
		_, _ = hash.Write([]byte(v.AsString()))
		_, _ = hash.Write([]byte(";"))
	}

	return strconv.FormatUint(hash.Sum64(), 16)
}

func sanitizeID(name string) string {
	if name == "" {
		return "unknown"
	}

	var sb strings.Builder
	sb.Grow(len(name))

	// Ensure the ID doesn't start with a number
	if unicode.IsDigit(rune(name[0])) {
		sb.WriteString("n_")
	}

	for _, r := range name {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '_', r == '.', r == '-':
			sb.WriteRune(r)
		default:
			sb.WriteRune('_')
		}
	}

	return sb.String()
}

func getTotalCountFromBuckets(buckets pmetric.ExponentialHistogramDataPointBuckets) float64 {
	counts := buckets.BucketCounts()
	var sum uint64
	for i := 0; i < counts.Len(); i++ {
		sum += counts.At(i)
	}
	return float64(sum)
}
