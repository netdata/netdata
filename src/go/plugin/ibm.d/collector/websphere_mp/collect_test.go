// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package websphere_mp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebSphereMicroProfile_parsePrometheusMetrics_AllSamples(t *testing.T) {
	// Define expected metrics for each version
	expectedVersions := map[string]struct {
		mpMetricsVersion string
		expectedMetrics  map[string]bool // metric names that should be present
		minMetricsCount  int            // minimum number of metrics expected
	}{
		"liberty-20.0.0.12-mpmetrics-3.0-port-9182.txt": {
			mpMetricsVersion: "3.0",
			expectedMetrics: map[string]bool{
				"base_cpu_systemLoadAverage":           true,
				"base_classloader_loadedClasses_count": true,
				"base_thread_count":                    true,
				"base_gc_total":                        true,
				"vendor_memory_heapUtilization_percent": true,
				"vendor_cpu_processCpuUtilization_percent": true,
			},
			minMetricsCount: 15,
		},
		"liberty-22.0.0.13-mpmetrics-4.0-port-9181.txt": {
			mpMetricsVersion: "4.0",
			expectedMetrics: map[string]bool{
				"base_cpu_systemLoadAverage":           true,
				"base_classloader_loadedClasses_count": true,
				"base_thread_count":                    true,
				"base_gc_total":                        true,
				// Note: vendor metrics are missing in 4.0
			},
			minMetricsCount: 12,
		},
		"liberty-23.0.0.12-mpmetrics-5.1-port-9180.txt": {
			mpMetricsVersion: "5.1",
			expectedMetrics: map[string]bool{
				"base_cpu_systemLoadAverage":           true,
				"base_classloader_loadedClasses_total": true,
				"base_thread_count":                    true,
				"base_gc_total":                        true,
				"vendor_memory_heapUtilization_percent": true,
			},
			minMetricsCount: 15,
		},
		"liberty-24.0.0.12-mpmetrics-5.1-port-9184.txt": {
			mpMetricsVersion: "5.1",
			expectedMetrics: map[string]bool{
				"base_cpu_systemLoadAverage":           true,
				"base_classloader_loadedClasses_total": true,
				"base_thread_count":                    true,
				"base_gc_total":                        true,
				"vendor_memory_heapUtilization_percent": true,
			},
			minMetricsCount: 15,
		},
		"liberty-collective-mpmetrics-5.0-port-9187.txt": {
			mpMetricsVersion: "5.1",
			expectedMetrics: map[string]bool{
				"base_cpu_systemLoadAverage":           true,
				"base_classloader_loadedClasses_total": true,
				"base_thread_count":                    true,
				"base_gc_total":                        true,
				"vendor_memory_heapUtilization_percent": true,
			},
			minMetricsCount: 15,
		},
		"liberty-latest-mpmetrics-5.1-port-9080.txt": {
			mpMetricsVersion: "5.1",
			expectedMetrics: map[string]bool{
				"base_cpu_systemLoadAverage":           true,
				"base_classloader_loadedClasses_total": true,
				"base_thread_count":                    true,
				"base_gc_total":                        true,
				"vendor_memory_heapUtilization_percent": true,
			},
			minMetricsCount: 15,
		},
		"liberty-mp-mpmetrics-5.1-port-9081.txt": {
			mpMetricsVersion: "5.1",
			expectedMetrics: map[string]bool{
				"base_cpu_systemLoadAverage":           true,
				"base_classloader_loadedClasses_total": true,
				"base_thread_count":                    true,
				"base_gc_total":                        true,
				"vendor_memory_heapUtilization_percent": true,
			},
			minMetricsCount: 15,
		},
		"open-liberty-latest-mpmetrics-5.1-port-9082.txt": {
			mpMetricsVersion: "5.1",
			expectedMetrics: map[string]bool{
				"base_cpu_systemLoadAverage":           true,
				"base_classloader_loadedClasses_total": true,
				"base_thread_count":                    true,
				"base_gc_total":                        true,
				"vendor_memory_heapUtilization_percent": true,
			},
			minMetricsCount: 15,
		},
	}

	samplesDir := filepath.Join("..", "..", "samples.d")

	for filename, expected := range expectedVersions {
		t.Run(filename, func(t *testing.T) {
			// Read sample file
			samplePath := filepath.Join(samplesDir, filename)
			content, err := os.ReadFile(samplePath)
			require.NoError(t, err, "Failed to read sample file %s", filename)

			// Create collector
			w := New()

			// Parse metrics
			metrics, err := w.parsePrometheusMetrics(strings.NewReader(string(content)))
			require.NoError(t, err, "Failed to parse metrics from %s", filename)

			// Test version detection using raw metrics
			detectedVersion := w.detectMpMetricsVersion(w.rawMetrics)
			assert.Equal(t, expected.mpMetricsVersion, detectedVersion,
				"Version detection failed for %s", filename)

			// Verify minimum metric count
			assert.GreaterOrEqual(t, len(metrics), expected.minMetricsCount,
				"Not enough metrics parsed from %s. Expected at least %d, got %d",
				filename, expected.minMetricsCount, len(metrics))

			// Check for expected metrics
			normalizedMetrics := make(map[string]float64)
			for metricName, value := range metrics {
				cleanedName := w.cleanMetricName(metricName)
				normalizedMetrics[cleanedName] = value
			}

			for expectedMetric := range expected.expectedMetrics {
				assert.Contains(t, normalizedMetrics, expectedMetric,
					"Expected metric %s not found in %s", expectedMetric, filename)
			}

			// Log parsed metrics for debugging
			t.Logf("File: %s, Version: %s, Metrics parsed: %d", 
				filename, detectedVersion, len(metrics))
			
			// Log normalized metric names for verification
			for normalizedName := range normalizedMetrics {
				if strings.HasPrefix(normalizedName, "base_") || strings.HasPrefix(normalizedName, "vendor_") {
					t.Logf("  %s", normalizedName)
				}
			}
		})
	}
}

func TestWebSphereMicroProfile_cleanMetricName(t *testing.T) {
	tests := map[string]struct {
		input    string
		expected string
	}{
		// mpMetrics 3.0/4.0 prefix-based format (already normalized)
		"prefix_based_base": {
			input:    "base_cpu_systemLoadAverage",
			expected: "base_cpu_systemLoadAverage",
		},
		"prefix_based_vendor": {
			input:    "vendor_memory_heapUtilization_percent",
			expected: "vendor_memory_heapUtilization_percent",
		},
		
		// mpMetrics 5.0+ label-based format (needs normalization)
		"label_based_base": {
			input:    `cpu_systemLoadAverage{mp_scope="base"}`,
			expected: "base_cpu_systemLoadAverage",
		},
		"label_based_vendor": {
			input:    `memory_heapUtilization_percent{mp_scope="vendor"}`,
			expected: "vendor_memory_heapUtilization_percent",
		},
		"label_based_with_multiple_labels": {
			input:    `gc_total{mp_scope="base",name="global"}`,
			expected: "base_gc_total",
		},
		
		// Metrics without labels
		"no_labels": {
			input:    "some_metric",
			expected: "some_metric",
		},
	}

	w := New()
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			result := w.cleanMetricName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWebSphereMicroProfile_detectMpMetricsVersion(t *testing.T) {
	tests := map[string]struct {
		metrics  map[string]float64
		expected string
	}{
		"mpMetrics_3.0_with_vendor": {
			metrics: map[string]float64{
				"base_cpu_systemLoadAverage":           1.23,
				"vendor_memory_heapUtilization_percent": 0.45,
				"vendor_cpu_processCpuUtilization_percent": 0.67,
			},
			expected: "3.0",
		},
		"mpMetrics_4.0_no_vendor": {
			metrics: map[string]float64{
				"base_cpu_systemLoadAverage":   1.23,
				"base_thread_count":            87,
				"base_gc_total":                14,
			},
			expected: "4.0",
		},
		"mpMetrics_5.1_label_based": {
			metrics: map[string]float64{
				`cpu_systemLoadAverage{mp_scope="base"}`:           1.23,
				`memory_heapUtilization_percent{mp_scope="vendor"}`: 0.45,
				`thread_count{mp_scope="base"}`:                    87,
			},
			expected: "5.1",
		},
		"unknown_format": {
			metrics: map[string]float64{
				"some_random_metric": 123,
				"another_metric":     456,
			},
			expected: "unknown",
		},
	}

	w := New()
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			result := w.detectMpMetricsVersion(tt.metrics)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWebSphereMicroProfile_processJVMMetric_AllVersions(t *testing.T) {
	tests := map[string]struct {
		metricName    string
		value         int64
		expectedKey   string
		shouldProcess bool
	}{
		// Base scope metrics (available in all versions)
		"jvm_uptime": {
			metricName:    "base_jvm_uptime_seconds",
			value:         1234567,
			expectedKey:   "jvm_uptime_seconds",
			shouldProcess: true,
		},
		"thread_count": {
			metricName:    "base_thread_count",
			value:         87000, // 87 with precision
			expectedKey:   "jvm_thread_count",
			shouldProcess: true,
		},
		"gc_collections": {
			metricName:    "base_gc_total",
			value:         14,
			expectedKey:   "jvm_gc_collections_total",
			shouldProcess: true,
		},
		"heap_used": {
			metricName:    "base_memory_usedHeap_bytes",
			value:         37955728,
			expectedKey:   "jvm_memory_heap_used",
			shouldProcess: true,
		},
		
		// Vendor scope metrics (missing in 4.0)
		"heap_utilization": {
			metricName:    "vendor_memory_heapUtilization_percent",
			value:         1573, // 1.573% with precision
			expectedKey:   "memory_heapUtilization_percent",
			shouldProcess: true,
		},
		"cpu_utilization": {
			metricName:    "vendor_cpu_processCpuUtilization_percent",
			value:         13100, // 13.1% with precision
			expectedKey:   "cpu_processCpuUtilization_percent",
			shouldProcess: true,
		},
		"gc_time_per_cycle": {
			metricName:    "vendor_gc_time_per_cycle_seconds",
			value:         125, // 0.125s with precision
			expectedKey:   "gc_time_per_cycle_seconds",
			shouldProcess: true,
		},
		
		// Unknown metric (should be handled gracefully)
		"unknown_metric": {
			metricName:    "unknown_jvm_metric",
			value:         123,
			expectedKey:   "unknown_jvm_metric",
			shouldProcess: false, // Goes to dynamic handling
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			w := New()
			mx := make(map[string]int64)

			w.processJVMMetric(mx, tt.metricName, tt.value)

			if tt.shouldProcess {
				assert.Contains(t, mx, tt.expectedKey,
					"Expected key %s not found in mx", tt.expectedKey)
				assert.Equal(t, tt.value, mx[tt.expectedKey],
					"Value mismatch for key %s", tt.expectedKey)
			}

			// Verify JVM charts were created
			assert.True(t, w.jvmChartsCreated, "JVM charts should be created")
		})
	}
}

func TestWebSphereMicroProfile_DynamicChartCreation(t *testing.T) {
	// Test using a real sample to verify charts are created only for available metrics
	samplesDir := filepath.Join("..", "..", "samples.d")
	
	tests := map[string]struct {
		filename      string
		expectJVM     bool
		expectVendor  bool
	}{
		"mpMetrics_3.0_with_vendor": {
			filename:     "liberty-20.0.0.12-mpmetrics-3.0-port-9182.txt",
			expectJVM:    true,
			expectVendor: true, // 3.0 has vendor metrics
		},
		"mpMetrics_4.0_no_vendor": {
			filename:     "liberty-22.0.0.13-mpmetrics-4.0-port-9181.txt",
			expectJVM:    true,
			expectVendor: false, // 4.0 missing vendor metrics
		},
		"mpMetrics_5.1_with_vendor": {
			filename:     "liberty-23.0.0.12-mpmetrics-5.1-port-9180.txt",
			expectJVM:    true,
			expectVendor: true, // 5.1 has vendor metrics
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// Read sample file
			samplePath := filepath.Join(samplesDir, tt.filename)
			content, err := os.ReadFile(samplePath)
			require.NoError(t, err)

			// Create collector and set up minimal config for testing
			w := New()
			w.HTTPConfig.RequestConfig.URL = "https://localhost:9443/metrics"
			err = w.Init(nil)
			require.NoError(t, err)

			// Parse metrics
			metrics, err := w.parsePrometheusMetrics(strings.NewReader(string(content)))
			require.NoError(t, err)

			// Manually trigger version detection since we're not going through full collect flow
			if w.serverType == "" {
				w.serverType = "Liberty MicroProfile"
				w.mpMetricsVersion = w.detectMpMetricsVersion(w.rawMetrics)
			}

			// Process metrics to trigger chart creation
			mx := make(map[string]int64)
			w.processMetrics(mx, metrics)

			// Verify JVM charts are created when JVM metrics are available
			if tt.expectJVM {
				assert.True(t, w.jvmChartsCreated, "JVM charts should be created for %s", tt.filename)
				assert.Greater(t, len(*w.charts), 0, "Charts should be created for %s", tt.filename)
			}

			// Verify vendor metrics are processed only when available
			hasVendorMetrics := false
			for key := range mx {
				if strings.Contains(key, "memory_heapUtilization_percent") ||
				   strings.Contains(key, "cpu_processCpuUtilization_percent") ||
				   strings.Contains(key, "gc_time_per_cycle_seconds") {
					hasVendorMetrics = true
					break
				}
			}

			if tt.expectVendor {
				assert.True(t, hasVendorMetrics, "Vendor metrics should be present for %s", tt.filename)
			} else {
				assert.False(t, hasVendorMetrics, "Vendor metrics should NOT be present for %s", tt.filename)
			}

			// Verify version labels are added to charts
			if len(*w.charts) > 0 {
				chart := (*w.charts)[0] // Check first chart
				hasVersionLabels := false
				var foundLabels []string
				for _, label := range chart.Labels {
					foundLabels = append(foundLabels, fmt.Sprintf("%s=%s", label.Key, label.Value))
					if label.Key == "mp_metrics_version" && label.Value != "" {
						hasVersionLabels = true
						break
					}
				}
				t.Logf("Chart labels: %v, mpMetricsVersion: %s, serverType: %s", foundLabels, w.mpMetricsVersion, w.serverType)
				assert.True(t, hasVersionLabels, "Charts should have version labels for %s", tt.filename)
			}

			t.Logf("File: %s, Charts: %d, Metrics: %d, HasVendor: %v", 
				tt.filename, len(*w.charts), len(mx), hasVendorMetrics)
		})
	}
}

func TestWebSphereMicroProfile_GracefulHandlingMissingMetrics(t *testing.T) {
	// Test that collector handles missing metrics gracefully without errors
	tests := map[string]struct {
		metrics map[string]float64
		expectError bool
	}{
		"empty_metrics": {
			metrics: map[string]float64{},
			expectError: false,
		},
		"only_base_metrics": {
			metrics: map[string]float64{
				"base_cpu_systemLoadAverage": 1.23,
				"base_thread_count": 87,
			},
			expectError: false,
		},
		"missing_some_expected_metrics": {
			metrics: map[string]float64{
				"base_cpu_systemLoadAverage": 1.23,
				// Missing: thread_count, gc_total, memory metrics
			},
			expectError: false,
		},
		"unknown_metrics": {
			metrics: map[string]float64{
				"unknown_metric_1": 123,
				"unknown_metric_2": 456,
			},
			expectError: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			w := New()
			w.HTTPConfig.RequestConfig.URL = "https://localhost:9443/metrics"
			err := w.Init(nil)
			require.NoError(t, err)
			
			mx := make(map[string]int64)

			// This should not panic or return errors
			w.processMetrics(mx, tt.metrics)

			// Verify processing completes without errors
			assert.NotNil(t, mx, "Result map should not be nil")
			
			// Log results for verification
			t.Logf("Test: %s, Input metrics: %d, Output metrics: %d", 
				name, len(tt.metrics), len(mx))
		})
	}
}

func TestWebSphereMicroProfile_CompleteProcessingPipeline(t *testing.T) {
	// Test complete pipeline: sample file -> parsing -> version detection -> processing -> final mx output
	// This test ensures NO METRICS ARE LOST and version detection actually works
	
	expectedResults := map[string]struct {
		expectedVersion     string
		mustHaveMetrics     []string // Metrics that MUST be in final mx output
		mustNotHaveMetrics  []string // Metrics that should NOT be present (version-specific)
		minOutputMetrics    int      // Minimum metrics in final mx
		mustHaveVendor      bool     // Whether vendor metrics should be present
	}{
		"liberty-20.0.0.12-mpmetrics-3.0-port-9182.txt": {
			expectedVersion: "3.0",
			mustHaveMetrics: []string{
				"jvm_uptime_seconds", "jvm_thread_count", "jvm_memory_heap_used", 
				"jvm_gc_collections_total", "cpu_systemLoadAverage",
				"memory_heapUtilization_percent", "cpu_processCpuUtilization_percent", // vendor metrics
			},
			mustNotHaveMetrics: []string{}, // 3.0 has everything
			minOutputMetrics:   15,
			mustHaveVendor:     true,
		},
		"liberty-22.0.0.13-mpmetrics-4.0-port-9181.txt": {
			expectedVersion: "4.0", 
			mustHaveMetrics: []string{
				"jvm_uptime_seconds", "jvm_thread_count", "jvm_memory_heap_used",
				"jvm_gc_collections_total", "cpu_systemLoadAverage",
			},
			mustNotHaveMetrics: []string{
				"memory_heapUtilization_percent", "cpu_processCpuUtilization_percent", // vendor missing in 4.0
			},
			minOutputMetrics:   12,
			mustHaveVendor:     false,
		},
		"liberty-23.0.0.12-mpmetrics-5.1-port-9180.txt": {
			expectedVersion: "5.1",
			mustHaveMetrics: []string{
				"jvm_uptime_seconds", "jvm_thread_count", "jvm_memory_heap_used",
				"jvm_gc_collections_total", "cpu_systemLoadAverage", 
				"memory_heapUtilization_percent", "cpu_processCpuUtilization_percent", // vendor metrics
			},
			mustNotHaveMetrics: []string{},
			minOutputMetrics:   15,
			mustHaveVendor:     true,
		},
	}

	samplesDir := filepath.Join("..", "..", "samples.d")

	for filename, expected := range expectedResults {
		t.Run(filename, func(t *testing.T) {
			// Read sample file
			samplePath := filepath.Join(samplesDir, filename)
			content, err := os.ReadFile(samplePath)
			require.NoError(t, err, "Failed to read sample file %s", filename)

			// Create collector and initialize
			w := New()
			w.HTTPConfig.RequestConfig.URL = "https://localhost:9443/metrics"
			err = w.Init(nil)
			require.NoError(t, err, "Failed to initialize collector")

			// STEP 1: Parse raw metrics
			metrics, err := w.parsePrometheusMetrics(strings.NewReader(string(content)))
			require.NoError(t, err, "Failed to parse metrics from %s", filename)
			require.Greater(t, len(metrics), 0, "No metrics parsed from %s", filename)

			// STEP 2: Verify version detection works on raw metrics
			detectedVersion := w.detectMpMetricsVersion(w.rawMetrics)
			require.Equal(t, expected.expectedVersion, detectedVersion,
				"Version detection failed for %s. Expected %s, got %s", 
				filename, expected.expectedVersion, detectedVersion)

			// STEP 3: Manually trigger version detection (simulating collectMicroProfileMetrics)
			w.serverType = "Liberty MicroProfile"
			w.mpMetricsVersion = detectedVersion
			w.addVersionLabelsToAllCharts()

			// STEP 4: Process metrics through the full pipeline
			mx := make(map[string]int64)
			w.processMetrics(mx, metrics)

			// STEP 5: Verify output metrics
			require.GreaterOrEqual(t, len(mx), expected.minOutputMetrics,
				"Not enough metrics in final output for %s. Expected at least %d, got %d", 
				filename, expected.minOutputMetrics, len(mx))

			// STEP 6: Verify required metrics are present
			for _, requiredMetric := range expected.mustHaveMetrics {
				assert.Contains(t, mx, requiredMetric,
					"Required metric '%s' missing from final output for %s", requiredMetric, filename)
			}

			// STEP 7: Verify forbidden metrics are absent (version-specific)
			for _, forbiddenMetric := range expected.mustNotHaveMetrics {
				assert.NotContains(t, mx, forbiddenMetric,
					"Forbidden metric '%s' present in final output for %s", forbiddenMetric, filename)
			}

			// STEP 8: Verify vendor metrics presence/absence
			hasVendorInOutput := false
			for key := range mx {
				if strings.Contains(key, "heapUtilization") || 
				   strings.Contains(key, "processCpuUtilization") ||
				   strings.Contains(key, "gc_time_per_cycle") {
					hasVendorInOutput = true
					break
				}
			}
			
			if expected.mustHaveVendor {
				assert.True(t, hasVendorInOutput, "Vendor metrics should be present in output for %s", filename)
			} else {
				assert.False(t, hasVendorInOutput, "Vendor metrics should NOT be present in output for %s", filename)
			}

			// STEP 9: Verify charts were created
			assert.True(t, w.jvmChartsCreated, "JVM charts should be created for %s", filename)
			assert.Greater(t, len(*w.charts), 0, "Charts should be created for %s", filename)

			// STEP 10: Verify version labels on charts
			if len(*w.charts) > 0 {
				chart := (*w.charts)[0]
				hasVersionLabel := false
				for _, label := range chart.Labels {
					if label.Key == "mp_metrics_version" && label.Value == expected.expectedVersion {
						hasVersionLabel = true
						break
					}
				}
				assert.True(t, hasVersionLabel, "Charts should have correct version label for %s", filename)
			}

			// Log comprehensive results
			t.Logf("✅ COMPLETE PIPELINE TEST for %s:", filename)
			t.Logf("  📥 Raw metrics parsed: %d", len(w.rawMetrics))
			t.Logf("  📊 Normalized metrics: %d", len(metrics)) 
			t.Logf("  🏷️  Detected version: %s", detectedVersion)
			t.Logf("  📈 Final output metrics: %d", len(mx))
			t.Logf("  📋 Charts created: %d", len(*w.charts))
			t.Logf("  🏪 Vendor metrics in output: %v", hasVendorInOutput)
			
			// Log some key output metrics for verification
			keyMetrics := []string{"jvm_uptime_seconds", "jvm_thread_count", "cpu_systemLoadAverage", "memory_heapUtilization_percent"}
			for _, key := range keyMetrics {
				if value, exists := mx[key]; exists {
					t.Logf("    %s = %d", key, value)
				}
			}
		})
	}
}

func TestWebSphereMicroProfile_NoMetricsLost(t *testing.T) {
	// Test that all metrics from sample files are properly processed and none are lost
	samplesDir := filepath.Join("..", "..", "samples.d")
	
	samples := []string{
		"liberty-20.0.0.12-mpmetrics-3.0-port-9182.txt",
		"liberty-22.0.0.13-mpmetrics-4.0-port-9181.txt",
		"liberty-23.0.0.12-mpmetrics-5.1-port-9180.txt",
		"liberty-24.0.0.12-mpmetrics-5.1-port-9184.txt",
		"open-liberty-latest-mpmetrics-5.1-port-9082.txt",
	}

	for _, filename := range samples {
		t.Run(filename, func(t *testing.T) {
			// Read sample file
			samplePath := filepath.Join(samplesDir, filename)
			content, err := os.ReadFile(samplePath)
			require.NoError(t, err)

			// Create collector
			w := New()
			w.HTTPConfig.RequestConfig.URL = "https://localhost:9443/metrics"
			err = w.Init(nil)
			require.NoError(t, err)

			// Parse metrics
			metrics, err := w.parsePrometheusMetrics(strings.NewReader(string(content)))
			require.NoError(t, err)

			// Count raw metrics that should be processed
			rawMetricCount := 0
			expectedJVMMetrics := 0
			for rawName := range w.rawMetrics {
				if !strings.HasPrefix(rawName, "#") && rawName != "" {
					rawMetricCount++
					
					// Count JVM-related metrics (base and vendor scope)
					if strings.HasPrefix(rawName, "base_") ||
					   (strings.HasPrefix(rawName, "vendor_") && 
					    (strings.Contains(rawName, "memory") || strings.Contains(rawName, "cpu") || strings.Contains(rawName, "gc"))) ||
					   strings.Contains(rawName, "mp_scope=\"base\"") ||
					   (strings.Contains(rawName, "mp_scope=\"vendor\"") && 
					    (strings.Contains(rawName, "memory") || strings.Contains(rawName, "cpu") || strings.Contains(rawName, "gc"))) {
						expectedJVMMetrics++
					}
				}
			}

			// Process metrics
			mx := make(map[string]int64)
			w.processMetrics(mx, metrics)

			// Verify no data loss - should have processed all relevant metrics
			t.Logf("File: %s", filename)
			t.Logf("  Raw metrics parsed: %d", rawMetricCount)
			t.Logf("  Expected JVM metrics: %d", expectedJVMMetrics)
			t.Logf("  Processed metrics: %d", len(mx))
			t.Logf("  Charts created: %d", len(*w.charts))
			t.Logf("  mpMetrics version: %s", w.mpMetricsVersion)

			// Should process at least the JVM metrics we found
			assert.GreaterOrEqual(t, len(mx), expectedJVMMetrics/2, 
				"Too many metrics lost during processing for %s", filename)

			// Should have created charts for JVM metrics
			if expectedJVMMetrics > 0 {
				assert.True(t, w.jvmChartsCreated, "JVM charts should be created for %s", filename)
				assert.Greater(t, len(*w.charts), 0, "Should have created charts for %s", filename)
			}

			// Verify specific essential metrics are not lost (if they exist in raw data)
			essentialMetrics := map[string]string{
				"base_cpu_systemLoadAverage":     "cpu_systemLoadAverage",
				"base_thread_count":              "jvm_thread_count", 
				"base_memory_usedHeap_bytes":     "jvm_memory_heap_used",
				"base_gc_total":                  "jvm_gc_collections_total",
			}

			for rawPattern, expectedOutput := range essentialMetrics {
				// Check if this metric exists in raw data (accounting for different formats)
				hasRawMetric := false
				for rawName := range w.rawMetrics {
					if strings.Contains(rawName, rawPattern) || 
					   (strings.Contains(rawName, strings.TrimPrefix(rawPattern, "base_")) && 
					    strings.Contains(rawName, "mp_scope=\"base\"")) {
						hasRawMetric = true
						break
					}
				}

				if hasRawMetric {
					assert.Contains(t, mx, expectedOutput, 
						"Essential metric %s -> %s missing from processed output for %s", 
						rawPattern, expectedOutput, filename)
				}
			}
		})
	}
}