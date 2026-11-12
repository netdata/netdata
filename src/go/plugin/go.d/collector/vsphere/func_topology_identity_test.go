// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"context"
	"errors"
	"testing"

	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"
)

func TestVSphereTopologyQualifiedIdentity(t *testing.T) {
	makeData := func(uuid, job string) topologyv1.Data {
		collr := newVSphereTopologyTestCollector()
		collr.resources.VCenterInstanceUUID = uuid
		data, ok := collr.topologyData(job, job)
		require.True(t, ok)
		validateVSphereTopologyV1Data(t, data)
		return data
	}
	first := makeData("vcenter-uuid-a", "job-a")
	same := makeData("vcenter-uuid-a", "job-b")
	other := makeData("vcenter-uuid-b", "job-a")
	firstRows := topologyTableRows(t, first.Actors, first.Dictionaries)
	sameRows := topologyTableRows(t, same.Actors, same.Dictionaries)
	otherRows := topologyTableRows(t, other.Actors, other.Dictionaries)
	for i, row := range firstRows {
		actorType := row["type"].(string)
		definition := first.Types.ActorTypes[actorType]
		assert.Equal(t, []string{"vsphere_instance_uuid", "object_type", "vsphere_moid"}, definition.Identity)
		assert.Equal(t, definition.Identity, definition.MergeIdentity)
		assert.Equal(t, []string{"vsphere_instance_uuid", "parent_object_type", "parent_moid"}, definition.ParentIdentity)
		assert.Equal(t, identityValues(row, definition.MergeIdentity), identityValues(sameRows[i], definition.MergeIdentity))
		assert.NotEqual(t, identityValues(row, definition.MergeIdentity), identityValues(otherRows[i], definition.MergeIdentity))
		if row["parent_moid"] != nil {
			parent := requireTopologyRow(t, firstRows, "vsphere_moid", row["parent_moid"])
			assert.Equal(t, identityValues(parent, definition.Identity), identityValues(row, definition.ParentIdentity))
		}
	}
	assert.Empty(t, first.Notifications)
}

func TestVSphereTopologyUnknownIdentityPreservesLocalMap(t *testing.T) {
	collr := newVSphereTopologyTestCollector()
	qualified, ok := collr.topologyData("job-a", "job-a")
	require.True(t, ok)
	collr.resources.VCenterInstanceUUID = " \t "
	local, ok := collr.topologyData("job-a", "job-a")
	require.True(t, ok)
	validateVSphereTopologyV1Data(t, local)
	require.Len(t, local.Notifications, 1)
	assert.Equal(t, "warning", local.Notifications[0].Severity)
	assert.Equal(t, "vsphere.identity_unqualified", local.Notifications[0].Code)
	assert.Nil(t, local.Notifications[0].Origin)
	for _, definition := range local.Types.ActorTypes {
		assert.Equal(t, []string{"object_type", "vsphere_moid"}, definition.Identity)
		assert.Empty(t, definition.MergeIdentity)
		assert.Equal(t, []string{"parent_object_type", "parent_moid"}, definition.ParentIdentity)
	}
	qualifiedActors := topologyTableRows(t, qualified.Actors, qualified.Dictionaries)
	localActors := topologyTableRows(t, local.Actors, local.Dictionaries)
	for i := range localActors {
		assert.Nil(t, localActors[i]["vsphere_instance_uuid"])
		delete(localActors[i], "vsphere_instance_uuid")
		delete(qualifiedActors[i], "vsphere_instance_uuid")
	}
	assert.Equal(t, qualifiedActors, localActors)
	assert.Equal(t, topologyTableRows(t, qualified.Links, qualified.Dictionaries), topologyTableRows(t, local.Links, local.Dictionaries))
	for key, section := range qualified.Evidence {
		assert.Equal(t, topologyTableRows(t, section.Table, qualified.Dictionaries), topologyTableRows(t, local.Evidence[key].Table, local.Dictionaries))
	}
	for key, section := range qualified.Tables.Actor {
		assert.Equal(t, topologyTableRows(t, section.Table, qualified.Dictionaries), topologyTableRows(t, local.Tables.Actor[key].Table, local.Dictionaries))
	}
	require.NotNil(t, qualified.Overlays)
	require.NotNil(t, local.Overlays)
	assert.Equal(t, topologyTableRows(t, *qualified.Overlays.Refs, qualified.Dictionaries), topologyTableRows(t, *local.Overlays.Refs, local.Dictionaries))
	assert.Equal(t, qualified.Presentation, local.Presentation)
	assert.Equal(t, qualified.Stats, local.Stats)
}

func TestVSphereNamespaceCapturedWithInventory(t *testing.T) {
	for _, uuid := range []string{"vcenter-uuid-a", ""} {
		t.Run("uuid="+uuid, func(t *testing.T) {
			collr, model, teardown := prepareVSphereSim(t)
			defer teardown()
			service := model.Service.Context.Map.Get(vim25.ServiceInstance).(*simulator.ServiceInstance)
			service.Content.About.InstanceUuid = uuid
			require.NoError(t, collr.Init(t.Context()))
			collr.stopDiscoveryTask(true)
			require.NotNil(t, collr.resources)
			assert.Equal(t, uuid, collr.resources.VCenterInstanceUUID)
			// Cleanup clears the client; cached topology must retain its original namespace.
			collr.Cleanup(context.Background())
			data, ok := collr.topologyData("job-a", "job-a")
			require.True(t, ok)
			validateVSphereTopologyV1Data(t, data)
			if uuid == "" {
				assert.Len(t, data.Notifications, 1)
			} else {
				assert.Empty(t, data.Notifications)
			}
		})
	}
}

func TestVSphereNamespaceRetainedOnDiscoveryFailure(t *testing.T) {
	collr := newVSphereTopologyTestCollector()
	previous := collr.resources
	collr.discoverer = namespaceFailureDiscoverer{}
	require.Error(t, collr.discoverOnce())
	assert.Same(t, previous, collr.resources)
	assert.Equal(t, "vcenter-uuid-a", collr.resources.VCenterInstanceUUID)
}

type namespaceFailureDiscoverer struct{}

func (namespaceFailureDiscoverer) Discover() (*rs.Resources, error) {
	return nil, errors.New("inventory unavailable")
}

func identityValues(row map[string]any, columns []string) []any {
	values := make([]any, len(columns))
	for i, column := range columns {
		values[i] = row[column]
	}
	return values
}
