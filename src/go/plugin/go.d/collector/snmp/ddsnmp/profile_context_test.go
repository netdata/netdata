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
	tests := map[string]struct {
		request                         ResolveRequest
		consumers                       []ProfileConsumer
		manualPolicy                    string
		manualApplied                   bool
		manualProfiles, missingProfiles []string
		sources                         []string
		projected                       []int
		acquisition                     []uint32
		bgpMode                         string
	}{
		"fallback keeps selector match": {
			request: ResolveRequest{
				SysObjectID:    "1.3.6.1.4",
				ManualProfiles: []string{"/private/SECRET/a-manual.yaml", "missing"},
				ManualPolicy:   ManualProfileFallback,
			},
			consumers:    []ProfileConsumer{ConsumerMetrics, ConsumerLicensing, ConsumerBGP},
			manualPolicy: "fallback",
			manualProfiles: []string{
				"a-manual",
				"missing",
			},
			missingProfiles: []string{"missing"},
			sources:         []string{"z-auto.yaml"},
			projected:       []int{1},
			acquisition:     []uint32{0},
			bgpMode:         "full",
		},
		"augment appends manual selection": {
			request: ResolveRequest{
				SysObjectID:    "1.3.6.1.4",
				ManualProfiles: []string{"/private/SECRET/a-manual.yaml", "missing"},
				ManualPolicy:   ManualProfileAugment,
			},
			consumers:     []ProfileConsumer{ConsumerMetrics, ConsumerLicensing, ConsumerBGP},
			manualPolicy:  "augment",
			manualApplied: true,
			manualProfiles: []string{
				"a-manual",
				"missing",
			},
			missingProfiles: []string{"missing"},
			sources:         []string{"z-auto.yaml", "a-manual.yaml"},
			projected:       []int{1, 2},
			acquisition:     []uint32{1, 0},
			bgpMode:         "full",
		},
		"override uses manual selection": {
			request: ResolveRequest{
				SysObjectID:    "1.3.6.1.4",
				ManualProfiles: []string{"/private/SECRET/a-manual.yaml", "missing"},
				ManualPolicy:   ManualProfileOverride,
			},
			consumers:     []ProfileConsumer{ConsumerMetrics, ConsumerLicensing, ConsumerBGP},
			manualPolicy:  "override",
			manualApplied: true,
			manualProfiles: []string{
				"a-manual",
				"missing",
			},
			missingProfiles: []string{"missing"},
			sources:         []string{"a-manual.yaml"},
			projected:       []int{1},
			acquisition:     []uint32{0},
			bgpMode:         "full",
		},
		"unmatched OID does not fall back to manual": {
			request: ResolveRequest{
				SysObjectID:    "9.9",
				ManualProfiles: []string{"a-manual"},
			},
			consumers:      []ProfileConsumer{ConsumerMetrics},
			manualPolicy:   "fallback",
			manualProfiles: []string{"a-manual"},
			bgpMode:        "absent",
		},
		"selected profile has no topology projection": {
			request:      ResolveRequest{SysObjectID: "1.3.6.1.4"},
			consumers:    []ProfileConsumer{ConsumerTopology},
			manualPolicy: "fallback",
			sources:      []string{"z-auto.yaml"},
			projected:    []int{0},
			bgpMode:      "absent",
		},
		"missing OID uses manual fallback": {
			request:        ResolveRequest{ManualProfiles: []string{"a-manual"}},
			consumers:      []ProfileConsumer{ConsumerMetrics},
			manualPolicy:   "fallback",
			manualApplied:  true,
			manualProfiles: []string{"a-manual"},
			sources:        []string{"a-manual.yaml"},
			projected:      []int{1},
			acquisition:    []uint32{0},
			bgpMode:        "absent",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			catalog := &Catalog{profiles: []*Profile{
				{SourceFile: "z-auto.yaml", Definition: &ddprofiledefinition.ProfileDefinition{
					Selector: ddprofiledefinition.SelectorSpec{
						{SysObjectID: ddprofiledefinition.SelectorIncludeExclude{Include: []string{"1.3.6.1.*"}}},
					},
					Metrics: []ddprofiledefinition.MetricsConfig{
						{Symbol: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.1.0", Name: "metric"}},
					},
				}},
				{SourceFile: "a-manual.yaml", Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{Symbol: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.2.0", Name: "manual_metric"}},
					},
				}},
			}}

			view := catalog.Resolve(tc.request).Project(tc.consumers[0], tc.consumers[1:]...)
			context := view.Context(250000, 64<<20)
			data := context.Snapshot()
			require.Equal(t, "available", data.State)
			require.Equal(t, tc.manualPolicy, data.ManualPolicy)
			require.Equal(t, tc.manualApplied, data.ManualApplied)
			require.Equal(t, tc.manualProfiles, data.ManualProfiles)
			require.Equal(t, tc.missingProfiles, data.MissingManualProfiles)
			require.Equal(t, tc.bgpMode, data.BGPMode)
			require.Len(t, data.Selected, len(tc.sources))
			for i, profile := range data.Selected {
				require.Equal(t, tc.sources[i], profile.Source.ID)
				require.Equal(t, tc.projected[i], profile.ProjectedOrdinal)
				if profile.ProjectedOrdinal == 0 {
					require.Nil(t, profile.AcquisitionIndex)
					require.Equal(t, ProfileProjectionCounts{}, profile.Projection)
				} else {
					require.NotNil(t, profile.AcquisitionIndex)
					require.Equal(t, tc.acquisition[i], *profile.AcquisitionIndex)
					require.Equal(t, 1, profile.Projection.Metrics)
				}
				if profile.Source.ID == "z-auto.yaml" {
					require.Equal(t, "selector", profile.Origin)
					require.Equal(t, "1.3.6.1.*", profile.MatchedSelector)
				} else {
					require.Equal(t, "manual", profile.Origin)
				}
			}
			encoded, err := json.Marshal(data)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "SECRET")
			if len(data.Selected) > 0 {
				data.Selected[0].Source.ID = "mutated"
				require.NotEqual(t, "mutated", context.Snapshot().Selected[0].Source.ID)
			}
			if len(data.ManualProfiles) > 0 {
				data.ManualProfiles[0] = "mutated"
				require.Equal(t, tc.manualProfiles, context.Snapshot().ManualProfiles)
			}
		})
	}
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
	tests := map[string]struct {
		records, bytes uint64
		state          string
	}{
		"record limit": {records: records - 1,
			bytes: size,
			state: "limit_exceeded"},
		"byte limit": {records: records,
			bytes: size - 1,
			state: "limit_exceeded"},
		"exact fit": {records: records,
			bytes: size,
			state: "available"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := v.Context(tc.records, tc.bytes).Snapshot()
			require.Equal(t, tc.state, got.State)
			if tc.state == "available" {
				require.Equal(t, data, got)
			}
		})
	}

}

func TestProfileContextFourSurfacesAndTopologyPruning(t *testing.T) {
	tests := map[string]struct {
		consumers                         []ProfileConsumer
		topologyPeers                     bool
		kinds                             map[ddprofiledefinition.TopologyKind]bool
		bgpMode                           string
		topologyKinds                     []string
		metrics, topology, licensing, bgp int
	}{
		"normal metrics licensing and full BGP": {
			consumers: []ProfileConsumer{ConsumerMetrics, ConsumerLicensing, ConsumerBGP},
			bgpMode:   "full",
			metrics:   2,
			licensing: 1,
			bgp:       3,
		},
		"topology and pruned BGP peers": {
			consumers:     []ProfileConsumer{ConsumerTopology, ConsumerBGP},
			topologyPeers: true,
			bgpMode:       "topology_peers",
			topology:      2,
			bgp:           1,
		},
		"VLAN topology kind filter": {
			consumers:     []ProfileConsumer{ConsumerTopology},
			kinds:         map[ddprofiledefinition.TopologyKind]bool{ddprofiledefinition.KindLldpRem: true},
			bgpMode:       "absent",
			topologyKinds: []string{string(ddprofiledefinition.KindLldpRem)},
			topology:      1,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			resolved := (&Catalog{profiles: []*Profile{projectionTestProfile()}}).Resolve(
				ResolveRequest{ManualProfiles: []string{"projection"}, ManualPolicy: ManualProfileAugment},
			)
			view := resolved.Project(tc.consumers[0], tc.consumers[1:]...)
			if tc.topologyPeers {
				view = view.FilterBGPToTopologyPeers()
			}
			if tc.kinds != nil {
				view = view.FilterByKind(tc.kinds)
			}
			data := view.Context(250000, 64<<20).Snapshot()
			require.Equal(t, "available", data.State)
			require.Equal(t, tc.bgpMode, data.BGPMode)
			require.Equal(t, tc.topologyKinds, data.TopologyKinds)
			require.Len(t, data.Selected, 1)
			require.Equal(t, tc.metrics, data.Selected[0].Projection.Metrics)
			require.Equal(t, tc.topology, data.Selected[0].Projection.Topology)
			require.Equal(t, tc.licensing, data.Selected[0].Projection.Licensing)
			require.Equal(t, tc.bgp, data.Selected[0].Projection.BGP)
		})
	}
}
