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
	require.Contains(t, Discoverer{CustomAttributeMatcher: m}.hostPathSet(), "customValue")
	require.Contains(t, Discoverer{CustomAttributeMatcher: m}.vmPathSet(), "customValue")
	require.Contains(t, Discoverer{CustomAttributeMatcher: m}.datastorePathSet(), "customValue")
	require.Contains(t, Discoverer{CustomAttributeMatcher: m}.clusterPathSet(), "customValue")
	require.Contains(t, Discoverer{CustomAttributeMatcher: m}.resourcePoolPathSet(), "customValue")
	require.Contains(t, Discoverer{CustomAttributeMatcher: m}.storagePodPathSet(), "customValue")
}

func TestDiscovererPathSetsAddVSANFieldsOnlyWhenEnabled(t *testing.T) {
	require.NotContains(t, Discoverer{}.clusterPathSet(), "configurationEx.vsanConfigInfo")
	require.NotContains(t, Discoverer{}.hostPathSet(), "config.vsanHostConfig.clusterInfo.nodeUuid")
	require.NotContains(t, Discoverer{}.vmPathSet(), "config.instanceUuid")

	d := Discoverer{CollectVSAN: true}
	require.Contains(t, d.clusterPathSet(), "configurationEx.vsanConfigInfo")
	require.Contains(t, d.hostPathSet(), "config.vsanHostConfig.clusterInfo.nodeUuid")
	require.Contains(t, d.vmPathSet(), "config.instanceUuid")
}
