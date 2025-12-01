package netdataexporter

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func TestNetdataExporter_Convert(t *testing.T) {
	tests := map[string]struct {
		inputMetrics       func() pmetric.Metrics
		expectedChartCount int
		expectedCharts     map[string]ChartDefinition
	}{
		"gauge_metric": {
			inputMetrics: func() pmetric.Metrics {
				metrics := pmetric.NewMetrics()
				rm := metrics.ResourceMetrics().AppendEmpty()

				// Add resource attributes
				rm.Resource().Attributes().PutStr("service.name", "test-service")

				sm := rm.ScopeMetrics().AppendEmpty()
				metric := sm.Metrics().AppendEmpty()
				metric.SetName("test.gauge")
				metric.SetDescription("Test Gauge")
				metric.SetUnit("bytes")

				gauge := metric.SetEmptyGauge()
				dp := gauge.DataPoints().AppendEmpty()
				dp.SetDoubleValue(42.0)
				dp.Attributes().PutStr("host", "test-host")

				return metrics
			},
			expectedChartCount: 1,
			expectedCharts:     map[string]ChartDefinition{},
		},
		"sum_metric_delta": {
			inputMetrics: func() pmetric.Metrics {
				metrics := pmetric.NewMetrics()
				rm := metrics.ResourceMetrics().AppendEmpty()

				sm := rm.ScopeMetrics().AppendEmpty()
				metric := sm.Metrics().AppendEmpty()
				metric.SetName("test.sum")
				metric.SetDescription("Test Sum")
				metric.SetUnit("count")

				sum := metric.SetEmptySum()
				sum.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
				sum.SetIsMonotonic(true)

				dp := sum.DataPoints().AppendEmpty()
				dp.SetIntValue(100)

				return metrics
			},
			expectedChartCount: 1,
			expectedCharts:     map[string]ChartDefinition{},
		},
		"sum_metric_cumulative": {
			inputMetrics: func() pmetric.Metrics {
				metrics := pmetric.NewMetrics()
				rm := metrics.ResourceMetrics().AppendEmpty()

				sm := rm.ScopeMetrics().AppendEmpty()
				metric := sm.Metrics().AppendEmpty()
				metric.SetName("test.sum")
				metric.SetDescription("Test Sum")
				metric.SetUnit("count")

				sum := metric.SetEmptySum()
				sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
				sum.SetIsMonotonic(true)

				dp := sum.DataPoints().AppendEmpty()
				dp.SetIntValue(100)

				return metrics
			},
			expectedChartCount: 1,
			expectedCharts:     map[string]ChartDefinition{},
		},
		"histogram_metric": {
			inputMetrics: func() pmetric.Metrics {
				metrics := pmetric.NewMetrics()
				rm := metrics.ResourceMetrics().AppendEmpty()

				sm := rm.ScopeMetrics().AppendEmpty()
				metric := sm.Metrics().AppendEmpty()
				metric.SetName("test.histogram")
				metric.SetDescription("Test Histogram")
				metric.SetUnit("ms")

				histogram := metric.SetEmptyHistogram()
				histogram.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)

				dp := histogram.DataPoints().AppendEmpty()
				dp.SetCount(30)
				dp.SetSum(200.0)
				dp.BucketCounts().FromRaw([]uint64{10, 15, 5})
				dp.ExplicitBounds().FromRaw([]float64{10.0, 20.0})

				return metrics
			},
			expectedChartCount: 3, // Count, Sum, and Buckets charts
			expectedCharts:     map[string]ChartDefinition{},
		},
		"exponential_histogram_metric": {
			inputMetrics: func() pmetric.Metrics {
				metrics := pmetric.NewMetrics()
				rm := metrics.ResourceMetrics().AppendEmpty()

				sm := rm.ScopeMetrics().AppendEmpty()
				metric := sm.Metrics().AppendEmpty()
				metric.SetName("test.exponential_histogram")
				metric.SetDescription("Test Exponential Histogram")
				metric.SetUnit("ms")

				expHistogram := metric.SetEmptyExponentialHistogram()
				expHistogram.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)

				dp := expHistogram.DataPoints().AppendEmpty()
				dp.SetCount(30)
				dp.SetSum(200.0)
				dp.SetZeroCount(5)

				// Set up positive buckets
				dp.Positive().SetOffset(0)
				dp.Positive().BucketCounts().FromRaw([]uint64{10, 5})

				// Set up negative buckets
				dp.Negative().SetOffset(0)
				dp.Negative().BucketCounts().FromRaw([]uint64{7, 3})

				return metrics
			},
			expectedChartCount: 3, // Count, Sum, and Distribution charts
			expectedCharts:     map[string]ChartDefinition{},
		},
		"summary_metric": {
			inputMetrics: func() pmetric.Metrics {
				metrics := pmetric.NewMetrics()
				rm := metrics.ResourceMetrics().AppendEmpty()

				sm := rm.ScopeMetrics().AppendEmpty()
				metric := sm.Metrics().AppendEmpty()
				metric.SetName("test.summary")
				metric.SetDescription("Test Summary")
				metric.SetUnit("ms")

				summary := metric.SetEmptySummary()

				dp := summary.DataPoints().AppendEmpty()
				dp.SetCount(100)
				dp.SetSum(5000.0)

				// Add quantiles (0%, 50%, 95%, 99%, 100%)
				qv := dp.QuantileValues().AppendEmpty()
				qv.SetQuantile(0.0)
				qv.SetValue(1.0)

				qv = dp.QuantileValues().AppendEmpty()
				qv.SetQuantile(0.5)
				qv.SetValue(5.0)

				qv = dp.QuantileValues().AppendEmpty()
				qv.SetQuantile(0.95)
				qv.SetValue(9.5)

				qv = dp.QuantileValues().AppendEmpty()
				qv.SetQuantile(0.99)
				qv.SetValue(10.0)

				qv = dp.QuantileValues().AppendEmpty()
				qv.SetQuantile(1.0)
				qv.SetValue(100.0)

				return metrics
			},
			expectedChartCount: 3, // Count, Sum, and Quantiles charts
			expectedCharts:     map[string]ChartDefinition{},
		},
		"multiple_data_points_by_attribute": {
			inputMetrics: func() pmetric.Metrics {
				metrics := pmetric.NewMetrics()
				rm := metrics.ResourceMetrics().AppendEmpty()

				sm := rm.ScopeMetrics().AppendEmpty()
				metric := sm.Metrics().AppendEmpty()
				metric.SetName("test.gauge.multi")
				metric.SetDescription("Test Gauge with Multiple Points")
				metric.SetUnit("bytes")

				gauge := metric.SetEmptyGauge()

				// First data point with host=server1
				dp1 := gauge.DataPoints().AppendEmpty()
				dp1.SetDoubleValue(100.0)
				dp1.Attributes().PutStr("host", "server1")

				// Second data point with host=server2
				dp2 := gauge.DataPoints().AppendEmpty()
				dp2.SetDoubleValue(200.0)
				dp2.Attributes().PutStr("host", "server2")

				return metrics
			},
			expectedChartCount: 2, // One chart per unique attribute set
			expectedCharts:     map[string]ChartDefinition{},
		},
		"staleness_marker": {
			inputMetrics: func() pmetric.Metrics {
				metrics := pmetric.NewMetrics()
				rm := metrics.ResourceMetrics().AppendEmpty()

				sm := rm.ScopeMetrics().AppendEmpty()
				metric := sm.Metrics().AppendEmpty()
				metric.SetName("test.gauge.stale")
				metric.SetDescription("Test Gauge with Staleness Marker")

				gauge := metric.SetEmptyGauge()

				// Normal data point
				dp1 := gauge.DataPoints().AppendEmpty()
				dp1.SetDoubleValue(100.0)

				// Stale data point (with staleness flag set)
				dp2 := gauge.DataPoints().AppendEmpty()
				dp2.SetDoubleValue(0.0)
				dp2.SetFlags(1) // Set staleness marker flag

				return metrics
			},
			expectedChartCount: 1,
			expectedCharts:     map[string]ChartDefinition{},
		},
		"resource_and_datapoint_attributes_as_labels": {
			inputMetrics: func() pmetric.Metrics {
				metrics := pmetric.NewMetrics()
				rm := metrics.ResourceMetrics().AppendEmpty()

				// Add resource attributes
				rm.Resource().Attributes().PutStr("service.name", "test-service")
				rm.Resource().Attributes().PutStr("deployment.environment", "production")

				sm := rm.ScopeMetrics().AppendEmpty()
				metric := sm.Metrics().AppendEmpty()
				metric.SetName("test.labels")

				gauge := metric.SetEmptyGauge()
				dp := gauge.DataPoints().AppendEmpty()
				dp.SetDoubleValue(42.0)

				// Add data point attributes
				dp.Attributes().PutStr("host", "host1")
				dp.Attributes().PutStr("region", "us-west")

				return metrics
			},
			expectedChartCount: 1,
			expectedCharts:     map[string]ChartDefinition{},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// Create a new exporter for each test to avoid state between tests
			exporter := &netdataExporter{
				charts: make(map[string]*ChartDefinition),
			}

			// Convert the metrics
			inputMetrics := tt.inputMetrics()
			charts := exporter.Convert(inputMetrics)

			// Verify the chart count
			assert.Equal(t, tt.expectedChartCount, len(charts), "Wrong number of charts created")

			switch name {
			case "gauge_metric":
				// Find the chart for the gauge metric
				var gaugeChart *ChartDefinition
				for _, chart := range charts {
					if chart.Context == "test.gauge" {
						gaugeChart = chart
						break
					}
				}
				require.NotNil(t, gaugeChart, "Gauge chart should exist")
				assert.Equal(t, "Test Gauge", gaugeChart.Title)
				assert.Equal(t, "bytes", gaugeChart.Units)
				assert.Equal(t, "test-service_test", gaugeChart.Family)
				assert.Equal(t, "line", gaugeChart.Type)
				assert.Len(t, gaugeChart.Dimensions, 1)
				assert.Equal(t, 42.0, gaugeChart.Dimensions[0].Value)
				assert.Equal(t, "absolute", gaugeChart.Dimensions[0].Algo)

			case "sum_metric_delta":
				// Find the chart for the delta sum metric
				var sumChart *ChartDefinition
				for _, chart := range charts {
					if chart.Context == "test.sum" {
						sumChart = chart
						break
					}
				}
				require.NotNil(t, sumChart, "Sum chart should exist")
				assert.Equal(t, "Test Sum", sumChart.Title)
				assert.Equal(t, "count", sumChart.Units)
				assert.Equal(t, "test", sumChart.Family)
				assert.Equal(t, "line", sumChart.Type)
				assert.Len(t, sumChart.Dimensions, 1)
				assert.Equal(t, 100.0, sumChart.Dimensions[0].Value)
				assert.Equal(t, "incremental", sumChart.Dimensions[0].Algo, "Delta temporality should use incremental algorithm")

			case "sum_metric_cumulative":
				// Find the chart for the cumulative sum metric
				var sumChart *ChartDefinition
				for _, chart := range charts {
					if chart.Context == "test.sum" {
						sumChart = chart
						break
					}
				}
				require.NotNil(t, sumChart, "Sum chart should exist")
				assert.Equal(t, "Test Sum", sumChart.Title)
				assert.Equal(t, "count", sumChart.Units)
				assert.Equal(t, "test", sumChart.Family)
				assert.Equal(t, "line", sumChart.Type)
				assert.Len(t, sumChart.Dimensions, 1)
				assert.Equal(t, 100.0, sumChart.Dimensions[0].Value)
				assert.Equal(t, "absolute", sumChart.Dimensions[0].Algo, "Cumulative temporality should use absolute algorithm")

			case "histogram_metric":
				// Check count chart
				var countChart *ChartDefinition
				for _, chart := range charts {
					if chart.Context == "test.histogram.count" {
						countChart = chart
						break
					}
				}
				require.NotNil(t, countChart, "Histogram count chart should exist")
				assert.Equal(t, "Test Histogram (Count)", countChart.Title)
				assert.Equal(t, "count", countChart.Units)
				assert.Equal(t, "line", countChart.Type)
				assert.Len(t, countChart.Dimensions, 1)
				assert.Equal(t, 30.0, countChart.Dimensions[0].Value)
				assert.Equal(t, "absolute", countChart.Dimensions[0].Algo)

				// Check sum chart
				var sumChart *ChartDefinition
				for _, chart := range charts {
					if chart.Context == "test.histogram.sum" {
						sumChart = chart
						break
					}
				}
				require.NotNil(t, sumChart, "Histogram sum chart should exist")
				assert.Equal(t, "Test Histogram (Sum)", sumChart.Title)
				assert.Equal(t, "ms", sumChart.Units)
				assert.Equal(t, "line", sumChart.Type)
				assert.Len(t, sumChart.Dimensions, 1)
				assert.Equal(t, 200.0, sumChart.Dimensions[0].Value)

				// Check buckets chart
				var bucketsChart *ChartDefinition
				for _, chart := range charts {
					if chart.Context == "test.histogram.buckets" {
						bucketsChart = chart
						break
					}
				}
				require.NotNil(t, bucketsChart, "Histogram buckets chart should exist")
				assert.Equal(t, "Test Histogram (Buckets)", bucketsChart.Title)
				assert.Equal(t, "count", bucketsChart.Units)
				assert.Equal(t, "stacked", bucketsChart.Type)
				assert.Len(t, bucketsChart.Dimensions, 3)

				// Verify bucket values (we don't know exact order, so check all values are present)
				bucketValues := []float64{10.0, 15.0, 5.0}
				for _, dim := range bucketsChart.Dimensions {
					assert.Contains(t, bucketValues, dim.Value)
				}

			case "exponential_histogram_metric":
				// Check count chart
				var countChart *ChartDefinition
				for _, chart := range charts {
					if chart.Context == "test.exponential_histogram.count" {
						countChart = chart
						break
					}
				}
				require.NotNil(t, countChart, "Exponential histogram count chart should exist")
				assert.Equal(t, "Test Exponential Histogram (Count)", countChart.Title)
				assert.Equal(t, "count", countChart.Units)
				assert.Equal(t, "line", countChart.Type)
				assert.Len(t, countChart.Dimensions, 1)
				assert.Equal(t, 30.0, countChart.Dimensions[0].Value)
				assert.Equal(t, "incremental", countChart.Dimensions[0].Algo, "Delta temporality should use incremental algorithm")

				// Check sum chart
				var sumChart *ChartDefinition
				for _, chart := range charts {
					if chart.Context == "test.exponential_histogram.sum" {
						sumChart = chart
						break
					}
				}
				require.NotNil(t, sumChart, "Exponential histogram sum chart should exist")
				assert.Equal(t, "Test Exponential Histogram (Sum)", sumChart.Title)
				assert.Equal(t, "ms", sumChart.Units)
				assert.Equal(t, "line", sumChart.Type)
				assert.Len(t, sumChart.Dimensions, 1)
				assert.Equal(t, 200.0, sumChart.Dimensions[0].Value)

				// Check distribution chart
				var distChart *ChartDefinition
				for _, chart := range charts {
					if chart.Context == "test.exponential_histogram.distribution" {
						distChart = chart
						break
					}
				}
				require.NotNil(t, distChart, "Exponential histogram distribution chart should exist")
				assert.Equal(t, "Test Exponential Histogram (Distribution)", distChart.Title)
				assert.Equal(t, "count", distChart.Units)
				assert.Equal(t, "stacked", distChart.Type)
				assert.Len(t, distChart.Dimensions, 3)

				// Find the dimensions for positive, zero, and negative values
				positiveValue := 0.0
				zeroValue := 0.0
				negativeValue := 0.0

				for _, dim := range distChart.Dimensions {
					if dim.Name == "Positive Values" {
						positiveValue = dim.Value
					} else if dim.Name == "Zero Values" {
						zeroValue = dim.Value
					} else if dim.Name == "Negative Values" {
						negativeValue = dim.Value
					}
				}

				assert.Equal(t, 15.0, positiveValue, "Positive bucket count should sum to 15")
				assert.Equal(t, 5.0, zeroValue, "Zero bucket count should be 5")
				assert.Equal(t, 10.0, negativeValue, "Negative bucket count should sum to 10")

			case "summary_metric":
				// Check count chart
				var countChart *ChartDefinition
				for _, chart := range charts {
					if chart.Context == "test.summary.count" {
						countChart = chart
						break
					}
				}
				require.NotNil(t, countChart, "Summary count chart should exist")
				assert.Equal(t, "Test Summary (Count)", countChart.Title)
				assert.Equal(t, "count", countChart.Units)
				assert.Equal(t, "line", countChart.Type)
				assert.Len(t, countChart.Dimensions, 1)
				assert.Equal(t, 100.0, countChart.Dimensions[0].Value)

				// Check sum chart
				var sumChart *ChartDefinition
				for _, chart := range charts {
					if chart.Context == "test.summary.sum" {
						sumChart = chart
						break
					}
				}
				require.NotNil(t, sumChart, "Summary sum chart should exist")
				assert.Equal(t, "Test Summary (Sum)", sumChart.Title)
				assert.Equal(t, "ms", sumChart.Units)
				assert.Equal(t, "line", sumChart.Type)
				assert.Len(t, sumChart.Dimensions, 1)
				assert.Equal(t, 5000.0, sumChart.Dimensions[0].Value)

				// Check quantiles chart
				var quantilesChart *ChartDefinition
				for _, chart := range charts {
					if chart.Context == "test.summary.quantiles" {
						quantilesChart = chart
						break
					}
				}
				require.NotNil(t, quantilesChart, "Summary quantiles chart should exist")
				assert.Equal(t, "Test Summary (Quantiles)", quantilesChart.Title)
				assert.Equal(t, "ms", quantilesChart.Units)
				assert.Equal(t, "line", quantilesChart.Type)
				assert.Len(t, quantilesChart.Dimensions, 5)

				// Find dimensions for min, max, and p50
				var minValue, maxValue, p50Value float64
				for _, dim := range quantilesChart.Dimensions {
					if dim.Name == "min" {
						minValue = dim.Value
					} else if dim.Name == "max" {
						maxValue = dim.Value
					} else if dim.Name == "p50" {
						p50Value = dim.Value
					}
				}

				assert.Equal(t, 1.0, minValue, "Minimum (p0) quantile should be 1.0")
				assert.Equal(t, 100.0, maxValue, "Maximum (p100) quantile should be 100.0")
				assert.Equal(t, 5.0, p50Value, "p50 quantile should be 5.0")

			case "multiple_data_points_by_attribute":
				assert.Len(t, charts, 2, "Should have two charts for the multiple data points test")

				// Collect all dimension values to ensure we have both 100.0 and 200.0
				values := make([]float64, 0, 2)
				for _, chart := range charts {
					assert.Equal(t, "test.gauge.multi", chart.Context, "Chart context should match metric name")
					assert.Equal(t, "Test Gauge with Multiple Points", chart.Title)
					assert.Equal(t, "bytes", chart.Units)

					for _, dim := range chart.Dimensions {
						values = append(values, dim.Value)
					}
				}

				assert.Contains(t, values, 100.0, "Should have a dimension with value 100.0")
				assert.Contains(t, values, 200.0, "Should have a dimension with value 200.0")

			case "staleness_marker":
				assert.Len(t, charts, 1, "Should have one chart for the staleness marker test")

				for _, chart := range charts {
					assert.Equal(t, "test.gauge.stale", chart.Context)
					assert.Equal(t, "Test Gauge with Staleness Marker", chart.Title)
					assert.Len(t, chart.Dimensions, 1, "Should have one dimension (non-stale point)")
					assert.Equal(t, 100.0, chart.Dimensions[0].Value, "Dimension value should be 100.0 (from non-stale point)")
				}

			case "resource_and_datapoint_attributes_as_labels":
				assert.Len(t, charts, 1, "Should have one chart for the labels test")

				for _, chart := range charts {
					assert.Equal(t, "test.labels", chart.Context)
					assert.Equal(t, "test-service_test", chart.Family, "Family should include service name")

					// Check for expected labels
					labelMap := make(map[string]string)
					for _, label := range chart.Labels {
						labelMap[label.Name] = label.Value
					}

					assert.Equal(t, "production", labelMap["deployment.environment"], "Should have deployment.environment label")
					assert.Equal(t, "test-service", labelMap["service.name"], "Should have service.name label")
					assert.Equal(t, "host1", labelMap["host"], "Should have host label")
					assert.Equal(t, "us-west", labelMap["region"], "Should have region label")

					// Check dimension
					assert.Len(t, chart.Dimensions, 1)
					assert.Equal(t, 42.0, chart.Dimensions[0].Value)
				}
			}
		})
	}
}

// Helper functions tests

func TestHelperFunctions(t *testing.T) {
	// Test sanitizeID
	t.Run("sanitizeID", func(t *testing.T) {
		tests := map[string]struct {
			input    string
			expected string
		}{
			"valid_id":               {"valid_id", "valid_id"},
			"spaces":                 {"test metric", "test_metric"},
			"special_chars":          {"test/metric:value", "test_metric_value"},
			"starts_with_number":     {"123test", "n_123test"},
			"multiple_special_chars": {"test@metric#value", "test_metric_value"},
			"empty_string":           {"", "unknown"},
			"very_long_id":           {strings.Repeat("a", 150), strings.Repeat("a", 150)},
		}

		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				result := sanitizeID(tt.input)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	// Test generateChartID
	t.Run("generateChartID", func(t *testing.T) {
		// Test that long metric names are truncated in the chart ID
		veryLongName := strings.Repeat("abcdefghij", 15) // 150 characters
		attrHash := "testhash"
		chartID := generateChartID(veryLongName, attrHash)

		// Chart ID should be truncated to maxChartIDLength
		assert.LessOrEqual(t, len(chartID), maxChartIDLength, "Chart ID should not exceed maxChartIDLength")

		// Chart ID should still end with the attribute hash
		assert.True(t, strings.HasSuffix(chartID, attrHash), "Chart ID should end with the attribute hash")
	})

	// Test attributesHash
	t.Run("attributesHash", func(t *testing.T) {
		// Test consistent hashing with different attribute orders
		attrs1 := pcommon.NewMap()
		attrs1.PutStr("a", "1")
		attrs1.PutStr("b", "2")

		attrs2 := pcommon.NewMap()
		attrs2.PutStr("b", "2")
		attrs2.PutStr("a", "1")

		hash1 := attributesHash(attrs1)
		hash2 := attributesHash(attrs2)

		assert.Equal(t, hash1, hash2, "Hash should be consistent regardless of attribute order")

		// Test empty attributes
		emptyAttrs := pcommon.NewMap()
		emptyHash := attributesHash(emptyAttrs)
		assert.Equal(t, "default", emptyHash, "Empty attributes should hash to 'default'")
	})

	// Test getFamily
	t.Run("getFamily", func(t *testing.T) {
		// Test with service name
		resourceAttrs := pcommon.NewMap()
		resourceAttrs.PutStr("service.name", "test-service")

		family := getFamily("test.metric", resourceAttrs)
		assert.Equal(t, "test-service_test", family, "Family should include service name")

		// Test without service name but with service namespace
		resourceAttrs = pcommon.NewMap()
		resourceAttrs.PutStr("service.namespace", "test-namespace")

		family = getFamily("test.metric", resourceAttrs)
		assert.Equal(t, "test_namespace_test", family, "Family should include service namespace")

		// Test without service name or namespace
		resourceAttrs = pcommon.NewMap()

		family = getFamily("test.metric", resourceAttrs)
		assert.Equal(t, "test", family, "Family should be based on metric name prefix")
	})
}
