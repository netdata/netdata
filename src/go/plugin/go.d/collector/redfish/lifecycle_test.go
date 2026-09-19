// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/acquisition"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/testutil"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/identity"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestCollectorDecodedJobInitializes(t *testing.T) {
	creator, ok := collectorapi.DefaultRegistry.Lookup("redfish")
	require.True(t, ok)
	require.Nil(t, creator.AgentFunctions)
	require.NotNil(t, creator.MethodHandler)
	require.NotNil(t, creator.SharedFunctions)
	methods := creator.SharedFunctions()
	require.Len(t, methods, 3)
	assert.Equal(t, "logs", methods[2].ID)
	assert.Equal(t, "sensors", methods[0].ID)
	assert.Equal(t, "hardware", methods[1].ID)
	server := testutil.NewServer(t, testutil.ServerConfig{})
	defer server.Close()

	for _, format := range []string{"yaml", "json"} {
		t.Run(format, func(t *testing.T) {
			collector := creator.CreateV2()
			config := map[string]any{"name": "endpoint-a", "url": server.URL, "auth_method": "none"}
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
			_, origin, err := acquisition.NormalizeServiceRoot(server.URL)
			require.NoError(t, err)
			labels := metrix.Labels{
				"endpoint_key": identity.Key("netdata:redfish:endpoint:v1", origin, identity.EndpointKeyHexChars),
			}
			point, ok := collector.MetricStore().Read().StateSet("collection_status", labels)
			require.True(t, ok)
			assert.True(t, point.States["success"])
			hardwareSeries := 0
			collector.MetricStore().
				Read(metrix.ReadFlatten()).
				ForEachByName("system_health_status", func(metrix.LabelView, metrix.SampleValue) {
					hardwareSeries++
				})
			assert.Positive(t, hardwareSeries)
			collecttest.AssertChartCoverage(t, collector, collecttest.ChartCoverageExpectation{})
			collector.Cleanup(context.Background())
		})
	}
}

func TestCollectorJobsOwnIndependentClients(t *testing.T) {
	server := testutil.NewServer(t, testutil.ServerConfig{})
	defer server.Close()
	creator := collectorapi.DefaultRegistry["redfish"]
	jobs := make([]collectorapi.CollectorV2, 2)
	for i := range jobs {
		jobs[i] = creator.CreateV2()
		require.NoError(
			t,
			yaml.Unmarshal(
				[]byte(fmt.Sprintf("name: endpoint-%d\nurl: %s\nauth_method: none\n", i, server.URL)),
				jobs[i],
			),
		)
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
	require.NoError(
		t,
		yaml.Unmarshal([]byte("name: endpoint-a\nurl: https://bmc.example.test\nauth_method: none\n"), collector),
	)
	collector.newClient = func(acquisition.Options, *http.Client) (endpointClient, error) {
		return nil, fmt.Errorf("test client construction failure")
	}
	require.ErrorContains(t, collector.Init(context.Background()), "test client construction failure")
	collector.Cleanup(context.Background())
	collector.Cleanup(context.Background())
	assert.Nil(t, collector.httpClient)
	assert.Nil(t, collector.client)
}

func TestCollectorSessionStartsDuringRunningCollection(t *testing.T) {
	server := testutil.NewServer(t, testutil.ServerConfig{
		SupportSession: true,
	})
	defer server.Close()
	collector := New()
	collector.Config = testConfig(server.URL, "session")
	defer collector.Cleanup(context.Background())
	require.NoError(t, collector.Init(context.Background()))
	require.NoError(t, collector.Check(context.Background()))
	assert.Zero(t, server.SessionCreates.Load())
	assert.Zero(t, server.SessionDeletes.Load())
	managed, ok := metrix.AsCycleManagedStore(collector.MetricStore())
	require.True(t, ok)
	cycle := managed.CycleController()
	cycle.BeginCycle()
	require.NoError(t, collector.Collect(context.Background()))
	require.NoError(t, cycle.CommitCycleSuccess())
	assert.Equal(t, int64(1), server.SessionCreates.Load())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	collector.Cleanup(ctx)
	collector.Cleanup(ctx)
	assert.Equal(t, int64(1), server.SessionDeletes.Load())
}

func TestDecodedCollectorSessionRecovery(t *testing.T) {
	var expire atomic.Bool
	server := testutil.NewServer(t, testutil.ServerConfig{
		SupportSession:    true,
		ExpireSessionOnce: &expire,
	})
	defer server.Close()
	collector := collectorapi.DefaultRegistry["redfish"].CreateV2()
	require.NoError(
		t,
		json.Unmarshal(
			[]byte(
				fmt.Sprintf(
					`{"name":"session","url":%q,"auth_method":"session","username":"user","password":"test-password"}`,
					server.URL,
				),
			),
			collector,
		),
	)
	require.NoError(t, collector.Init(t.Context()))
	defer collector.Cleanup(context.Background())
	require.NoError(t, collector.Check(t.Context()))
	assert.Zero(t, server.SessionCreates.Load())
	managed, ok := metrix.AsCycleManagedStore(collector.MetricStore())
	require.True(t, ok)
	_, origin, err := acquisition.NormalizeServiceRoot(server.URL)
	require.NoError(t, err)
	labels := metrix.Labels{
		"endpoint_key": identity.Key("netdata:redfish:endpoint:v1", origin, identity.EndpointKeyHexChars),
	}
	for cycle := range 3 {
		managed.CycleController().BeginCycle()
		require.NoError(t, collector.Collect(t.Context()))
		require.NoError(t, managed.CycleController().CommitCycleSuccess())
		point, ok := collector.MetricStore().Read().StateSet("collection_status", labels)
		require.True(t, ok)
		assert.True(t, point.States["success"])
		var hardwareSeries int
		collector.MetricStore().
			Read(metrix.ReadFlatten()).
			ForEachByName("system_health_status", func(metrix.LabelView, metrix.SampleValue) {
				hardwareSeries++
			})
		assert.Positive(t, hardwareSeries, "session recovery must not leave a hardware gap")
		if cycle == 0 {
			expire.Store(true)
		}
	}
	assert.Equal(t, int64(2), server.SessionCreates.Load())
	collector.Cleanup(t.Context())
	assert.Zero(t, server.ActiveSessions.Load())
}
