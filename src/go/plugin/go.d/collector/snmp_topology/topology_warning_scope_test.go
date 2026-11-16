// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologydiag"
)

func TestTopologyRowWarningsAreScopedAcrossRefreshes(t *testing.T) {
	const root = "1.3.6.1.4.1.99999.983"
	definition := &ddprofiledefinition.ProfileDefinition{}
	require.NoError(t, yaml.Unmarshal([]byte(`topology:
- kind: if_name
  table: {OID: 1.3.6.1.4.1.99999.983, name: interfaces}
  symbols: [{OID: 1.3.6.1.4.1.99999.983.1, name: ifName}]
  metric_tags:
  - tag: if_index
    index_decode: {components: [{type: integer}], component: 1}
`), definition))
	definition.Normalize()
	require.NoError(t, ddprofiledefinition.ValidateEnrichProfile(definition))
	profile := &ddsnmp.Profile{SourceFile: "strict-topology.yaml", Definition: definition}
	ddsnmp.FinalizeProfiles([]*ddsnmp.Profile{profile})
	coll := newTestSNMPTopologyCollector()
	var output bytes.Buffer
	coll.Logger = logger.NewWithWriter(&output)
	coll.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile { return []*ddsnmp.Profile{profile} }
	for refresh := 0; refresh < 2; refresh++ {
		for i, host := range []string{"192.0.2.10", "192.0.2.11"} {
			dev := ddsnmp.DeviceConnectionInfo{Hostname: host, Port: 161, SNMPVersion: "2c"}
			handler := snmpmock.NewMockHandler(gomock.NewController(t))
			expectTopologyRefreshSNMPClient(handler, dev)
			handler.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()
			handler.EXPECT().BulkWalkAll(root).Return([]gosnmp.SnmpPDU{{Name: fmt.Sprintf("%s.1.7.8", root), Type: gosnmp.OctetString, Value: []byte("Gi0/1")}}, nil)
			coll.newSnmpClient = func() gosnmp.Handler { return handler }
			attempt := testTopologyAttemptID(ddsnmp.DeviceRegistrationID(i + 1))
			attempt.Ordinal += uint64(refresh)
			snapshot, outcome, capture := coll.refreshDeviceTopology(context.Background(), attempt, dev, testTopologyTarget())
			require.Equal(t, topologydiag.RefreshOutcomeSuccess, outcome)
			require.NotNil(t, snapshot)
			require.NotNil(t, capture)
			// Context order may change without resetting each VLAN's warning budget.
			vlans := []string{"100", "200"}
			if refresh > 0 {
				vlans = []string{"200", "100"}
			}
			for _, vlan := range vlans {
				handler := snmpmock.NewMockHandler(gomock.NewController(t))
				expectTopologyVLANClient(handler, dev, vlan, nil)
				handler.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()
				handler.EXPECT().BulkWalkAll(root).Return([]gosnmp.SnmpPDU{{Name: fmt.Sprintf("%s.1.7.8", root), Type: gosnmp.OctetString, Value: []byte("Gi0/1")}}, nil)
				handler.EXPECT().Close().Return(nil)
				coll.newSnmpClient = func() gosnmp.Handler { return handler }
				_, progress, err := collectTopologyVLANContext(context.Background(), coll, attempt.RegistrationID, dev, vlan, []*ddsnmp.Profile{profile}, nil)
				require.NoError(t, err)
				require.Equal(t, topologydiag.AcquisitionPhaseSuccess, progress.collection.Outcome)
			}
		}
		require.Equal(t, 6, strings.Count(output.String(), "failed to collect 1 rows"), output.String())
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	require.Len(t, lines, 6, output.String())
	for i, host := range []string{"192.0.2.10", "192.0.2.11"} {
		for _, scope := range []string{"main", "100", "200"} {
			count := 0
			for _, line := range lines {
				if !strings.Contains(line, "device="+host) || !strings.Contains(line, fmt.Sprintf("registration=%d", i+1)) {
					continue
				}
				matches := scope == "main" && strings.Contains(line, "context=main")
				matches = matches || scope != "main" && strings.Contains(line, "context=vlan") && strings.Contains(line, "vlan="+scope)
				if matches {
					require.Contains(t, line, "strict-topology.yaml")
					count++
				}
			}
			require.Equal(t, 1, count, "%s %s: %s", host, scope, output.String())
		}
	}
}
