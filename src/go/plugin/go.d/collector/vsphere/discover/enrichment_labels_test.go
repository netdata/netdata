// SPDX-License-Identifier: GPL-3.0-or-later

package discover

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

func TestAddCustomAttributeLabels(t *testing.T) {
	var labels map[string]string
	values := map[int32]string{
		1: "platform",
		2: "prod",
		3: "secret",
	}
	namesByKey := map[int32]string{
		1: "Owner Team",
		2: "Env.Name",
	}

	addCustomAttributeLabels(&labels, values, namesByKey)

	require.Equal(t, map[string]string{
		"vsphere_custom_attribute_env_name":   "prod",
		"vsphere_custom_attribute_owner_team": "platform",
	}, labels)
}

func TestAddTagLabels(t *testing.T) {
	m, err := matcher.NewSimplePatternsMatcher("Business* Env")
	require.NoError(t, err)
	var labels map[string]string
	tagsByCategory := map[string][]string{
		"Business Unit": {"Payments", "Core", "Core"},
		"Env":           {"prod"},
		"Secret":        {"hidden"},
	}

	addTagLabels(&labels, tagsByCategory, m)

	require.Equal(t, map[string]string{
		"vsphere_tag_business_unit": "Core|Payments",
		"vsphere_tag_env":           "prod",
	}, labels)
}

func TestResourceLabelsByRefFindsClusterByValue(t *testing.T) {
	res := &rs.Resources{
		Clusters: rs.Clusters{
			"domain-c1": &rs.Cluster{ID: "domain-c1"},
		},
	}

	labels := resourceLabelsByRef(res, types.ManagedObjectReference{Type: "ClusterComputeResource", Value: "domain-c1"})
	require.NotNil(t, labels)

	addUserMetadataLabel(labels, "vsphere_tag_env", "prod")
	require.Equal(t, "prod", res.Clusters.Get("domain-c1").Labels["vsphere_tag_env"])
}

func TestDiscovererPathSetsAddCustomValueOnlyWhenCustomAttributesEnabled(t *testing.T) {
	m, err := matcher.NewSimplePatternsMatcher("Owner")
	require.NoError(t, err)

	require.NotContains(t, Discoverer{}.hostPathSet(), "customValue")
	for name, pathSet := range map[string][]string{
		"hosts":              Discoverer{CustomAttributeMatcher: m}.hostPathSet(),
		"VMs":                Discoverer{CustomAttributeMatcher: m}.vmPathSet(),
		"datastores":         Discoverer{CustomAttributeMatcher: m}.datastorePathSet(),
		"clusters":           Discoverer{CustomAttributeMatcher: m}.clusterPathSet(),
		"resource pools":     Discoverer{CustomAttributeMatcher: m}.resourcePoolPathSet(),
		"datastore clusters": Discoverer{CustomAttributeMatcher: m}.storagePodPathSet(),
	} {
		t.Run(name, func(t *testing.T) {
			require.Contains(t, pathSet, "customValue")
		})
	}
}

func TestDiscovererPathSetsAddVSANFieldsOnlyWhenEnabled(t *testing.T) {
	tests := map[string]struct {
		disabled []string
		enabled  []string
		field    string
	}{
		"clusters": {
			disabled: Discoverer{}.clusterPathSet(),
			enabled:  Discoverer{CollectVSAN: true}.clusterPathSet(),
			field:    "configurationEx.vsanConfigInfo",
		},
		"hosts": {
			disabled: Discoverer{}.hostPathSet(),
			enabled:  Discoverer{CollectVSAN: true}.hostPathSet(),
			field:    "config.vsanHostConfig.clusterInfo.nodeUuid",
		},
		"VMs": {
			disabled: Discoverer{}.vmPathSet(),
			enabled:  Discoverer{CollectVSAN: true}.vmPathSet(),
			field:    "config.instanceUuid",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.NotContains(t, tc.disabled, tc.field)
			require.Contains(t, tc.enabled, tc.field)
		})
	}
}
