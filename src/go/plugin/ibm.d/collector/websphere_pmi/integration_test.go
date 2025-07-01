// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ChartOutput represents the expected output for a chart
type ChartOutput struct {
	ID         string            `json:"id"`
	Title      string            `json:"title"`
	Units      string            `json:"units"`
	Family     string            `json:"family"`
	Context    string            `json:"context"`
	Type       string            `json:"type"`
	Priority   int               `json:"priority"`
	Labels     map[string]string `json:"labels"`
	Dimensions []DimensionOutput `json:"dimensions"`
}

// DimensionOutput represents the expected output for a dimension
type DimensionOutput struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Mul    int    `json:"mul,omitempty"`
	Div    int    `json:"div,omitempty"`
	Hidden bool   `json:"hidden,omitempty"`

	// Include the actual value from the source data
	Value int64 `json:"value"`
	// Include source path for traceability
	SourcePath string `json:"source_path"`
}

// IntegrationTestOutput represents the complete test output
type IntegrationTestOutput struct {
	TotalSourceMetrics int           `json:"total_source_metrics"`
	TotalCharts        int           `json:"total_charts"`
	TotalDimensions    int           `json:"total_dimensions"`
	Charts             []ChartOutput `json:"charts"`

	// Validation results
	ValidationErrors []string `json:"validation_errors,omitempty"`
}

// TestDynamicCollectionIntegration tests the entire pipeline from XML to Netdata charts
func TestDynamicCollectionIntegration(t *testing.T) {
	testCases := []struct {
		name           string
		xmlFile        string
		goldenFile     string
		generateGolden bool // Set to true to generate golden file
	}{
		{
			name:           "WebSphere Traditional 9.0.5.x Full PMI",
			xmlFile:        "../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml",
			goldenFile:     "testdata/integration_golden_traditional_9.0.5.json",
			generateGolden: true, // Set to false once golden file is validated
		},
		{
			name:           "WebSphere Traditional 8.5.5.24 Full PMI",
			xmlFile:        "../../samples.d/traditional-8.5.5.24-pmi-full-port-9284.xml",
			goldenFile:     "testdata/integration_golden_traditional_8.5.5.json",
			generateGolden: true, // Set to false once golden file is validated
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Load XML data
			xmlData, err := os.ReadFile(tc.xmlFile)
			require.NoError(t, err)

			var stats pmiStatsResponse
			err = xml.Unmarshal(xmlData, &stats)
			require.NoError(t, err)

			// Step 1: Flatten
			flattener := NewXMLFlattener()
			flatResult := flattener.FlattenPMIStats(&stats)

			// Step 2: Correlate
			correlator := NewCorrelationEngine()
			chartCandidates := correlator.CorrelateMetrics(flatResult.Metrics)

			// Step 3: Convert to Netdata charts
			netdataCharts := correlator.ConvertToNetdataCharts(chartCandidates)

			// Build output structure with validation
			output := buildTestOutput(flatResult, chartCandidates, netdataCharts)

			if tc.generateGolden {
				// Generate golden file
				saveGoldenFile(t, tc.goldenFile, output)
				t.Logf("Generated golden file: %s", tc.goldenFile)
				t.Logf("Total source metrics: %d", output.TotalSourceMetrics)
				t.Logf("Total charts: %d", output.TotalCharts)
				t.Logf("Total dimensions: %d", output.TotalDimensions)
				if len(output.ValidationErrors) > 0 {
					t.Logf("Validation errors found:")
					for _, err := range output.ValidationErrors {
						t.Logf("  - %s", err)
					}
				}
			} else {
				// Compare with golden file
				golden := loadGoldenFile(t, tc.goldenFile)
				compareOutputs(t, golden, output)
			}
		})
	}
}

// buildTestOutput builds the test output structure with validation
func buildTestOutput(flatResult *FlattenerResult, candidates []ChartCandidate, charts *module.Charts) IntegrationTestOutput {
	output := IntegrationTestOutput{
		TotalSourceMetrics: len(flatResult.Metrics),
		TotalCharts:        len(*charts),
		Charts:             make([]ChartOutput, 0, len(*charts)),
		ValidationErrors:   []string{},
	}

	// Map to track metric path to value
	metricValues := make(map[string]int64)
	for _, metric := range flatResult.Metrics {
		metricValues[metric.Path] = metric.Value
	}

	// Process each chart
	totalDims := 0
	for i, chart := range *charts {
		candidate := candidates[i]

		chartOut := ChartOutput{
			ID:         chart.ID,
			Title:      chart.Title,
			Units:      chart.Units,
			Family:     chart.Fam,
			Context:    chart.Ctx,
			Type:       string(chart.Type),
			Priority:   chart.Priority,
			Labels:     make(map[string]string),
			Dimensions: make([]DimensionOutput, 0, len(chart.Dims)),
		}

		// Convert labels
		for _, label := range chart.Labels {
			chartOut.Labels[label.Key] = label.Value
		}

		// Process dimensions
		for j, dim := range chart.Dims {
			dimOut := DimensionOutput{
				ID:     dim.ID,
				Name:   dim.Name,
				Mul:    dim.Mul,
				Div:    dim.Div,
				Hidden: dim.Hidden,
			}

			// Get source metric info
			if j < len(candidate.Dimensions) {
				sourcePath := candidate.Dimensions[j].Metric.Path
				dimOut.SourcePath = sourcePath
				dimOut.Value = candidate.Dimensions[j].Metric.Value
			}

			chartOut.Dimensions = append(chartOut.Dimensions, dimOut)
			totalDims++
		}

		// Validate chart
		validateChart(chartOut, &output.ValidationErrors)

		output.Charts = append(output.Charts, chartOut)
	}

	output.TotalDimensions = totalDims

	// Sort charts for consistent output
	sort.Slice(output.Charts, func(i, j int) bool {
		return output.Charts[i].Context < output.Charts[j].Context
	})

	return output
}

// validateChart validates a chart against Netdata rules
func validateChart(chart ChartOutput, errors *[]string) {
	// Rule: Context must not be empty and should follow pattern
	if chart.Context == "" {
		*errors = append(*errors, fmt.Sprintf("Chart %s: empty context", chart.ID))
	}
	if !strings.HasPrefix(chart.Context, "websphere_pmi.") {
		*errors = append(*errors, fmt.Sprintf("Chart %s: context should start with 'websphere_pmi.'", chart.ID))
	}

	// Rule: Title must not be empty
	if chart.Title == "" {
		*errors = append(*errors, fmt.Sprintf("Chart %s: empty title", chart.ID))
	}

	// Rule: Units must be valid
	validUnits := map[string]bool{
		"bytes": true, "kilobytes": true, "megabytes": true, "gigabytes": true,
		"requests": true, "requests/s": true, "operations/s": true,
		"milliseconds": true, "seconds": true, "minutes": true,
		"percentage": true, "percent": true,
		"connections": true, "threads": true, "processes": true,
		"errors": true, "packets": true, "queries": true,
		"items": true, "messages": true, "sessions": true,
		"bytes/s": true,
	}
	if !validUnits[chart.Units] {
		*errors = append(*errors, fmt.Sprintf("Chart %s: invalid units '%s'", chart.ID, chart.Units))
	}

	// Rule: Chart type must be valid
	validTypes := map[string]bool{"line": true, "area": true, "stacked": true}
	if !validTypes[chart.Type] {
		*errors = append(*errors, fmt.Sprintf("Chart %s: invalid type '%s'", chart.ID, chart.Type))
	}

	// Rule: Must have at least one dimension
	if len(chart.Dimensions) == 0 {
		*errors = append(*errors, fmt.Sprintf("Chart %s: no dimensions", chart.ID))
	}

	// Rule: All dimensions must have the same unit (implicit from chart unit)
	// This is checked by the correlator

	// Rule: Dimension IDs must be unique within chart
	dimIDs := make(map[string]bool)
	for _, dim := range chart.Dimensions {
		if dimIDs[dim.ID] {
			*errors = append(*errors, fmt.Sprintf("Chart %s: duplicate dimension ID '%s'", chart.ID, dim.ID))
		}
		dimIDs[dim.ID] = true

		// Rule: Dimension ID must not contain invalid characters
		if strings.ContainsAny(dim.ID, " .:#/@()[]{}\"'<>?|~`!$%^&*+=;,") {
			*errors = append(*errors, fmt.Sprintf("Chart %s: dimension ID '%s' contains invalid characters", chart.ID, dim.ID))
		}

		// Rule: Dimension must have a name
		if dim.Name == "" {
			*errors = append(*errors, fmt.Sprintf("Chart %s: dimension '%s' has empty name", chart.ID, dim.ID))
		}
	}

	// Rule: Priority should be reasonable
	if chart.Priority < 1000 || chart.Priority > 100000 {
		*errors = append(*errors, fmt.Sprintf("Chart %s: unusual priority %d", chart.ID, chart.Priority))
	}

	// Rule: Family should not be empty
	if chart.Family == "" {
		*errors = append(*errors, fmt.Sprintf("Chart %s: empty family", chart.ID))
	}
}

// saveGoldenFile saves the output as a golden file
func saveGoldenFile(t *testing.T, filename string, output IntegrationTestOutput) {
	// Ensure directory exists
	dir := filepath.Dir(filename)
	err := os.MkdirAll(dir, 0755)
	require.NoError(t, err)

	// Marshal with indentation for readability
	data, err := json.MarshalIndent(output, "", "  ")
	require.NoError(t, err)

	err = os.WriteFile(filename, data, 0644)
	require.NoError(t, err)
}

// loadGoldenFile loads a golden file
func loadGoldenFile(t *testing.T, filename string) IntegrationTestOutput {
	data, err := os.ReadFile(filename)
	require.NoError(t, err)

	var output IntegrationTestOutput
	err = json.Unmarshal(data, &output)
	require.NoError(t, err)

	return output
}

// compareOutputs compares actual output with golden file
func compareOutputs(t *testing.T, expected, actual IntegrationTestOutput) {
	// Compare high-level stats
	assert.Equal(t, expected.TotalSourceMetrics, actual.TotalSourceMetrics, "Total source metrics mismatch")
	assert.Equal(t, expected.TotalCharts, actual.TotalCharts, "Total charts mismatch")
	assert.Equal(t, expected.TotalDimensions, actual.TotalDimensions, "Total dimensions mismatch")

	// Compare validation errors
	assert.Equal(t, len(expected.ValidationErrors), len(actual.ValidationErrors), "Validation error count mismatch")

	// Compare charts
	assert.Equal(t, len(expected.Charts), len(actual.Charts), "Chart count mismatch")

	// Detailed chart comparison
	for i, expectedChart := range expected.Charts {
		if i >= len(actual.Charts) {
			t.Errorf("Missing chart at index %d: %s", i, expectedChart.ID)
			continue
		}

		actualChart := actual.Charts[i]
		assert.Equal(t, expectedChart.ID, actualChart.ID, "Chart ID mismatch at index %d", i)
		assert.Equal(t, expectedChart.Title, actualChart.Title, "Chart title mismatch for %s", expectedChart.ID)
		assert.Equal(t, expectedChart.Units, actualChart.Units, "Chart units mismatch for %s", expectedChart.ID)
		assert.Equal(t, expectedChart.Family, actualChart.Family, "Chart family mismatch for %s", expectedChart.ID)
		assert.Equal(t, expectedChart.Context, actualChart.Context, "Chart context mismatch for %s", expectedChart.ID)
		assert.Equal(t, expectedChart.Type, actualChart.Type, "Chart type mismatch for %s", expectedChart.ID)
		assert.Equal(t, expectedChart.Priority, actualChart.Priority, "Chart priority mismatch for %s", expectedChart.ID)
		assert.Equal(t, expectedChart.Labels, actualChart.Labels, "Chart labels mismatch for %s", expectedChart.ID)

		// Compare dimensions
		assert.Equal(t, len(expectedChart.Dimensions), len(actualChart.Dimensions),
			"Dimension count mismatch for chart %s", expectedChart.ID)

		for j, expectedDim := range expectedChart.Dimensions {
			if j >= len(actualChart.Dimensions) {
				t.Errorf("Missing dimension at index %d in chart %s: %s", j, expectedChart.ID, expectedDim.ID)
				continue
			}

			actualDim := actualChart.Dimensions[j]
			assert.Equal(t, expectedDim.ID, actualDim.ID, "Dimension ID mismatch in chart %s", expectedChart.ID)
			assert.Equal(t, expectedDim.Name, actualDim.Name, "Dimension name mismatch in chart %s", expectedChart.ID)
			assert.Equal(t, expectedDim.Mul, actualDim.Mul, "Dimension Mul mismatch in chart %s", expectedChart.ID)
			assert.Equal(t, expectedDim.Div, actualDim.Div, "Dimension Div mismatch in chart %s", expectedChart.ID)
			assert.Equal(t, expectedDim.Hidden, actualDim.Hidden, "Dimension Hidden mismatch in chart %s", expectedChart.ID)
			assert.Equal(t, expectedDim.Value, actualDim.Value, "Dimension value mismatch in chart %s", expectedChart.ID)
			assert.Equal(t, expectedDim.SourcePath, actualDim.SourcePath, "Dimension source path mismatch in chart %s", expectedChart.ID)
		}
	}
}

// TestDynamicCollectionMetricCoverage ensures no metrics are lost
func TestDynamicCollectionMetricCoverage(t *testing.T) {
	// Load XML data
	xmlData, err := os.ReadFile("../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml")
	require.NoError(t, err)

	var stats pmiStatsResponse
	err = xml.Unmarshal(xmlData, &stats)
	require.NoError(t, err)

	// Flatten
	flattener := NewXMLFlattener()
	flatResult := flattener.FlattenPMIStats(&stats)

	// Correlate
	correlator := NewCorrelationEngine()
	chartCandidates := correlator.CorrelateMetrics(flatResult.Metrics)

	// Count metrics in charts
	metricsInCharts := make(map[string]bool)
	for _, chart := range chartCandidates {
		for _, dim := range chart.Dimensions {
			metricsInCharts[dim.Metric.Path] = true
		}
	}

	// Check coverage
	uncoveredMetrics := []string{}
	for _, metric := range flatResult.Metrics {
		if !metricsInCharts[metric.Path] {
			uncoveredMetrics = append(uncoveredMetrics, metric.Path)
		}
	}

	// Report
	t.Logf("Total source metrics: %d", len(flatResult.Metrics))
	t.Logf("Metrics in charts: %d", len(metricsInCharts))
	t.Logf("Coverage: %.1f%%", float64(len(metricsInCharts))*100/float64(len(flatResult.Metrics)))

	if len(uncoveredMetrics) > 0 {
		t.Logf("Uncovered metrics:")
		for _, path := range uncoveredMetrics {
			t.Logf("  - %s", path)
		}
	}

	// All metrics should be covered
	assert.Equal(t, len(flatResult.Metrics), len(metricsInCharts), "Some metrics are not included in any chart")
}

// TestChartHierarchyAndLabels validates chart families and label hierarchy
func TestChartHierarchyAndLabels(t *testing.T) {
	xmlData, err := os.ReadFile("../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml")
	require.NoError(t, err)

	var stats pmiStatsResponse
	err = xml.Unmarshal(xmlData, &stats)
	require.NoError(t, err)

	flattener := NewXMLFlattener()
	flatResult := flattener.FlattenPMIStats(&stats)

	correlator := NewCorrelationEngine()
	chartCandidates := correlator.CorrelateMetrics(flatResult.Metrics)
	netdataCharts := correlator.ConvertToNetdataCharts(chartCandidates)

	// Build family hierarchy
	familyHierarchy := make(map[string][]string) // family -> list of contexts
	for _, chart := range *netdataCharts {
		familyHierarchy[chart.Fam] = append(familyHierarchy[chart.Fam], chart.Ctx)
	}

	// Log hierarchy
	t.Logf("Chart Family Hierarchy:")
	families := make([]string, 0, len(familyHierarchy))
	for fam := range familyHierarchy {
		families = append(families, fam)
	}
	sort.Strings(families)

	for _, fam := range families {
		contexts := familyHierarchy[fam]
		sort.Strings(contexts)
		t.Logf("  %s (%d charts)", fam, len(contexts))
		// Show first few contexts as examples
		for i, ctx := range contexts {
			if i < 3 {
				t.Logf("    - %s", ctx)
			} else if i == 3 {
				t.Logf("    ... and %d more", len(contexts)-3)
				break
			}
		}
	}

	// Validate labels
	for _, chart := range *netdataCharts {
		// All charts should have basic labels
		assert.NotNil(t, chart.Labels, "Chart %s should have labels", chart.ID)

		// Check for instance-specific charts
		hasInstance := false
		for _, label := range chart.Labels {
			if label.Key == "instance" {
				hasInstance = true
				assert.NotEmpty(t, label.Value, "Instance label should not be empty in chart %s", chart.ID)
			}
		}

		// Log example of instance chart
		if hasInstance && strings.Contains(chart.Ctx, "thread_pools") {
			t.Logf("Example instance chart: %s (instance: %s)", chart.Title, chart.Labels[0].Value)
		}
	}
}

// keys helper function
func keys(m map[string]bool) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
}
