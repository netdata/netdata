// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_SGLangRollupCardinality(t *testing.T) {
	profile, err := os.ReadFile("../../config/go.d/prometheus.profiles/default/sglang.yaml")
	require.NoError(t, err)
	for _, limits := range []struct {
		name          string
		total, metric int
	}{
		{"context", 500, 10000},
		{"metric_in_context", 10000, 500},
	} {
		t.Run(limits.name, func(t *testing.T) {
			var population atomic.Int64
			population.Store(600)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				var body strings.Builder
				for _, metric := range []string{"num_requests_total", "prompt_tokens_total", "generation_tokens_total", "http_requests_total", "http_responses_total"} {
					fmt.Fprintf(&body, "# TYPE sglang:%s counter\n", metric)
				}
				body.WriteString("# TYPE sglang:http_requests_active gauge\n")
				for i := range int(population.Load()) {
					count := i%5 + 1
					labels := fmt.Sprintf(`model_name="model_a",engine_type="unified",priority="%d",is_streaming="%t",custom_tag="tag_%d"`, i, i%2 == 0, i)
					fmt.Fprintf(&body, "sglang:num_requests_total{%s} %d\n", labels, count)
					fmt.Fprintf(&body, "sglang:prompt_tokens_total{%s} %d\n", labels, count*32)
					fmt.Fprintf(&body, "sglang:generation_tokens_total{%s} %d\n", labels, count*8)
					fmt.Fprintf(&body, "sglang:http_requests_total{endpoint=\"/unmatched/%d\",method=\"GET\"} %d\n", i, count)
					fmt.Fprintf(&body, "sglang:http_responses_total{endpoint=\"/unmatched/%d\",method=\"GET\",status_code=\"404\"} %d\n", i, count)
					fmt.Fprintf(&body, "sglang:http_requests_active{endpoint=\"/unmatched/%d\",method=\"GET\"} 0\n", i)
				}
				_, _ = w.Write([]byte(body.String()))
			}))
			defer server.Close()
			collector := NewWithOptions(WithProfileCatalog(loadTestCatalog(t, map[string]string{"sglang": string(profile)})))
			collector.URL, collector.MaxTS, collector.MaxTSPerMetric = server.URL, limits.total, limits.metric
			require.NoError(t, collector.Init(context.Background()))
			t.Cleanup(func() { collector.Cleanup(context.Background()) })
			require.NoError(t, collector.Check(context.Background()))
			engine, err := chartengine.New(chartengine.WithEnginePolicy(collector.EnginePolicy()))
			require.NoError(t, err)
			require.NoError(t, engine.LoadYAML([]byte(collector.ChartTemplateYAML()), 1))
			contexts := make(map[string]string)
			for _, size := range []int64{600, 3, 600, 3} {
				population.Store(size)
				collectProfileRelabelOnce(t, collector)
				values := commitCardinalityPlan(t, collector, engine, contexts)
				finished, tokens, httpTotal := []float64{900, 900}, []float64{57600, 14400}, float64(1800)
				if size == 3 {
					finished, tokens, httpTotal = []float64{4, 2}, []float64{192, 48}, 6
				}
				for _, context := range []string{"service.finished_requests", "models.finished_requests", "models.by_engine_finished_requests"} {
					assert.ElementsMatch(t, finished, values["prometheus.sglang."+context], context)
				}
				for _, context := range []string{"service.tokens", "models.tokens", "models.by_engine_tokens"} {
					assert.ElementsMatch(t, tokens, values["prometheus.sglang."+context], context)
				}
				assert.Equal(t, []float64{httpTotal}, values["prometheus.sglang.service.http_requests_total"])
				assert.Equal(t, []float64{httpTotal}, values["prometheus.sglang.service.http_responses_total"])
				assert.Equal(t, []float64{0}, values["prometheus.sglang.service.http_requests_active"])
				if size == 600 {
					assert.NotContains(t, values, "prometheus.sglang.models.by_priority_tokens")
				} else {
					assert.Len(t, values["prometheus.sglang.models.by_priority_tokens"], 6)
				}
				for _, context := range []string{"models.by_priority_finished_requests", "http_routes.route_http_requests_total", "http_routes.method_http_responses_total"} {
					if size == 600 {
						assert.NotContains(t, values, "prometheus.sglang."+context)
					} else {
						assert.Len(t, values["prometheus.sglang."+context], 3)
					}
				}
			}
		})
	}
}
