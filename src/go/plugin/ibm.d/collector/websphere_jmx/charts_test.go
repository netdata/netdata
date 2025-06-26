// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package websphere_jmx

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/stretchr/testify/assert"
)

func TestInitCharts(t *testing.T) {
	ws := &WebSphereJMX{}
	ws.initCharts()
	
	assert.NotNil(t, ws.charts)
	assert.Greater(t, len(*ws.charts), 0)
	
	// Check that base charts are present
	foundHeap := false
	foundGC := false
	for _, chart := range *ws.charts {
		if chart.ID == "jvm_heap_memory" {
			foundHeap = true
		}
		if chart.ID == "jvm_gc_count" {
			foundGC = true
		}
	}
	assert.True(t, foundHeap, "JVM heap chart not found")
	assert.True(t, foundGC, "JVM GC chart not found")
}

func TestNewThreadPoolCharts(t *testing.T) {
	poolName := "Default ThreadPool"
	charts := newThreadPoolCharts(poolName)
	
	assert.NotNil(t, charts)
	assert.Greater(t, len(*charts), 0)
	
	// Check chart properties
	for _, chart := range *charts {
		assert.Contains(t, chart.ID, "threadpool_default_threadpool_")
		assert.Equal(t, 1, len(chart.Labels))
		assert.Equal(t, "thread_pool", chart.Labels[0].Key)
		assert.Equal(t, poolName, chart.Labels[0].Value)
	}
}

func TestNewJDBCPoolCharts(t *testing.T) {
	poolName := "jdbc/OracleDS"
	charts := newJDBCPoolCharts(poolName)
	
	assert.NotNil(t, charts)
	assert.Greater(t, len(*charts), 0)
	
	// Check chart properties
	for _, chart := range *charts {
		assert.Contains(t, chart.ID, "jdbc_jdbc_oracleds_")
		assert.Equal(t, 1, len(chart.Labels))
		assert.Equal(t, "jdbc_pool", chart.Labels[0].Key)
		assert.Equal(t, poolName, chart.Labels[0].Value)
	}
}

func TestNewJMSDestinationCharts(t *testing.T) {
	destName := "Queue.Orders"
	destType := "queue"
	charts := newJMSDestinationCharts(destName, destType)
	
	assert.NotNil(t, charts)
	assert.Greater(t, len(*charts), 0)
	
	// Check chart properties
	for _, chart := range *charts {
		assert.Contains(t, chart.ID, "jms_queue_orders_")
		assert.Equal(t, 2, len(chart.Labels))
		
		// Check labels
		labelMap := make(map[string]string)
		for _, label := range chart.Labels {
			labelMap[label.Key] = label.Value
		}
		assert.Equal(t, destName, labelMap["jms_destination"])
		assert.Equal(t, destType, labelMap["jms_type"])
	}
}

func TestNewApplicationCharts(t *testing.T) {
	appName := "MyWebApp"
	
	tests := map[string]struct {
		includeSessions     bool
		includeTransactions bool
		expectedChartCount  int
	}{
		"basic only": {
			includeSessions:     false,
			includeTransactions: false,
			expectedChartCount:  3, // requests, errors, response_time
		},
		"with sessions": {
			includeSessions:     true,
			includeTransactions: false,
			expectedChartCount:  5, // + sessions, sessions_invalidated
		},
		"with transactions": {
			includeSessions:     false,
			includeTransactions: true,
			expectedChartCount:  5, // + transactions_active, transactions_total
		},
		"all enabled": {
			includeSessions:     true,
			includeTransactions: true,
			expectedChartCount:  7, // all charts
		},
	}
	
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			charts := newApplicationCharts(appName, test.includeSessions, test.includeTransactions)
			
			assert.NotNil(t, charts)
			assert.Equal(t, test.expectedChartCount, len(*charts))
			
			// Check that all charts have proper labels
			for _, chart := range *charts {
				assert.Contains(t, chart.ID, "app_mywebapp_")
				assert.Equal(t, 1, len(chart.Labels))
				assert.Equal(t, "application", chart.Labels[0].Key)
				assert.Equal(t, appName, chart.Labels[0].Value)
			}
		})
	}
}

func TestChartPriorities(t *testing.T) {
	ws := &WebSphereJMX{}
	ws.initCharts()
	
	// Verify that charts have different priorities
	priorities := make(map[int]bool)
	for _, chart := range *ws.charts {
		if priorities[chart.Priority] {
			t.Errorf("Duplicate priority %d found", chart.Priority)
		}
		priorities[chart.Priority] = true
	}
}

func TestChartDimensions(t *testing.T) {
	// Test that dimension IDs match what collect.go expects
	charts := baseCharts.Copy()
	
	// Find JVM heap chart
	var heapChart *module.Chart
	for _, chart := range *charts {
		if chart.ID == "jvm_heap_memory" {
			heapChart = chart
			break
		}
	}
	
	assert.NotNil(t, heapChart)
	assert.Equal(t, 3, len(heapChart.Dims))
	
	// Check dimension IDs
	dimIDs := make(map[string]bool)
	for _, dim := range heapChart.Dims {
		dimIDs[dim.ID] = true
	}
	
	assert.True(t, dimIDs["jvm_heap_used"])
	assert.True(t, dimIDs["jvm_heap_committed"])
	assert.True(t, dimIDs["jvm_heap_max"])
}