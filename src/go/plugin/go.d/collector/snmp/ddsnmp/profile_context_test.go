// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/stretchr/testify/require"
)

func TestProfileContextReportsActualPolicyAndProjection(t *testing.T) {
	catalog := &Catalog{profiles: []*Profile{
		{SourceFile: "z-auto.yaml", Definition: &ddprofiledefinition.ProfileDefinition{
			Selector: ddprofiledefinition.SelectorSpec{
				{SysObjectID: ddprofiledefinition.SelectorIncludeExclude{Include: []string{"1.3.6.1.*"}}},
			},
			Metrics: []ddprofiledefinition.MetricsConfig{{Symbol: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.1.0", Name: "metric"}}},
		}},
		{SourceFile: "a-manual.yaml", Definition: &ddprofiledefinition.ProfileDefinition{
			Metrics: []ddprofiledefinition.MetricsConfig{
				{Symbol: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.2.0", Name: "manual_metric"}},
			},
		}},
	}}
	for _, policy := range []ManualProfilePolicy{ManualProfileFallback, ManualProfileAugment, ManualProfileOverride} {
		view := catalog.Resolve(ResolveRequest{SysObjectID: "1.3.6.1.4", ManualProfiles: []string{"/private/SECRET/a-manual.yaml", "missing"}, ManualPolicy: policy}).
			Project(ConsumerMetrics, ConsumerLicensing, ConsumerBGP)
		c := view.Context(250000, 64<<20)
		data := c.Snapshot()
		require.Equal(t, "available", data.State)
		require.Equal(t, []string{"a-manual", "missing"}, data.ManualProfiles)
		require.Equal(t, []string{"missing"}, data.MissingManualProfiles)
		require.Equal(t, "full", data.BGPMode)
		require.Len(t, data.Selected, len(view.Profiles()))
		for i, p := range data.Selected {
			require.Equal(t, i+1, p.ProjectedOrdinal)
			require.Equal(t, 1, p.Projection.Metrics)
			if policy == ManualProfileAugment {
				if p.Source.ID == "a-manual.yaml" {
					require.EqualValues(t, 0, *p.AcquisitionIndex)
				} else {
					require.EqualValues(t, 1, *p.AcquisitionIndex)
				}
			}
			if p.Source.ID == "z-auto.yaml" {
				require.Equal(t, "selector", p.Origin)
				require.Equal(t, "1.3.6.1.*", p.MatchedSelector)
			} else {
				require.Equal(t, "manual", p.Origin)
			}
		}
		encoded, err := json.Marshal(data)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "SECRET")
		data.Selected[0].Source.ID = "mutated"
		require.NotEqual(t, "mutated", c.Snapshot().Selected[0].Source.ID)
		data.ManualProfiles[0] = "mutated"
		require.Equal(t, "a-manual", c.Snapshot().ManualProfiles[0])
	}
	// Fallback does not retry manual selection after a present but unmatched OID.
	absent := catalog.Resolve(ResolveRequest{SysObjectID: "9.9", ManualProfiles: []string{"a-manual"}}).
		Project(ConsumerMetrics).
		Context(250000, 64<<20).
		Snapshot()
	require.Empty(t, absent.Selected)
	require.False(t, absent.ManualApplied)
	// The selector matched, but the topology projection contains no applicable data.
	projectedAway := catalog.Resolve(ResolveRequest{SysObjectID: "1.3.6.1.4"}).Project(ConsumerTopology).Context(250000, 64<<20).Snapshot()
	require.Len(t, projectedAway.Selected, 1)
	require.Zero(t, projectedAway.Selected[0].ProjectedOrdinal)
	require.Equal(t, ProfileProjectionCounts{}, projectedAway.Selected[0].Projection)
	missingOID := catalog.Resolve(ResolveRequest{ManualProfiles: []string{"a-manual"}}).
		Project(ConsumerMetrics).
		Context(250000, 64<<20).
		Snapshot()
	require.True(t, missingOID.ManualApplied)
	require.Equal(t, "manual", missingOID.Selected[0].Origin)
}

func TestProfileContextLoadedSourcesAndAdmission(t *testing.T) {
	dir := t.TempDir()
	require.NoError(
		t,
		os.WriteFile(filepath.Join(dir, "_base.yaml"), []byte("metrics:\n  - symbol:\n      OID: 1.3.6.1.2.0\n      name: test\n"), 0600),
	)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "device.yaml"), []byte("extends:\n  - _base.yaml\n"), 0600))
	p, err := loadProfile(filepath.Join(dir, "device.yaml"), multipath.MultiPath{dir})
	require.NoError(t, err)
	v := (&Catalog{profiles: []*Profile{p}}).Resolve(ResolveRequest{ManualProfiles: []string{"device"}}).Project(ConsumerMetrics)
	context := v.Context(250000, 64<<20)
	data := context.Snapshot()
	require.Equal(t, ProfileSource{ID: "device.yaml", Class: "unknown", Priority: 1}, data.Selected[0].Source)
	require.Equal(
		t,
		[]ProfileExtensionContext{{Source: ProfileSource{ID: "_base.yaml", Class: "unknown", Priority: 1}}},
		data.Selected[0].Extensions,
	)
	records, size := context.Shape()
	restored, err := RestoreProfileContext(data, 250000, 64<<20)
	require.NoError(t, err)
	rr, rb := restored.Shape()
	require.Equal(t, records, rr)
	require.Equal(t, size, rb)
	require.Equal(t, "limit_exceeded", v.Context(records-1, size).Snapshot().State)
	require.Equal(t, "limit_exceeded", v.Context(records, size-1).Snapshot().State)
	require.Equal(t, data, v.Context(records, size).Snapshot())
}

func TestProfileContextFourSurfacesAndTopologyPruning(t *testing.T) {
	p := projectionTestProfile()
	resolved := (&Catalog{profiles: []*Profile{p}}).Resolve(
		ResolveRequest{ManualProfiles: []string{"projection"}, ManualPolicy: ManualProfileAugment},
	)
	normal := resolved.Project(ConsumerMetrics, ConsumerLicensing, ConsumerBGP).Context(250000, 64<<20).Snapshot()
	require.Equal(t, "available", normal.State)
	require.Equal(t, "full", normal.BGPMode)
	require.Equal(t, 2, normal.Selected[0].Projection.Metrics)
	require.Equal(t, 1, normal.Selected[0].Projection.Licensing)
	require.Equal(t, 3, normal.Selected[0].Projection.BGP)
	require.Zero(t, normal.Selected[0].Projection.Topology)
	topology := resolved.Project(ConsumerTopology, ConsumerBGP).FilterBGPToTopologyPeers().Context(250000, 64<<20).Snapshot()
	require.Equal(t, "available", topology.State)
	require.Equal(t, "topology_peers", topology.BGPMode)
	require.Equal(t, 2, topology.Selected[0].Projection.Topology)
	require.Equal(t, 1, topology.Selected[0].Projection.BGP)
	require.Zero(t, topology.Selected[0].Projection.Metrics)
	require.Zero(t, topology.Selected[0].Projection.Licensing)
	vlan := resolved.Project(ConsumerTopology).
		FilterByKind(map[ddprofiledefinition.TopologyKind]bool{ddprofiledefinition.KindLldpRem: true}).
		Context(250000, 64<<20).
		Snapshot()
	require.Equal(t, "available", vlan.State)
	require.Equal(t, []string{string(ddprofiledefinition.KindLldpRem)}, vlan.TopologyKinds)
	require.Equal(t, 1, vlan.Selected[0].Projection.Topology)
	require.Equal(t, "absent", vlan.BGPMode)
}
