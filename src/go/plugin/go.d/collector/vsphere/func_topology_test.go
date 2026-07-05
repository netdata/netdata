// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
)

func TestFuncTopology_Handle(t *testing.T) {
	tests := map[string]struct {
		method    string
		collector func() *Collector
		want      int
		check     func(*testing.T, any)
	}{
		"without discovery": {
			method:    "topology:vsphere",
			collector: New,
			want:      503,
		},
		"with empty inventory cache": {
			method: "topology:vsphere",
			collector: func() *Collector {
				collr := New()
				collr.resources = &rs.Resources{}
				return collr
			},
			want: 200,
			check: func(t *testing.T, raw any) {
				data, ok := raw.(topologyv1.Data)
				require.True(t, ok)
				validateVSphereTopologyV1Data(t, data)
				require.Equal(t, 0, data.Actors.Rows)
				require.Equal(t, 0, data.Links.Rows)
				require.Nil(t, data.Overlays)
				require.EqualValues(t, 0, data.Stats["hosts"])
				require.EqualValues(t, 0, data.Stats["vms"])
				require.EqualValues(t, 0, data.Stats["overlay_refs"])
			},
		},
		"unknown method": {
			method:    "unknown",
			collector: New,
			want:      404,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			handler := &funcTopology{collector: tc.collector(), agentID: "vsphere_vcenter1", jobName: "vsphere_vcenter1"}

			resp := handler.Handle(context.Background(), tc.method, nil)

			require.Equal(t, tc.want, resp.Status)
			if tc.check != nil {
				tc.check(t, resp.Data)
			}
		})
	}
}

func newVSphereTopologyTestCollector() *Collector {
	collr := New()
	collr.resources = &rs.Resources{
		DataCenters: rs.DataCenters{
			"datacenter-1": {ID: "datacenter-1", Name: "DC1"},
		},
		Clusters: rs.Clusters{
			"domain-c1": {
				ID:            "domain-c1",
				Name:          "Cluster1",
				Hier:          rs.ClusterHierarchy{DC: rs.HierarchyValue{ID: "datacenter-1", Name: "DC1"}},
				OverallStatus: "green",
				DrsEnabled:    true,
				HaEnabled:     true,
			},
		},
		Hosts: rs.Hosts{
			"host-1": {
				ID:              "host-1",
				Name:            "Host1",
				Hier:            rs.HostHierarchy{DC: rs.HierarchyValue{ID: "datacenter-1", Name: "DC1"}, Cluster: rs.HierarchyValue{ID: "domain-c1", Name: "Cluster1"}},
				ConnectionState: "connected",
				PowerState:      "poweredOn",
				OverallStatus:   "green",
			},
		},
		VMs: rs.VMs{
			"vm-1": {
				ID:                    "vm-1",
				Name:                  "VM1",
				Hier:                  rs.VMHierarchy{DC: rs.HierarchyValue{ID: "datacenter-1", Name: "DC1"}, Cluster: rs.HierarchyValue{ID: "domain-c1", Name: "Cluster1"}, Host: rs.HierarchyValue{ID: "host-1", Name: "Host1"}},
				ConnectionState:       "connected",
				PowerState:            "poweredOn",
				OverallStatus:         "green",
				SnapshotCount:         2,
				SnapshotMaxChainDepth: 3,
				Labels:                map[string]string{"vsphere_tag_service": "payments"},
			},
		},
		Datastores: rs.Datastores{
			"datastore-1": {
				ID:              "datastore-1",
				Name:            "Datastore1",
				Hier:            rs.DatastoreHierarchy{DC: rs.HierarchyValue{ID: "datacenter-1", Name: "DC1"}},
				Type:            "VMFS",
				Accessible:      true,
				MaintenanceMode: "normal",
				Capacity:        1000,
				FreeSpace:       400,
			},
		},
		Networks: rs.Networks{
			"network-1": {
				ID:            "network-1",
				Name:          "VM Network",
				Type:          "Network",
				Hier:          rs.NetworkHierarchy{DC: rs.HierarchyValue{ID: "datacenter-1", Name: "DC1"}},
				Accessible:    true,
				HostIDs:       []string{"host-1"},
				VMIDs:         []string{"vm-1"},
				OverallStatus: "green",
			},
		},
		StoragePods: rs.StoragePods{
			"group-p1": {
				ID:                "group-p1",
				Name:              "Pod1",
				Hier:              rs.StoragePodHierarchy{DC: rs.HierarchyValue{ID: "datacenter-1", Name: "DC1"}},
				StorageDRSEnabled: new(true),
			},
		},
		ResourcePools: rs.ResourcePools{
			"resgroup-1": {
				ID:            "resgroup-1",
				Name:          "Resources",
				Hier:          rs.ResourcePoolHierarchy{DC: rs.HierarchyValue{ID: "datacenter-1", Name: "DC1"}, Cluster: rs.HierarchyValue{ID: "domain-c1", Name: "Cluster1"}},
				OverallStatus: "green",
			},
		},
	}
	return collr
}

func TestFuncTopology_HandleWithInventoryCache(t *testing.T) {
	collr := newVSphereTopologyTestCollector()
	handler := &funcTopology{collector: collr, agentID: "vsphere_vcenter1", jobName: "vsphere_vcenter1"}

	resp := handler.Handle(context.Background(), "topology:vsphere", nil)

	require.Equal(t, 200, resp.Status)
	require.Equal(t, "topology", resp.ResponseType)
	data, ok := resp.Data.(topologyv1.Data)
	require.True(t, ok)
	validateVSphereTopologyV1Data(t, data)
	require.Equal(t, topologyv1.SchemaVersion, data.SchemaVersion)
	require.Equal(t, "vsphere", data.Producer.Source)
	require.Equal(t, "go.d/vsphere", data.Producer.Plugin)
	require.Equal(t, "vsphere_vcenter1", data.Producer.Instance)
	require.Equal(t, "virtualization", data.Types.ActorTypes["vsphere_vm"].Layer)
	require.Equal(t, 8, data.Actors.Rows)
	require.Equal(t, 9, data.Links.Rows)
	require.EqualValues(t, 1, data.Stats["overlay_refs"])

	actors := topologyTableRows(t, data.Actors, data.Dictionaries)
	requireTopologyRow(t, actors, "vsphere_moid", "datacenter-1")
	requireTopologyRow(t, actors, "vsphere_moid", "domain-c1")
	requireTopologyRow(t, actors, "vsphere_moid", "host-1")
	vm := requireTopologyRow(t, actors, "vsphere_moid", "vm-1")
	datastore := requireTopologyRow(t, actors, "vsphere_moid", "datastore-1")
	network := requireTopologyRow(t, actors, "vsphere_moid", "network-1")
	require.Equal(t, "vsphere_vm", vm["type"])
	require.Equal(t, "vm", vm["object_type"])
	require.Equal(t, "VM1", vm["name"])
	require.Equal(t, "network", network["object_type"])
	require.Equal(t, "VM Network", network["name"])
	require.Subset(t, data.Types.ActorTypes["vsphere_vm"].Search.Columns, []string{"name", "vsphere_moid", "object_type"})
	require.Contains(t, data.Types.ActorTypes["vsphere_vm"].Search.LabelKeys, "vsphere_tag_service")
	require.NotContains(t, data.Types.ActorTypes["vsphere_host"].Search.LabelKeys, "vsphere_tag_service")

	require.NotNil(t, data.Tables)
	details := topologyTableRows(t, data.Tables.Actor[vsphereTopologyDetailTable].Table, data.Dictionaries)
	vmDetail := requireTopologyRow(t, details, "vsphere_moid", "vm-1")
	require.EqualValues(t, 2, vmDetail["snapshot_count"])
	require.EqualValues(t, 3, vmDetail["snapshot_chain_depth"])

	links := topologyTableRows(t, data.Links, data.Dictionaries)
	keys := topologyLinkKeys(t, actors, links)
	require.Contains(t, keys, "datacenter:datacenter-1->cluster:domain-c1:vsphere_ownership")
	require.Contains(t, keys, "cluster:domain-c1->host:host-1:vsphere_ownership")
	require.Contains(t, keys, "vm:vm-1->host:host-1:vsphere_runs_on")
	require.Contains(t, keys, "host:host-1->network:network-1:vsphere_connected_to")
	require.Contains(t, keys, "vm:vm-1->network:network-1:vsphere_connected_to")

	evidence := topologyTableRows(t, data.Evidence[vsphereRunsOnEvidenceType].Table, data.Dictionaries)
	runEvidence := requireTopologyRow(t, evidence, "relationship", "runs_on")
	require.Equal(t, "vm", runEvidence["source_type"])
	require.Equal(t, "vm-1", runEvidence["source_moid"])
	require.Equal(t, "host", runEvidence["target_type"])
	require.Equal(t, "host-1", runEvidence["target_moid"])

	template := data.Types.OverlayTemplates[vsphereDatastoreUtilizationOverlay]
	require.Equal(t, topologyv1.OverlayProviderNetdataMetrics, template.Provider)
	require.Equal(t, []string{"vsphere.datastore_space_utilization"}, template.Contexts)
	require.Equal(t, []string{"used"}, template.Dimensions)
	require.Equal(t, []string{vsphereOverlaySelectorCollectJob, vsphereOverlaySelectorID}, template.SelectorParams)
	require.Equal(t, topologyv1.NewOverlayMerge(topologyv1.OverlayMergeRefsSet, topologyv1.OverlayMergeValuesLast), template.Merge)

	require.NotNil(t, data.Overlays)
	require.NotNil(t, data.Overlays.Refs)
	refs := topologyTableRows(t, *data.Overlays.Refs, data.Dictionaries)
	ref := requireTopologyRow(t, refs, vsphereOverlaySelectorID, "datastore-1")
	require.Equal(t, vsphereDatastoreUtilizationOverlay, ref[topologyv1.OverlayRefsTemplateColumn])
	require.Equal(t, topologyIntValue(t, datastore["_row"]), topologyIntValue(t, ref[topologyv1.OverlayRefsActorColumn]))
	require.Equal(t, "vsphere_vcenter1", ref[vsphereOverlaySelectorCollectJob])
}

func TestFuncTopology_DoesNotLinkToFilteredActors(t *testing.T) {
	collr := New()
	collr.resources = &rs.Resources{
		DataCenters: rs.DataCenters{
			"datacenter-1": {ID: "datacenter-1", Name: "DC1"},
		},
		Clusters: rs.Clusters{},
		Hosts: rs.Hosts{
			"host-1": {
				ID:   "host-1",
				Name: "Host1",
				Hier: rs.HostHierarchy{
					DC:      rs.HierarchyValue{ID: "datacenter-1", Name: "DC1"},
					Cluster: rs.HierarchyValue{ID: "domain-c-filtered", Name: "Filtered"},
				},
			},
		},
		VMs: rs.VMs{
			"vm-1": {
				ID:   "vm-1",
				Name: "VM1",
				Hier: rs.VMHierarchy{
					DC:      rs.HierarchyValue{ID: "datacenter-1", Name: "DC1"},
					Cluster: rs.HierarchyValue{ID: "domain-c-filtered", Name: "Filtered"},
					Host:    rs.HierarchyValue{ID: "host-filtered", Name: "FilteredHost"},
				},
			},
		},
	}

	data, ok := collr.topologyData("agent", "job")

	require.True(t, ok)
	validateVSphereTopologyV1Data(t, data)
	require.Equal(t, 3, data.Actors.Rows)
	actors := topologyTableRows(t, data.Actors, data.Dictionaries)
	keys := topologyLinkKeys(t, actors, topologyTableRows(t, data.Links, data.Dictionaries))
	require.Contains(t, keys, "datacenter:datacenter-1->host:host-1:vsphere_ownership")
	require.Contains(t, keys, "datacenter:datacenter-1->vm:vm-1:vsphere_ownership")
	require.NotContains(t, keys, "cluster:domain-c-filtered->host:host-1:vsphere_ownership")
	require.NotContains(t, keys, "vm:vm-1->host:host-filtered:vsphere_runs_on")
}

func TestFuncTopology_SanitizesOverlayCollectJobLikeChartLabels(t *testing.T) {
	collr := newVSphereTopologyTestCollector()

	data, ok := collr.topologyData("vsphere_vcenter1", "job'\nname\x00")

	require.True(t, ok)
	validateVSphereTopologyV1Data(t, data)
	require.Equal(t, "vsphere_vcenter1", data.Producer.Instance)
	require.NotNil(t, data.Overlays)
	require.NotNil(t, data.Overlays.Refs)
	refs := topologyTableRows(t, *data.Overlays.Refs, data.Dictionaries)
	ref := requireTopologyRow(t, refs, vsphereOverlaySelectorID, "datastore-1")
	require.Equal(t, "job name", ref[vsphereOverlaySelectorCollectJob])
}

func validateVSphereTopologyV1Data(t *testing.T, data topologyv1.Data) {
	t.Helper()

	payload, err := json.Marshal(data)
	require.NoError(t, err)

	var decodedData map[string]any
	require.NoError(t, json.Unmarshal(payload, &decodedData))
	require.NoError(t, topologyv1.ValidateDecodedData(decodedData))

	schemaBytes, err := os.ReadFile(filepath.Clean(filepath.Join("..", "..", "..", "..", "..", "plugins.d", "FUNCTION_TOPOLOGY_SCHEMA.json")))
	require.NoError(t, err)

	var schemaDoc any
	require.NoError(t, json.Unmarshal(schemaBytes, &schemaDoc))

	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("schema.json", schemaDoc))
	schema, err := compiler.Compile("schema.json")
	require.NoError(t, err)

	response := map[string]any{
		"status": 200,
		"type":   topologyv1.ResponseType,
		"data":   decodedData,
	}
	require.NoError(t, schema.Validate(response))
}

func topologyTableRows(t *testing.T, table topologyv1.Table, dictionaries topologyv1.Dictionaries) []map[string]any {
	t.Helper()

	rows := make([]map[string]any, table.Rows)
	for row := range rows {
		rows[row] = map[string]any{"_row": row}
	}

	for columnIndex, column := range table.Columns {
		values := topologyColumnValues(t, table.Rows, table.Values[columnIndex])
		for rowIndex, value := range values {
			if column.Type == "string_ref" && value != nil {
				value = resolveTopologyStringRef(t, dictionaries, column.Dictionary, value)
			}
			rows[rowIndex][column.ID] = value
		}
	}
	return rows
}

func topologyColumnValues(t *testing.T, rows int, encoding topologyv1.ColumnEncoding) []any {
	t.Helper()

	switch value := encoding.(type) {
	case topologyv1.ConstEncoding:
		values := make([]any, rows)
		for i := range values {
			values[i] = value.Value
		}
		return values
	case topologyv1.ValuesEncoding:
		require.Len(t, value.Values, rows)
		return value.Values
	case topologyv1.DictEncoding:
		require.Len(t, value.Indexes, rows)
		values := make([]any, rows)
		for i, index := range value.Indexes {
			require.GreaterOrEqual(t, index, 0)
			require.Less(t, index, len(value.Values))
			values[i] = value.Values[index]
		}
		return values
	default:
		t.Fatalf("unsupported topology column encoding %T", encoding)
		return nil
	}
}

func resolveTopologyStringRef(t *testing.T, dictionaries topologyv1.Dictionaries, name string, value any) string {
	t.Helper()

	index, ok := value.(int)
	require.True(t, ok)
	values := dictionaries[name]
	require.Less(t, index, len(values))
	text, ok := values[index].(string)
	require.True(t, ok)
	return text
}

func requireTopologyRow(t *testing.T, rows []map[string]any, key string, value any) map[string]any {
	t.Helper()

	for _, row := range rows {
		if row[key] == value {
			return row
		}
	}
	t.Fatalf("topology row with %s=%v not found", key, value)
	return nil
}

func topologyLinkKeys(t *testing.T, actors, links []map[string]any) map[string]struct{} {
	t.Helper()

	actorNames := make(map[int]string, len(actors))
	for _, actor := range actors {
		row, ok := actor["_row"].(int)
		require.True(t, ok)
		actorNames[row] = actor["object_type"].(string) + ":" + actor["vsphere_moid"].(string)
	}

	keys := make(map[string]struct{}, len(links))
	for _, link := range links {
		src := topologyIntValue(t, link["src_actor"])
		dst := topologyIntValue(t, link["dst_actor"])
		keys[actorNames[src]+"->"+actorNames[dst]+":"+link["type"].(string)] = struct{}{}
	}
	return keys
}

func topologyIntValue(t *testing.T, value any) int {
	t.Helper()

	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case uint64:
		return int(v)
	default:
		t.Fatalf("unexpected integer value %T", value)
		return 0
	}
}
