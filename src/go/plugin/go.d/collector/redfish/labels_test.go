// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/testutil"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/stretchr/testify/require"
)

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
