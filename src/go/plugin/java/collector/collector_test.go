// SPDX-License-Identifier: GPL-3.0-or-later

package collector

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"crypto/sha256"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/java/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	collectorv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestRecordedExportThroughReceiverStoreAndCharts(t *testing.T) {
	c := New(RuntimeConfig{})
	c.ingress.Admit("fixture-instance", "checkout")
	token := strings.Repeat("a", 64)
	c.credentials[sha256.Sum256([]byte(token))] = admission{"fixture-instance", "checkout"}
	data, err := os.ReadFile("../ingest/testdata/java-2.32.0.json")
	require.NoError(t, err)
	var req collectorv1.ExportMetricsServiceRequest
	require.NoError(t, protojson.Unmarshal(data, &req))
	// Bind the captured resource to this test's admitted application.
	for _, r := range req.ResourceMetrics {
		for _, a := range r.GetResource().GetAttributes() {
			if a.Key == "service.name" {
				a.Value = &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "checkout"}}
			}
		}
	}

	var newest uint64
	for _, r := range req.ResourceMetrics {
		for _, scope := range r.ScopeMetrics {
			for _, metric := range scope.Metrics {
				for _, p := range metric.GetGauge().GetDataPoints() {
					newest = max(newest, p.TimeUnixNano)
				}
				for _, p := range metric.GetSum().GetDataPoints() {
					newest = max(newest, p.TimeUnixNano)
				}
				for _, p := range metric.GetHistogram().GetDataPoints() {
					newest = max(newest, p.TimeUnixNano)
				}
			}
		}
	}
	now := time.Unix(0, int64(newest)).Add(time.Second)
	c.now = func() time.Time { return now }
	// The shape fixture combines observations from different exports. Replay them
	// in one source interval to exercise every chart through the freshness filter.
	for _, r := range req.ResourceMetrics {
		for _, scope := range r.ScopeMetrics {
			for _, metric := range scope.Metrics {
				for _, p := range metric.GetGauge().GetDataPoints() {
					p.TimeUnixNano = newest
				}
				for _, p := range metric.GetSum().GetDataPoints() {
					p.TimeUnixNano = newest
				}
				for _, p := range metric.GetHistogram().GetDataPoints() {
					p.TimeUnixNano = newest
				}
			}
		}
	}
	data, err = proto.Marshal(&req)
	require.NoError(t, err)
	handler := c.receiver()
	send := func() *collectorv1.ExportMetricsServiceResponse {
		r := httptest.NewRequest(http.MethodPost, "/v1/metrics", bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/x-protobuf")
		r.Header.Set("x-netdata-java-token", token)
		w := httptest.NewRecorder()
		handler(w, r)
		require.Equal(t, http.StatusOK, w.Code)
		var response collectorv1.ExportMetricsServiceResponse
		require.NoError(t, proto.Unmarshal(w.Body.Bytes(), &response))
		return &response
	}
	assert.Nil(t, send().PartialSuccess)
	assert.EqualValues(t, 14, send().PartialSuccess.RejectedDataPoints, "duplicate exports must not refresh state")
	managed, ok := metrix.AsCycleManagedStore(c.store)
	require.True(t, ok)
	cycle := managed.CycleController()
	engine, err := chartengine.New()
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(charts), 1))
	collect := func() chartengine.Plan {
		cycle.BeginCycle()
		require.NoError(t, c.Collect(context.Background()))
		require.NoError(t, cycle.CommitCycleSuccess())
		plan, err := engine.PreparePlan(c.store.Read(metrix.ReadRaw(), metrix.ReadFlatten()))
		require.NoError(t, err)
		result := plan.Plan()
		require.NoError(t, plan.Commit())
		return result
	}
	contexts := make(map[string]bool)
	created := make(map[string]bool)
	for _, a := range collect().Actions {
		switch a := a.(type) {
		case chartengine.CreateChartAction:
			contexts[a.Meta.Context] = true
			if a.Meta.Context == "java.http_request_duration" {
				assert.Equal(t, "observations/s", a.Meta.Units, "heatmap intensity counts observations; boundaries carry seconds")
			}
			created[a.ChartID] = true
			assert.Equal(t, "checkout", a.Labels["application"])
			assert.Equal(t, "fixture-instance", a.Labels["instance"])
		case chartengine.CreateDimensionAction:
			if a.ChartMeta.Context == "java.http_requests" {
				assert.Equal(t, chartengine.AlgorithmIncremental, a.Algorithm)
			}
		}
	}
	assert.Len(t, contexts, 6)
	// A new cumulative start time must create a fresh Netdata counter baseline.
	reset := proto.Clone(&req).(*collectorv1.ExportMetricsServiceRequest)
	now = now.Add(2 * time.Second)
	for _, r := range reset.ResourceMetrics {
		for _, scope := range r.ScopeMetrics {
			for _, metric := range scope.Metrics {
				for _, p := range metric.GetHistogram().GetDataPoints() {
					p.TimeUnixNano, p.StartTimeUnixNano = uint64(now.UnixNano()), uint64(now.Add(-time.Second).UnixNano())
					p.Count = 0
					zero := 0.0
					p.Sum = &zero
					for i := range p.BucketCounts {
						p.BucketCounts[i] = 0
					}
				}
			}
		}
	}
	result, _ := c.ingress.Ingest(reset, now)
	assert.Equal(t, 2, result.Accepted, "only the two advancing HTTP series are accepted")
	resetCharts := 0
	for _, a := range collect().Actions {
		if a, ok := a.(chartengine.CreateChartAction); ok {
			assert.Contains(t, a.Meta.Context, "java.http_")
			assert.False(t, created[a.ChartID], "reset must not reuse the old counter chart")
			created[a.ChartID] = true
			resetCharts++
		}
	}
	assert.Equal(t, 4, resetCharts)
	// Source freshness controls publication even while the job remains healthy.
	now = now.Add(10 * time.Second)
	removed := make(map[string]bool)
	for range 12 {
		for _, a := range collect().Actions {
			if a, ok := a.(chartengine.RemoveChartAction); ok {
				removed[a.ChartID] = true
			}
		}
	}
	assert.Equal(t, created, removed)
}

func TestApplicationCoverageAndStaleness(t *testing.T) {
	c := New(RuntimeConfig{})
	target := protocol.Process{PID: 42, StartTime: 9876, BootID: "fixture", Application: "checkout", Location: "Host"}
	c.targets[target.Instance()] = targetState{Process: target, Status: "Attached", LastSeen: time.Unix(100, 0), Runtime: "21"}
	c.ApplicationNames = map[string]string{"checkout": "Checkout API"}
	rows := c.Applications()
	require.Len(t, rows, 1)
	assert.Equal(t, "No fresh data", rows[0].Status)
	assert.Equal(t, "Checkout API", rows[0].Name)
	assert.Equal(t, target.Instance(), rows[0].Instance)
	assert.Equal(t, "Not observed", rows[0].HTTP)
	var schema map[string]any
	require.NoError(t, json.Unmarshal([]byte(configSchema), &schema))
}

func TestCollectCanceled(t *testing.T) {
	c := New(RuntimeConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, c.Collect(ctx), context.Canceled)
}
