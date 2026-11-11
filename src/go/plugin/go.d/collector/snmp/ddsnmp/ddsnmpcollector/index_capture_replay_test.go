// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"slices"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// These are protocol/capture oracles, not expected values computed from the profile's index configuration.
// SwOS returns an offset INDEX column, cetpower repeats zero for distinct phases, and istars/Check Point append .0.
// All cases replay stock YAML (including inheritance), first WALK and cached GET, with and without the old column.
func TestStockIndexCaptureReplay(t *testing.T) {
	tests := []struct {
		name, fixture, profile, table, root, oldColumn, metric, identity string
		want                                                             map[string]int64
	}{
		{
			name:      "SwOS offset",
			fixture:   "swos",
			profile:   "mikrotik-swos",
			table:     "mtxrPOETable",
			root:      "1.3.6.1.4.1.14988.1.1.15.1",
			oldColumn: "1.3.6.1.4.1.14988.1.1.15.1.1.1",
			metric:    "mtxrPOEPower",
			identity:  "poe_interface_index",
			want:      map[string]int64{"2": 0, "3": 0, "4": 0, "5": 0},
		},
		{
			name:      "cetpower output duplicate zero",
			fixture:   "cetpower",
			profile:   "generic-ups",
			table:     "upsOutputTable",
			root:      "1.3.6.1.2.1.33.1.4.4",
			oldColumn: "1.3.6.1.2.1.33.1.4.4.1.1",
			metric:    "upsOutputVoltage",
			identity:  "ups_output_line_index",
			want:      map[string]int64{"1": 230, "2": 0, "3": 0},
		},
		{
			name:      "cetpower input duplicate zero",
			fixture:   "cetpower",
			profile:   "generic-ups",
			table:     "upsInputTable",
			root:      "1.3.6.1.2.1.33.1.3.3",
			oldColumn: "1.3.6.1.2.1.33.1.3.3.1.1",
			metric:    "upsInputVoltage",
			identity:  "ups_input_line_index",
			want:      map[string]int64{"1": 238, "2": 0, "3": 0},
		},
		{
			name:      "cetpower bypass duplicate zero",
			fixture:   "cetpower",
			profile:   "generic-ups",
			table:     "upsBypassTable",
			root:      "1.3.6.1.2.1.33.1.5.3",
			oldColumn: "1.3.6.1.2.1.33.1.5.3.1.1",
			metric:    "upsBypassVoltage",
			identity:  "ups_bypass_line_index",
			want:      map[string]int64{"1": 238, "2": 0, "3": 0},
		},
		{
			name:      "istars output extra zero",
			fixture:   "istars",
			profile:   "generic-ups",
			table:     "upsOutputTable",
			root:      "1.3.6.1.2.1.33.1.4.4",
			oldColumn: "1.3.6.1.2.1.33.1.4.4.1.1",
			metric:    "upsOutputVoltage",
			identity:  "ups_output_line_index",
			want:      map[string]int64{"1": 115},
		},
		{
			name:      "istars input extra zero",
			fixture:   "istars",
			profile:   "generic-ups",
			table:     "upsInputTable",
			root:      "1.3.6.1.2.1.33.1.3.3",
			oldColumn: "1.3.6.1.2.1.33.1.3.3.1.1",
			metric:    "upsInputVoltage",
			identity:  "ups_input_line_index",
			want:      map[string]int64{"1": 224},
		},
		{
			name:      "istars bypass extra zero",
			fixture:   "istars",
			profile:   "generic-ups",
			table:     "upsBypassTable",
			root:      "1.3.6.1.2.1.33.1.5.3",
			oldColumn: "1.3.6.1.2.1.33.1.5.3.1.1",
			metric:    "upsBypassVoltage",
			identity:  "ups_bypass_line_index",
			want:      map[string]int64{"1": 224},
		},
		{
			name:      "Check Point extra zero",
			fixture:   "gaia_5900",
			profile:   "checkpoint",
			table:     "powerSupplyTable",
			root:      "1.3.6.1.4.1.2620.1.6.7.9.1",
			oldColumn: "1.3.6.1.4.1.2620.1.6.7.9.1.1.1",
			metric:    "powerSupplyStatus",
			identity:  "psu_index",
			want:      map[string]int64{"1": 1, "2": 1},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, oldColumnPresent := range []bool{true, false} {
				mode := "old column absent"
				if oldColumnPresent {
					mode = "old column returned"
				}
				t.Run(mode, func(t *testing.T) {
					fixture := mustLoadSNMPFixture(t, "testdata/index-captures/"+tc.fixture+".snmprec")
					if !oldColumnPresent {
						fixture.entries = slices.DeleteFunc(
							fixture.entries,
							func(pdu gosnmp.SnmpPDU) bool { return strings.HasPrefix(pdu.Name, tc.oldColumn+".") },
						)
					}
					ctrl, handler := setupMockHandler(t)
					defer ctrl.Finish()
					expectSNMPWalkFromFixture(handler, gosnmp.Version2c, fixture, tc.root)
					getCalls := 0
					handler.EXPECT().Get(gomock.Any()).DoAndReturn(func(oids []string) (*gosnmp.SnmpPacket, error) {
						getCalls++
						pdus := make([]gosnmp.SnmpPDU, 0, len(oids))
						for _, oid := range oids {
							require.False(
								t,
								strings.HasPrefix(trimOID(oid), tc.oldColumn+"."),
								"cached acquisition must not request inaccessible INDEX column",
							)
							pdu, ok := fixture.byOID[trimOID(oid)]
							require.True(t, ok, "unexpected cached acquisition %s", oid)
							pdus = append(pdus, pdu)
						}
						return &gosnmp.SnmpPacket{
							Variables: pdus,
						}, nil
					}).AnyTimes()
					profile := mustLoadIndexCaptureRow(t, tc.profile, tc.table)
					collector := New(Config{
						SnmpClient: handler,
						Profiles:   []*ddsnmp.Profile{profile},
						Log:        logger.New(),
					})
					for _, phase := range []string{"initial WALK", "cached GET"} {
						t.Run(phase, func(t *testing.T) {
							result, err := collector.Collect()
							require.NoError(t, err)
							require.Len(t, result, 1)
							require.Zero(t, result[0].Stats.Errors.Processing.Table)
							actual := map[string]int64{}
							for _, metric := range result[0].Metrics {
								require.Contains(
									t,
									tc.want,
									metric.Tags[tc.identity],
									"every metric must preserve the row's real identity",
								)
								if metric.Name == tc.metric {
									key := metric.Tags[tc.identity]
									_, duplicate := actual[key]
									require.False(t, duplicate, "distinct captured rows must not collapse")
									actual[key] = metric.Value
								}
							}
							require.Equal(t, tc.want, actual)
						})
					}
					require.Positive(t, getCalls, "second collection must exercise cached acquisition")
				})
			}
		})
	}
}

func mustLoadIndexCaptureRow(t *testing.T, name, table string) *ddsnmp.Profile {
	t.Helper()
	profile, err := ddsnmp.LoadProfileByName(name)
	require.NoError(t, err)
	// Narrow the workload, preserving the stock row's metric/tag declarations and preparation unchanged.
	profile.Definition.Metrics = slices.DeleteFunc(
		profile.Definition.Metrics,
		func(row ddprofiledefinition.MetricsConfig) bool { return row.Table.Name != table },
	)
	require.Len(t, profile.Definition.Metrics, 1)
	profile.Definition.Metadata = nil
	profile.Definition.SysobjectIDMetadata = nil
	profile.Definition.MetricTags = nil
	profile.Definition.StaticTags = nil
	profile.Definition.VirtualMetrics = nil
	profile.Definition.Topology = nil
	profile.Definition.Licensing = nil
	profile.Definition.BGP = nil
	return profile
}

func TestStockIndexCaptureReplay_RB750ScopedAcquisition(t *testing.T) {
	for _, tc := range []struct {
		profile  string
		wantRows int
	}{{"topology-role-mikrotik-rb750gr3", 2}, {"_std-topology-lldp-mib", 0}} {
		t.Run(tc.profile, func(t *testing.T) {
			profile, err := ddsnmp.LoadProfileByName(tc.profile)
			require.NoError(t, err)
			profile.Definition.Topology = slices.DeleteFunc(
				profile.Definition.Topology,
				func(row ddprofiledefinition.TopologyConfig) bool { return row.Table.Name != "lldpRemManAddrTable" },
			)
			require.Len(t, profile.Definition.Topology, 1)
			profile.Definition = &ddprofiledefinition.ProfileDefinition{
				Topology: profile.Definition.Topology,
			}
			fixture := mustLoadSNMPFixture(t, "../testdata/profile-mib-access-rb750gr3.snmprec")
			ctrl, handler := setupMockHandler(t)
			defer ctrl.Finish()
			expectSNMPWalkFromFixture(handler, gosnmp.Version2c, fixture, "1.0.8802.1.1.2.1.4.2")
			collector := New(Config{
				SnmpClient: handler,
				Profiles:   []*ddsnmp.Profile{profile},
				Log:        logger.New(),
			})
			result, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, result, 1)
			require.Zero(t, result[0].Stats.Errors.Processing.Table)
			require.Len(t, result[0].TopologyMetrics, tc.wantRows)
			expected := map[string]map[string]string{
				"6": {
					"lldp_loc_port_num":          "6",
					"lldp_rem_index":             "2",
					"lldp_rem_mgmt_addr_subtype": "1",
					"lldp_rem_mgmt_addr":         "c0000201",
				},
				"10": {
					"lldp_loc_port_num":          "10",
					"lldp_rem_index":             "1",
					"lldp_rem_mgmt_addr_subtype": "2",
					"lldp_rem_mgmt_addr":         "20010db8000000000000000000000001",
				},
			}
			for _, metric := range result[0].TopologyMetrics {
				require.Equal(t, "lldp_rem_man_addr", metric.Name)
				want, ok := expected[metric.Tags["lldp_loc_port_num"]]
				require.True(t, ok)
				for tag, value := range want {
					require.Equal(t, value, metric.Tags[tag], tag)
				}
				require.NotContains(
					t,
					metric.Tags,
					"lldp_rem_mgmt_addr_len",
					"truncated suffix cannot fabricate a length",
				)
				require.NotContains(
					t,
					metric.Tags,
					"lldp_rem_mgmt_addr_if_subtype",
					"capture has no standard readable anchor",
				)
				delete(expected, metric.Tags["lldp_loc_port_num"])
			}
			if tc.wantRows > 0 {
				require.Empty(t, expected)
			}
		})
	}
}
