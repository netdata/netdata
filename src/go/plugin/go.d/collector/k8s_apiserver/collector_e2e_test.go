// SPDX-License-Identifier: GPL-3.0-or-later

//go:build e2e

package k8s_apiserver

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCollector_E2E_KindCluster tests the collector against a real kind cluster.
// Run with: go test -tags=e2e -v ./collector/k8s_apiserver/...
// Requires: kubectl proxy running on port 8001
func TestCollector_E2E_KindCluster(t *testing.T) {
	// Skip if not in e2e mode or no kubectl proxy
	if os.Getenv("K8S_APISERVER_E2E") == "" {
		t.Skip("Skipping e2e test. Set K8S_APISERVER_E2E=1 and run kubectl proxy --port=8001")
	}

	collr := New()
	collr.URL = "http://127.0.0.1:8001/metrics"
	collr.TLSConfig.TLSCA = "" // Disable TLS for kubectl proxy

	require.NoError(t, collr.Init(context.TODO()))
	require.NoError(t, collr.Check(context.TODO()))

	mx := collr.Collect(context.TODO())
	require.NotNil(t, mx)

	t.Logf("Collected %d metrics", len(mx))

	// Verify key metrics are present
	expectedMetrics := []string{
		"request_total",
		"inflight_mutating",
		"inflight_readonly",
		"process_goroutines",
		"process_threads",
		"process_resident_memory_bytes",
		"audit_events_total",
	}

	for _, m := range expectedMetrics {
		_, ok := mx[m]
		assert.True(t, ok, "Expected metric %s not found", m)
	}

	// Print some interesting metrics
	t.Logf("Sample metrics:")
	sampleKeys := []string{
		"request_total",
		"inflight_mutating",
		"inflight_readonly",
		"process_goroutines",
		"process_resident_memory_bytes",
	}
	for _, k := range sampleKeys {
		if v, ok := mx[k]; ok {
			t.Logf("  %s = %d", k, v)
		}
	}

	// Verify we have dynamic dimensions
	verbCount := 0
	codeCount := 0
	for k := range mx {
		if len(k) > 15 && k[:15] == "request_by_verb" {
			verbCount++
		}
		if len(k) > 15 && k[:15] == "request_by_code" {
			codeCount++
		}
	}
	t.Logf("Dynamic dimensions: %d verbs, %d codes", verbCount, codeCount)
	assert.Greater(t, verbCount, 0, "Expected some verb dimensions")
	assert.Greater(t, codeCount, 0, "Expected some code dimensions")

	collr.Cleanup(context.TODO())
}
