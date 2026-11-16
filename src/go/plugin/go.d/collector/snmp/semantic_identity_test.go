// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
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
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

// A supported custom profile exercises semantic text from actual PDUs in every
// typed acquisition path. Presentation normalization must not rewrite identity.
func TestTypedIdentityBytesSurviveCollection(t *testing.T) {
	const root = "1.3.6.1.4.1.99999"
	profile := &ddsnmp.Profile{
		SourceFile: "semantic-identity.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{},
	}
	require.NoError(t, yaml.Unmarshal([]byte(`
name: semantic-identity
topology:
  - kind: cdp_cache
    table: {OID: 1.3.6.1.4.1.99999.1, name: neighborTable}
    symbols:
      - {OID: 1.3.6.1.4.1.99999.1.1, name: cdp_cache}
    metric_tags:
      - tag: cdp_device_id
        symbol: {OID: 1.3.6.1.4.1.99999.1.2, name: neighborName}
      - {tag: 'rm:temporary', index: 1}
bgp:
  - id: peer
    kind: peer
    table: {OID: 1.3.6.1.4.1.99999.2, name: peerTable}
    identity:
      process_id:
        symbol: {OID: 1.3.6.1.4.1.99999.2.2, name: processName}
      routing_instance:
        symbol: {OID: 1.3.6.1.4.1.99999.2.2, name: instanceName}
      neighbor: {value: 192.0.2.1}
    traffic:
      messages:
        received:
          symbol: {OID: 1.3.6.1.4.1.99999.2.1, name: receivedMessages}
    metric_tags:
      - tag: peer_name
        symbol: {OID: 1.3.6.1.4.1.99999.2.2, name: peerName}
      - {tag: 'rm:temporary', index: 1}
licensing:
  - id: license
    table: {OID: 1.3.6.1.4.1.99999.3, name: licenseTable}
    identity:
      id:
        symbol: {OID: 1.3.6.1.4.1.99999.3.2, name: licenseID}
    signals:
      usage:
        used:
          symbol: {OID: 1.3.6.1.4.1.99999.3.1, name: licenseUsed}
    metric_tags:
      - tag: license_name
        symbol: {OID: 1.3.6.1.4.1.99999.3.2, name: licenseName}
      - {tag: 'rm:temporary', index: 1}
`), profile.Definition))
	profile.Definition.Normalize()
	require.NoError(t, ddprofiledefinition.ValidateEnrichProfile(profile.Definition))
	require.NoError(t, ddsnmp.CompileTransforms(profile))
	ddsnmp.FinalizeProfiles([]*ddsnmp.Profile{profile})

	for name, values := range map[string][]string{
		"apostrophe":  {"edge'a", "edgea"},
		"line_breaks": {"edge\r\na", "edge  a"},
		"nul":         {"edge\x00a", "edgea"},
		"backslash":   {"edge\\", "edge/"},
	} {
		t.Run(name, func(t *testing.T) {
			handler := snmpmock.NewMockHandler(gomock.NewController(t))
			handler.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()
			handler.EXPECT().BulkWalkAll(gomock.Any()).DoAndReturn(func(oid string) ([]gosnmp.SnmpPDU, error) {
				require.Contains(t, []string{root + ".1", root + ".2", root + ".3"}, oid)
				var pdus []gosnmp.SnmpPDU
				for i, value := range values {
					pdus = append(
						pdus,
						gosnmp.SnmpPDU{
							Name:  fmt.Sprintf("%s.1.%d", oid, i+1),
							Type:  gosnmp.Counter32,
							Value: uint(100 + i),
						},
						gosnmp.SnmpPDU{
							Name:  fmt.Sprintf("%s.2.%d", oid, i+1),
							Type:  gosnmp.OctetString,
							Value: []byte(value),
						},
					)
				}
				return pdus, nil
			}).AnyTimes()
			acquisition := ddsnmpcollector.New(
				ddsnmpcollector.Config{
					SnmpClient: handler,
					Profiles:   []*ddsnmp.Profile{profile},
					Log:        logger.New(),
				},
			)
			for range 2 {
				result, err := acquisition.Collect()
				require.NoError(t, err)
				require.Len(t, result, 1)
				pm := result[0]
				require.NoError(t, pm.BGPCollectError)
				require.Len(t, pm.BGPRows, 2)
				require.Len(t, pm.LicenseRows, 2)
				require.Len(t, pm.TopologyMetrics, 2)
				var peers, licenses, neighbors []string
				for _, row := range pm.BGPRows {
					peers = append(peers, row.Identity.ProcessID)
					require.Equal(t, row.Identity.ProcessID, row.Identity.RoutingInstance)
					require.Equal(t, row.Identity.ProcessID, row.Tags["peer_name"])
					require.NotContains(t, row.Tags, "rm:temporary")
				}
				for _, row := range pm.LicenseRows {
					licenses = append(licenses, row.ID)
					require.Equal(t, row.ID, row.Tags["license_name"])
					require.NotContains(t, row.Tags, "rm:temporary")
				}
				for _, row := range pm.TopologyMetrics {
					neighbors = append(neighbors, row.Tags["cdp_device_id"])
					require.NotContains(t, row.Tags, "rm:temporary")
				}
				require.ElementsMatch(t, values, peers)
				require.ElementsMatch(t, values, licenses)
				require.ElementsMatch(t, values, neighbors)

				metrics := typedBGPMetricsFromProfileMetrics(result)
				var tables []ddsnmp.Metric
				for _, metric := range metrics {
					if metric.IsTable {
						tables = append(tables, metric)
					}
				}
				c := newTestSNMPCollector()
				c.sysInfo = &snmputils.SysInfo{}
				samples := map[string]int64{}
				c.collectProfileTableMetrics(samples, tables)
				require.Len(t, c.tableMetricCharts, 2)
				require.Len(t, samples, 2)
				for _, sample := range samples {
					require.Contains(t, []int64{100, 101}, sample)
				}
				wire := renderIdentityMetrics(t, tables)
				require.Contains(t, wire, " = 100\n")
				require.Contains(t, wire, " = 101\n")
				require.Equal(t, 2, strings.Count(wire, "CLABEL 'process_id'"))
				require.NotContains(t, wire, "\r")
				require.NotContains(t, wire, "\x00")
				require.NotContains(t, wire, "\\")

				cache := newBGPPeerCache()
				cache.reset()
				for _, row := range pm.BGPRows {
					cache.updateRow("fixture", row)
				}
				cache.finalize()
				response := newTestFuncBGPPeers(cache).Handle(context.Background(), bgpPeersMethodID, nil)
				rows := responseRows(t, response)
				require.Len(t, rows, 2)
				var displayed []any
				for _, row := range rows {
					displayed = append(displayed, row[findBGPPeerColIdx("Process ID")])
				}
				require.ElementsMatch(t, []any{values[0], values[1]}, displayed)
			}
		})
	}
}
