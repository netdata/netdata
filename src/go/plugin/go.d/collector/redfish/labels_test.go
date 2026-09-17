// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/testutil"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

func TestDecodedCollectorPreservesEndpointJobIdentity(t *testing.T) {
	for name, job := range map[string]string{
		"unicode whitespace": "\u00a0endpoint\u00a0",
		"maximum length":     strings.Repeat("j", 256),
	} {
		t.Run(name, func(t *testing.T) {
			server := testutil.NewServer(t, testutil.ServerConfig{})
			t.Cleanup(server.Close)
			cfg := confgroup.Config{
				"name":        job,
				"module":      "redfish",
				"url":         server.URL,
				"auth_method": "none",
			}
			cfg.ApplyDefaults(confgroup.Default{})
			require.Equal(t, job, cfg.Name())
			creator, ok := collectorapi.DefaultRegistry.Lookup("redfish")
			require.True(t, ok)
			collector := creator.CreateV2()
			raw, err := json.Marshal(cfg)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(raw, collector))
			require.NoError(t, collector.Init(t.Context()))
			t.Cleanup(func() { collector.Cleanup(context.Background()) })
			require.NoError(t, collector.Check(t.Context()))
			sourceTestCollectCycle(t, collector)
			reader := collector.MetricStore().Read(metrix.ReadFlatten())
			for _, metric := range []string{"collection_duration_seconds", "system_health"} {
				seen := false
				reader.ForEachByName(metric, func(labels metrix.LabelView, _ metrix.SampleValue) {
					seen = true
					got, present := labels.Get("endpoint_job")
					require.True(t, present, metric)
					require.Equal(t, job, got, metric)
				})
				require.True(t, seen, metric)
			}
		})
	}
}

func TestDecodedCollectorPreservesServiceName(t *testing.T) {
	const root = "/redfish/v1/"
	docs := map[string]map[string]any{
		root: testutil.Resource(root, "ServiceRoot", "Named BMC service", map[string]any{
			"RedfishVersion": "1.20.0", "Systems": testutil.Link(root + "Systems"),
		}),
		root + "Systems":   testutil.Collection(root+"Systems", "ComputerSystem", root+"Systems/1"),
		root + "Systems/1": testutil.Resource(root+"Systems/1", "ComputerSystem", "System", nil),
	}
	collector := sourceTestDecodedCollector(t, testutil.ServeDocuments(t, docs))
	sourceTestCollectCycle(t, collector)
	count := 0
	collector.MetricStore().
		Read(metrix.ReadFlatten()).
		ForEachByName("service_acquisition_state", func(labels metrix.LabelView, value metrix.SampleValue) {
			count++
			name, ok := labels.Get("resource_name")
			require.True(t, ok, "service metric must retain ServiceRoot.Name")
			require.Equal(t, "Named BMC service", name)
		})
	require.Positive(t, count, "test must observe the service metric")
}
