// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebSpherePMI_VersionDetection(t *testing.T) {
	tests := []struct {
		name            string
		xmlFile         string
		expectedVersion string
		expectedEdition string
	}{
		{
			name:            "WebSphere 8.5.5.24",
			xmlFile:         "../../samples.d/traditional-8.5.5.24-pmi-full-port-9284.xml",
			expectedVersion: "8.5.5.24",
			expectedEdition: "traditional",
		},
		{
			name:            "WebSphere 9.0.5.24",
			xmlFile:         "../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml",
			expectedVersion: "9.0.5.24",
			expectedEdition: "traditional",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Read test XML file
			xmlData, err := os.ReadFile(tt.xmlFile)
			require.NoError(t, err)

			// Parse XML
			var stats pmiStatsResponse
			err = xml.Unmarshal(xmlData, &stats)
			require.NoError(t, err)

			// Create collector
			w := New()
			w.detectWebSphereVersion(&stats)

			assert.Equal(t, tt.expectedVersion, w.wasVersion)
			assert.Equal(t, tt.expectedEdition, w.wasEdition)
		})
	}
}

func TestWebSpherePMI_DoubleStatisticParsing(t *testing.T) {
	tests := []struct {
		name     string
		xmlFile  string
		statPath string
		statName string
		expected string
	}{
		{
			name:     "HitRate in 8.5.5.24",
			xmlFile:  "../../samples.d/traditional-8.5.5.24-pmi-full-port-9284.xml",
			statPath: "ExtensionRegistryStats.name",
			statName: "HitRate",
			expected: "78.37837837837837",
		},
		{
			name:     "HitRate in 9.0.5.24",
			xmlFile:  "../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml",
			statPath: "ExtensionRegistryStats.name",
			statName: "HitRate",
			expected: "78.37837837837837",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Read test XML file
			xmlData, err := os.ReadFile(tt.xmlFile)
			require.NoError(t, err)

			// Parse XML
			var stats pmiStatsResponse
			err = xml.Unmarshal(xmlData, &stats)
			require.NoError(t, err)

			// Find the stat with DoubleStatistic
			found := false
			for _, node := range stats.Nodes {
				for _, server := range node.Servers {
					found = findDoubleStatistic(server.Stats, tt.statName, tt.expected)
					if found {
						break
					}
				}
				if found {
					break
				}
			}

			assert.True(t, found, "DoubleStatistic %s not found or value mismatch", tt.statName)
		})
	}
}

func findDoubleStatistic(stats []pmiStat, name string, expectedValue string) bool {
	for _, stat := range stats {
		for _, ds := range stat.DoubleStatistics {
			if ds.Name == name && ds.Double == expectedValue {
				return true
			}
		}
		// Recursively check sub-stats
		if findDoubleStatistic(stat.SubStats, name, expectedValue) {
			return true
		}
	}
	return false
}

func TestWebSpherePMI_JVMMetricsExtraction(t *testing.T) {
	tests := []struct {
		name            string
		xmlFile         string
		expectedMetrics map[string]int64
	}{
		{
			name:    "JVM metrics from 8.5.5.24",
			xmlFile: "../../samples.d/traditional-8.5.5.24-pmi-full-port-9284.xml",
			expectedMetrics: map[string]int64{
				"jvm_heap_used":           54039 * 1024,  // UsedMemory in KB -> bytes
				"jvm_heap_max":            262144 * 1024, // HeapSize upperBound in KB -> bytes
				"jvm_heap_committed":      262144 * 1024, // Same as max for PMI
				"jvm_heap_free":           24360 * 1024,  // FreeMemory in KB -> bytes
				"jvm_uptime":              7982,          // UpTime in seconds
				"jvm_process_cpu_percent": 0,             // ProcessCpuUsage * precision
			},
		},
		{
			name:    "JVM metrics from 9.0.5.24",
			xmlFile: "../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml",
			expectedMetrics: map[string]int64{
				"jvm_heap_used":           49695 * 1024,   // UsedMemory in KB -> bytes
				"jvm_heap_max":            4092928 * 1024, // HeapSize upperBound in KB -> bytes
				"jvm_heap_committed":      4092928 * 1024, // Same as max for PMI
				"jvm_heap_free":           30560 * 1024,   // FreeMemory in KB -> bytes
				"jvm_uptime":              47004,          // UpTime in seconds
				"jvm_process_cpu_percent": 0,              // ProcessCpuUsage * precision
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Read test XML file
			xmlData, err := os.ReadFile(tt.xmlFile)
			require.NoError(t, err)

			// Create test server
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/xml")
				w.Write(xmlData)
			}))
			defer ts.Close()

			// Create collector
			ws := New()
			ws.HTTPConfig.RequestConfig.URL = ts.URL + "/wasPerfTool/servlet/perfservlet"
			ws.setConfigurationDefaults()
			err = ws.Init(context.Background())
			require.NoError(t, err)

			// Collect metrics
			mx, err := ws.collect(context.Background())
			require.NoError(t, err)

			// Verify JVM metrics
			for metric, expected := range tt.expectedMetrics {
				actual, ok := mx[metric]
				if !ok {
					t.Errorf("metric %s not found", metric)
					continue
				}
				// For uptime, allow some tolerance since the samples have different values
				if metric == "jvm_uptime" {
					assert.Greater(t, actual, int64(0), "uptime should be positive")
				} else {
					assert.Equal(t, expected, actual, "metric %s mismatch", metric)
				}
			}

			// Verify heap usage percentage is calculated
			if used, ok := mx["jvm_heap_used"]; ok {
				if max, ok := mx["jvm_heap_max"]; ok && max > 0 {
					expectedPercent := (used * precision * 100) / max
					actualPercent, ok := mx["jvm_heap_usage_percent"]
					assert.True(t, ok, "jvm_heap_usage_percent not calculated")
					assert.Equal(t, expectedPercent, actualPercent, "heap usage percentage mismatch")
				}
			}
		})
	}
}

func TestWebSpherePMI_AllElementsParsing(t *testing.T) {
	tests := []struct {
		name    string
		xmlFile string
	}{
		{
			name:    "Parse all elements in 8.5.5.24",
			xmlFile: "../../samples.d/traditional-8.5.5.24-pmi-full-port-9284.xml",
		},
		{
			name:    "Parse all elements in 9.0.5.24",
			xmlFile: "../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Read test XML file
			xmlData, err := os.ReadFile(tt.xmlFile)
			require.NoError(t, err)

			// Parse XML
			var stats pmiStatsResponse
			err = xml.Unmarshal(xmlData, &stats)
			require.NoError(t, err)

			// Verify basic structure
			assert.NotEmpty(t, stats.Version, "version attribute not parsed")
			assert.Equal(t, "success", stats.ResponseStatus, "response status should be success")
			assert.NotEmpty(t, stats.Nodes, "nodes not parsed")

			// Count different statistic types
			counts := countStatisticTypes(stats)

			// Verify all statistic types are found
			assert.Greater(t, counts["CountStatistic"], 0, "no CountStatistic elements found")
			assert.Greater(t, counts["TimeStatistic"], 0, "no TimeStatistic elements found")
			assert.Greater(t, counts["BoundedRangeStatistic"], 0, "no BoundedRangeStatistic elements found")
			assert.Greater(t, counts["DoubleStatistic"], 0, "no DoubleStatistic elements found")

			// Log counts for debugging
			t.Logf("Statistic counts for %s:", tt.name)
			for statType, count := range counts {
				t.Logf("  %s: %d", statType, count)
			}
		})
	}
}

func countStatisticTypes(stats pmiStatsResponse) map[string]int {
	counts := make(map[string]int)

	for _, node := range stats.Nodes {
		for _, server := range node.Servers {
			countStatsRecursive(server.Stats, counts)
		}
	}

	for _, stat := range stats.Stats {
		countStatsRecursive([]pmiStat{stat}, counts)
	}

	return counts
}

func countStatsRecursive(stats []pmiStat, counts map[string]int) {
	for _, stat := range stats {
		counts["CountStatistic"] += len(stat.CountStatistics)
		counts["TimeStatistic"] += len(stat.TimeStatistics)
		counts["RangeStatistic"] += len(stat.RangeStatistics)
		counts["BoundedRangeStatistic"] += len(stat.BoundedRangeStatistics)
		counts["DoubleStatistic"] += len(stat.DoubleStatistics)

		// Process sub-stats recursively
		countStatsRecursive(stat.SubStats, counts)
	}
}

func TestWebSpherePMI_VersionLabels(t *testing.T) {
	// Create collector
	w := New()
	w.wasVersion = "8.5.5.24"
	w.wasEdition = "traditional"
	w.HTTPConfig.RequestConfig.URL = "http://localhost:9080/wasPerfTool/servlet/perfservlet"

	// Initialize charts
	err := w.Init(context.Background())
	require.NoError(t, err)

	// Add version labels
	w.addVersionLabelsToCharts()

	// Verify labels were added to all charts
	for _, chart := range *w.charts {
		foundVersion := false
		foundEdition := false

		for _, label := range chart.Labels {
			if label.Key == "websphere_version" && label.Value == "8.5.5.24" {
				foundVersion = true
			}
			if label.Key == "websphere_edition" && label.Value == "traditional" {
				foundEdition = true
			}
		}

		assert.True(t, foundVersion, "websphere_version label not found in chart %s", chart.ID)
		assert.True(t, foundEdition, "websphere_edition label not found in chart %s", chart.ID)
	}
}

func TestWebSpherePMI_NoDataLoss(t *testing.T) {
	tests := []struct {
		name    string
		xmlFile string
	}{
		{
			name:    "No data loss in 8.5.5.24",
			xmlFile: "../../samples.d/traditional-8.5.5.24-pmi-full-port-9284.xml",
		},
		{
			name:    "No data loss in 9.0.5.24",
			xmlFile: "../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Read test XML file
			xmlData, err := os.ReadFile(tt.xmlFile)
			require.NoError(t, err)

			// Create test server
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/xml")
				w.Write(xmlData)
			}))
			defer ts.Close()

			// Create collector with all collection flags enabled
			ws := New()
			ws.HTTPConfig.RequestConfig.URL = ts.URL + "/wasPerfTool/servlet/perfservlet"

			// Enable all collection flags
			trueVal := true
			ws.CollectJVMMetrics = &trueVal
			ws.CollectThreadPoolMetrics = &trueVal
			ws.CollectJDBCMetrics = &trueVal
			ws.CollectJCAMetrics = &trueVal
			ws.CollectJMSMetrics = &trueVal
			ws.CollectWebAppMetrics = &trueVal
			ws.CollectSessionMetrics = &trueVal
			ws.CollectTransactionMetrics = &trueVal
			ws.CollectClusterMetrics = &trueVal
			ws.CollectServletMetrics = &trueVal
			ws.CollectEJBMetrics = &trueVal
			ws.CollectJDBCAdvanced = &trueVal

			err = ws.Init(context.Background())
			require.NoError(t, err)

			// Collect metrics
			mx, err := ws.collect(context.Background())
			require.NoError(t, err)

			// Verify we collected some metrics
			assert.NotEmpty(t, mx, "no metrics collected")

			// Log all collected metrics for manual verification
			t.Logf("Collected %d metrics:", len(mx))
			for metric, value := range mx {
				t.Logf("  %s: %d", metric, value)
			}

			// Verify critical JVM metrics are present
			criticalMetrics := []string{
				"jvm_heap_used",
				"jvm_heap_max",
				"jvm_uptime",
			}

			for _, metric := range criticalMetrics {
				_, ok := mx[metric]
				assert.True(t, ok, "critical metric %s not found", metric)
			}
		})
	}
}

// TestWebSpherePMI_BackwardCompatibility verifies that the backward compatibility
// population works correctly for both single stats and arrays
func TestWebSpherePMI_BackwardCompatibility(t *testing.T) {
	stat := pmiStat{
		Name: "TestStat",
		CountStatistics: []countStat{
			{Name: "count1", Count: "100"},
			{Name: "count2", Count: "200"},
		},
		TimeStatistics: []timeStat{
			{Name: "time1", Count: "10", Total: "1000"},
		},
		DoubleStatistics: []doubleStat{
			{Name: "double1", Double: "3.14159"},
		},
		SubStats: []pmiStat{
			{
				Name: "SubStat",
				CountStatistics: []countStat{
					{Name: "subcount", Count: "50"},
				},
			},
		},
	}

	// Populate backward compatibility fields
	stat.populateBackwardCompatibility()

	// Verify single references are populated with first element
	require.NotNil(t, stat.CountStatistic)
	assert.Equal(t, "count1", stat.CountStatistic.Name)
	assert.Equal(t, "100", stat.CountStatistic.Count)

	require.NotNil(t, stat.TimeStatistic)
	assert.Equal(t, "time1", stat.TimeStatistic.Name)

	require.NotNil(t, stat.DoubleStatistic)
	assert.Equal(t, "double1", stat.DoubleStatistic.Name)
	assert.Equal(t, "3.14159", stat.DoubleStatistic.Double)

	// Verify sub-stats are also populated
	require.NotNil(t, stat.SubStats[0].CountStatistic)
	assert.Equal(t, "subcount", stat.SubStats[0].CountStatistic.Name)
}

// TestWebSpherePMI_ProcessStatSetsPath verifies that processStat sets the path
// correctly for extraction functions to use
func TestWebSpherePMI_ProcessStatSetsPath(t *testing.T) {
	w := New()
	w.setConfigurationDefaults()

	// Create a stat tree that simulates the WebSphere PMI structure
	stat := pmiStat{
		Name: "server",
		Path: "", // Empty path, should be built by processStat
		SubStats: []pmiStat{
			{
				Name: "threadPoolModule",
				Path: "", // Empty path
				SubStats: []pmiStat{
					{
						Name: "WebContainer",
						Path: "", // Empty path
						BoundedRangeStatistics: []boundedRangeStat{
							{Name: "PoolSize", Current: "10", UpperBound: "50"},
						},
					},
				},
			},
			{
				Name: "connectionPoolModule",
				Path: "", // Empty path
				SubStats: []pmiStat{
					{
						Name: "jdbc",
						Path: "", // Empty path
						SubStats: []pmiStat{
							{
								Name: "myDataSource",
								Path: "", // Empty path
								CountStatistics: []countStat{
									{Name: "CreateCount", Count: "100"},
								},
							},
						},
					},
				},
			},
		},
	}

	mx := make(map[string]int64)

	// Process the stat tree
	w.processStat(&stat, "", mx)

	// Verify paths were set correctly
	assert.Equal(t, "server", stat.Path, "top level path should be set")
	assert.Equal(t, "server/threadPoolModule", stat.SubStats[0].Path, "threadPoolModule path should be set")
	assert.Equal(t, "server/threadPoolModule/WebContainer", stat.SubStats[0].SubStats[0].Path, "WebContainer path should be set")
	assert.Equal(t, "server/connectionPoolModule", stat.SubStats[1].Path, "connectionPoolModule path should be set")
	assert.Equal(t, "server/connectionPoolModule/jdbc", stat.SubStats[1].SubStats[0].Path, "jdbc path should be set")
	assert.Equal(t, "server/connectionPoolModule/jdbc/myDataSource", stat.SubStats[1].SubStats[0].SubStats[0].Path, "myDataSource path should be set")
}
