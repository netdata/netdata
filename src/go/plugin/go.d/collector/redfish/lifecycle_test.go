// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestCollectorDecodedJobInitializes(t *testing.T) {
	creator, ok := collectorapi.DefaultRegistry.Lookup("redfish")
	require.True(t, ok)
	require.Nil(t, creator.AgentFunctions)
	require.Nil(t, creator.MethodHandler)
	server := newRedfishTestServer(t, redfishTestServerConfig{})
	defer server.Close()

	for _, format := range []string{"yaml", "json"} {
		t.Run(format, func(t *testing.T) {
			collector := creator.CreateV2()
			config := map[string]any{"name": "endpoint-a", "url": server.URL, "node_mode": "local", "auth_method": "none"}
			var payload []byte
			var err error
			if format == "json" {
				payload, err = json.Marshal(config)
				require.NoError(t, err)
				require.NoError(t, json.Unmarshal(payload, collector))
			} else {
				payload, err = yaml.Marshal(config)
				require.NoError(t, err)
				require.NoError(t, yaml.Unmarshal(payload, collector))
			}
			t.Cleanup(func() { collector.Cleanup(context.Background()) })
			require.NoError(t, collector.Init(context.Background()))
			require.NoError(t, collector.Check(context.Background()))
			managed, ok := metrix.AsCycleManagedStore(collector.MetricStore())
			require.True(t, ok)
			cycle := managed.CycleController()
			cycle.BeginCycle()
			require.NoError(t, collector.Collect(context.Background()))
			require.NoError(t, cycle.CommitCycleSuccess())
			_, origin, err := normalizeServiceRoot(server.URL)
			require.NoError(t, err)
			labels := metrix.Labels{"endpoint_key": stableKey("netdata:redfish:endpoint:v1", origin, endpointKeyHexChars), "endpoint_job": "endpoint-a"}
			point, ok := collector.MetricStore().Read().StateSet("collection_status", labels)
			require.True(t, ok)
			assert.True(t, point.States["success"])
			hardwareSeries := 0
			collector.MetricStore().Read(metrix.ReadFlatten()).ForEachByName("system_health", func(labels metrix.LabelView, value metrix.SampleValue) {
				hardwareSeries++
				job, found := labels.Get("endpoint_job")
				require.True(t, found)
				assert.Equal(t, "endpoint-a", job)
			})
			assert.Positive(t, hardwareSeries)
			collecttest.AssertChartCoverage(t, collector, collecttest.ChartCoverageExpectation{})
			collector.Cleanup(context.Background())
		})
	}
}

func TestCollectorJobsOwnIndependentClients(t *testing.T) {
	server := newRedfishTestServer(t, redfishTestServerConfig{})
	defer server.Close()
	creator := collectorapi.DefaultRegistry["redfish"]
	jobs := make([]collectorapi.CollectorV2, 2)
	for i := range jobs {
		jobs[i] = creator.CreateV2()
		require.NoError(t, yaml.Unmarshal([]byte(fmt.Sprintf("name: endpoint-%d\nurl: %s\nnode_mode: local\nauth_method: none\n", i, server.URL)), jobs[i]))
		t.Cleanup(func() { jobs[i].Cleanup(context.Background()) })
		require.NoError(t, jobs[i].Init(context.Background()))
		require.NoError(t, jobs[i].Check(context.Background()))
	}
	jobs[0].Cleanup(context.Background())
	require.NoError(t, jobs[1].Check(context.Background()))
	managed, ok := metrix.AsCycleManagedStore(jobs[1].MetricStore())
	require.True(t, ok)
	managed.CycleController().BeginCycle()
	require.NoError(t, jobs[1].Collect(context.Background()))
	require.NoError(t, managed.CycleController().CommitCycleSuccess())
}

func TestCollectorCleanupAfterFailedInitialization(t *testing.T) {
	collector := New()
	require.NoError(t, yaml.Unmarshal([]byte("name: endpoint-a\nurl: https://bmc.example.test\nnode_mode: local\nauth_method: none\n"), collector))
	collector.newClient = func(Config, *http.Client) (endpointClient, error) {
		return nil, fmt.Errorf("test client construction failure")
	}
	require.ErrorContains(t, collector.Init(context.Background()), "test client construction failure")
	collector.Cleanup(context.Background())
	collector.Cleanup(context.Background())
	assert.Nil(t, collector.httpClient)
	assert.Nil(t, collector.client)
}
