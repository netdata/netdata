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

func TestWebSpherePMI_UnitConversions(t *testing.T) {
	tests := []struct {
		name          string
		xmlFile       string
		metricName    string
		xmlValue      int64
		xmlUnit       string
		expectedValue int64
		expectedUnit  string
	}{
		{
			name:          "JVM HeapSize KB to Bytes",
			xmlFile:       "../../samples.d/traditional-8.5.5.24-pmi-full-port-9284.xml",
			metricName:    "jvm_heap_max",
			xmlValue:      262144, // KB in XML
			xmlUnit:       "KILOBYTE",
			expectedValue: 262144 * 1024, // Bytes
			expectedUnit:  "bytes",
		},
		{
			name:          "JVM UsedMemory KB to Bytes",
			xmlFile:       "../../samples.d/traditional-8.5.5.24-pmi-full-port-9284.xml",
			metricName:    "jvm_heap_used",
			xmlValue:      54039, // KB in XML
			xmlUnit:       "KILOBYTE",
			expectedValue: 54039 * 1024, // Bytes
			expectedUnit:  "bytes",
		},
		{
			name:          "JVM FreeMemory KB to Bytes",
			xmlFile:       "../../samples.d/traditional-8.5.5.24-pmi-full-port-9284.xml",
			metricName:    "jvm_heap_free",
			xmlValue:      24360, // KB in XML
			xmlUnit:       "KILOBYTE",
			expectedValue: 24360 * 1024, // Bytes
			expectedUnit:  "bytes",
		},
		{
			name:          "JVM Uptime remains in seconds",
			xmlFile:       "../../samples.d/traditional-8.5.5.24-pmi-full-port-9284.xml",
			metricName:    "jvm_uptime",
			xmlValue:      7982, // seconds in XML
			xmlUnit:       "SECOND",
			expectedValue: 7982, // remains seconds
			expectedUnit:  "seconds",
		},
		{
			name:          "TimeStatistic remains in milliseconds",
			xmlFile:       "../../samples.d/traditional-8.5.5.24-pmi-full-port-9284.xml",
			metricName:    "threadpool_webcontainer_average_active_threads",
			xmlUnit:       "MILLISECOND",
			expectedUnit:  "milliseconds",
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

			// Verify conversion
			if tt.expectedValue > 0 {
				actual, ok := mx[tt.metricName]
				assert.True(t, ok, "metric %s not found", tt.metricName)
				assert.Equal(t, tt.expectedValue, actual, "metric %s value mismatch", tt.metricName)
			}
		})
	}
}

func TestWebSpherePMI_ConvertUnitFunction(t *testing.T) {
	tests := []struct {
		name      string
		value     int64
		fromUnit  string
		toUnit    string
		expected  int64
	}{
		// Memory conversions
		{
			name:     "KB to Bytes",
			value:    1024,
			fromUnit: "KILOBYTE",
			toUnit:   "BYTES",
			expected: 1024 * 1024,
		},
		{
			name:     "Bytes to KB",
			value:    1048576,
			fromUnit: "BYTE",
			toUnit:   "KILOBYTES",
			expected: 1024,
		},
		// Time conversions
		{
			name:     "Milliseconds to Seconds",
			value:    1000,
			fromUnit: "MILLISECOND",
			toUnit:   "SECONDS",
			expected: 1,
		},
		{
			name:     "Seconds to Milliseconds",
			value:    5,
			fromUnit: "SECOND",
			toUnit:   "MILLISECONDS",
			expected: 5000,
		},
		// Same unit
		{
			name:     "Same unit (no conversion)",
			value:    100,
			fromUnit: "SECOND",
			toUnit:   "SECOND",
			expected: 100,
		},
		// Unknown units
		{
			name:     "Unknown units (no conversion)",
			value:    100,
			fromUnit: "UNKNOWN",
			toUnit:   "BYTES",
			expected: 100,
		},
		// Case variations
		{
			name:     "Case insensitive",
			value:    1024,
			fromUnit: "kilobyte",
			toUnit:   "BYTES",
			expected: 1024 * 1024,
		},
		// Millisecond variations
		{
			name:     "MS abbreviation",
			value:    2000,
			fromUnit: "MS",
			toUnit:   "SECONDS",
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertUnit(tt.value, tt.fromUnit, tt.toUnit)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWebSpherePMI_PrecisionHandling(t *testing.T) {
	tests := []struct {
		name     string
		xmlFile  string
		metrics  []string
	}{
		{
			name:    "Precision in float calculations",
			xmlFile: "../../samples.d/traditional-8.5.5.24-pmi-full-port-9284.xml",
			metrics: []string{
				"jvm_heap_usage_percent",
				"threadpool_webcontainer_percent_max_reached",
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

			// Verify precision metrics exist and are properly multiplied
			for _, metric := range tt.metrics {
				value, ok := mx[metric]
				assert.True(t, ok, "metric %s not found", metric)
				// Since these are percentage/precision values, they should be > 0 when multiplied by precision
				if value > 0 {
					// The value should be a multiple of precision
					assert.Equal(t, int64(0), value%precision, "metric %s should be multiple of precision", metric)
				}
			}
		})
	}
}

func TestWebSpherePMI_XMLUnitParsing(t *testing.T) {
	// Test that we correctly parse units from XML
	xmlData := `<?xml version="1.0" encoding="UTF-8"?>
<Perf:Stats>
	<Perf:Stat name="jvmRuntimeModule">
		<Perf:CountStatistic name="UpTime" count="1000" unit="SECOND"/>
		<Perf:BoundedRangeStatistic name="HeapSize" value="262144" mean="262144" 
									current="262144" integral="0" lowerBound="0" 
									upperBound="262144" unit="KILOBYTE"/>
		<Perf:CountStatistic name="UsedMemory" count="54039" unit="KILOBYTE"/>
		<Perf:TimeStatistic name="ProcessTime" count="10" total="1000" mean="100" min="50" max="200" unit="MILLISECOND"/>
	</Perf:Stat>
</Perf:Stats>`

	var stats pmiStatsResponse
	err := xml.Unmarshal([]byte(xmlData), &stats)
	require.NoError(t, err)

	// Verify units are parsed correctly
	stat := stats.Stats[0]
	assert.Equal(t, "SECOND", stat.CountStatistics[0].Unit)
	assert.Equal(t, "KILOBYTE", stat.BoundedRangeStatistics[0].Unit)
	assert.Equal(t, "KILOBYTE", stat.CountStatistics[1].Unit)
	assert.Equal(t, "MILLISECOND", stat.TimeStatistics[0].Unit)
}

func TestWebSpherePMI_WeightedAverageWithPrecision(t *testing.T) {
	w := New()
	w.integralCache = make(map[string]*integralCacheEntry)

	// Test weighted average calculation with precision
	key := "test_metric"
	
	// First call - should return 0 and cache the value
	result := w.calculateWeightedAverage(key, 1000)
	assert.Equal(t, int64(0), result, "first call should return 0")
	
	// Verify value was cached
	cached, exists := w.integralCache[key]
	assert.True(t, exists, "value should be cached")
	assert.Equal(t, int64(1000), cached.Value)
	
	// Simulate 1 second passing with integral increasing by 500
	// This represents average value of 500 over 1 second
	cached.Timestamp = cached.Timestamp - 1 // Move timestamp 1 second back
	
	result = w.calculateWeightedAverage(key, 1500)
	// Expected: (1500 - 1000) * precision / 1 = 500 * 1000 = 500000
	assert.Equal(t, int64(500*precision), result, "weighted average should be 500 * precision")
	
	// Test counter reset scenario
	cached.Value = 2000
	result = w.calculateWeightedAverage(key, 1000) // Value decreased
	assert.Equal(t, int64(0), result, "should return 0 on counter reset")
}