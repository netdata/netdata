// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo && integration
// +build cgo,integration

package websphere_jmx

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebSphereJMX_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// This test requires a running WebSphere instance with JMX enabled
	// Set environment variable WEBSPHERE_JMX_URL to run integration tests
	// Example: WEBSPHERE_JMX_URL=service:jmx:rmi:///jndi/rmi://localhost:9999/jmxrmi
	jmxURL := getEnvOrSkip(t, "WEBSPHERE_JMX_URL")

	ws := New()
	ws.Config.JMXURL = jmxURL
	ws.Config.JMXTimeout = confopt.Duration(10 * time.Second)
	ws.Config.InitTimeout = confopt.Duration(30 * time.Second)

	ctx := context.Background()
	require.NoError(t, ws.Init(ctx))
	defer ws.Cleanup(ctx)

	require.NoError(t, ws.Check(ctx))
	
	mx := ws.Collect(ctx)
	require.NotNil(t, mx)
	assert.Greater(t, len(mx), 0, "should collect at least some metrics")

	// Verify JVM metrics are present
	assert.Contains(t, mx, "jvm_heap_used", "should collect JVM heap metrics")
	assert.Contains(t, mx, "jvm_thread_count", "should collect JVM thread metrics")
	
	// Verify heap usage percentage is calculated
	if mx["jvm_heap_max"] > 0 {
		assert.Contains(t, mx, "jvm_heap_usage_percent", "should calculate heap usage percentage")
	}
}

func TestWebSphereJMX_Integration_WithAuth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	jmxURL := getEnvOrSkip(t, "WEBSPHERE_JMX_URL_AUTH")
	username := getEnvOrSkip(t, "WEBSPHERE_JMX_USERNAME")
	password := getEnvOrSkip(t, "WEBSPHERE_JMX_PASSWORD")

	ws := New()
	ws.Config.JMXURL = jmxURL
	ws.Config.JMXUsername = username
	ws.Config.JMXPassword = password
	ws.Config.JMXTimeout = confopt.Duration(10 * time.Second)

	ctx := context.Background()
	require.NoError(t, ws.Init(ctx))
	defer ws.Cleanup(ctx)

	require.NoError(t, ws.Check(ctx))
	
	mx := ws.Collect(ctx)
	require.NotNil(t, mx)
	assert.Greater(t, len(mx), 0)
}

func TestWebSphereJMX_Integration_CardinalityLimits(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	jmxURL := getEnvOrSkip(t, "WEBSPHERE_JMX_URL")

	ws := New()
	ws.Config.JMXURL = jmxURL
	
	// Set very low limits to test cardinality control
	ws.Config.MaxThreadPools = 1
	ws.Config.MaxJDBCPools = 1
	ws.Config.MaxApplications = 1
	ws.Config.MaxJMSDestinations = 1

	ctx := context.Background()
	require.NoError(t, ws.Init(ctx))
	defer ws.Cleanup(ctx)

	require.NoError(t, ws.Check(ctx))
	
	mx := ws.Collect(ctx)
	require.NotNil(t, mx)
	
	// Count dynamic instances to verify limits are respected
	threadPools := 0
	jdbcPools := 0
	applications := 0
	jmsDestinations := 0
	
	for key := range mx {
		switch {
		case strings.HasPrefix(key, "threadpool_"):
			if strings.Contains(key, "_size") {
				threadPools++
			}
		case strings.HasPrefix(key, "jdbc_"):
			if strings.Contains(key, "_size") {
				jdbcPools++
			}
		case strings.HasPrefix(key, "app_"):
			if strings.Contains(key, "_requests") {
				applications++
			}
		case strings.HasPrefix(key, "jms_"):
			if strings.Contains(key, "_messages_current") {
				jmsDestinations++
			}
		}
	}
	
	assert.LessOrEqual(t, threadPools, 1, "should respect thread pool limit")
	assert.LessOrEqual(t, jdbcPools, 1, "should respect JDBC pool limit")
	assert.LessOrEqual(t, applications, 1, "should respect application limit")
	assert.LessOrEqual(t, jmsDestinations, 1, "should respect JMS destination limit")
}

func TestWebSphereJMX_Integration_SelectiveCollection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	jmxURL := getEnvOrSkip(t, "WEBSPHERE_JMX_URL")

	ws := New()
	ws.Config.JMXURL = jmxURL
	
	// Disable most collection to test selective monitoring
	ws.Config.CollectJVMMetrics = true
	ws.Config.CollectThreadPoolMetrics = false
	ws.Config.CollectJDBCMetrics = false
	ws.Config.CollectJMSMetrics = false
	ws.Config.CollectWebAppMetrics = false
	ws.Config.CollectSessionMetrics = false
	ws.Config.CollectTransactionMetrics = false

	ctx := context.Background()
	require.NoError(t, ws.Init(ctx))
	defer ws.Cleanup(ctx)

	require.NoError(t, ws.Check(ctx))
	
	mx := ws.Collect(ctx)
	require.NotNil(t, mx)
	
	// Should have JVM metrics
	assert.Contains(t, mx, "jvm_heap_used")
	
	// Should not have other metrics
	hasThreadPool := false
	hasJDBC := false
	hasApp := false
	
	for key := range mx {
		if strings.HasPrefix(key, "threadpool_") {
			hasThreadPool = true
		}
		if strings.HasPrefix(key, "jdbc_") {
			hasJDBC = true
		}
		if strings.HasPrefix(key, "app_") {
			hasApp = true
		}
	}
	
	assert.False(t, hasThreadPool, "should not collect thread pool metrics when disabled")
	assert.False(t, hasJDBC, "should not collect JDBC metrics when disabled")
	assert.False(t, hasApp, "should not collect application metrics when disabled")
}

func getEnvOrSkip(t *testing.T, envVar string) string {
	value := os.Getenv(envVar)
	if value == "" {
		t.Skipf("skipping test: %s environment variable not set", envVar)
	}
	return value
}