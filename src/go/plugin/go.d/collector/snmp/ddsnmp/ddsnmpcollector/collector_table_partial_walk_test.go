// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils/snmpmock"
)

// A walk an error-status ended keeps the rows before it: they are collected
// and feed the tables that depend on them, the status counts as an SNMP error,
// and nothing built from the partial rows is served from the cache: the next
// collection walks both tables again (the mock allows no GET).
func TestCollector_PartialTableWalk(t *testing.T) {
	const (
		table      = "1.3.6.1.4.1.25461.1.1.7.1.2.1"
		valueOID   = table + ".1.1"
		descrTable = "1.3.6.1.2.1.47.1.1.1.1.2"
	)
	status := &gosnmp.StatusError{Status: gosnmp.GenErr, Index: 1}
	values := []gosnmp.VarBind{createGauge32PDU(valueOID+".1", 100), createGauge32PDU(valueOID+".2", 150)}
	descrs := []gosnmp.VarBind{
		createStringPDU(descrTable+".1", "Power Supply 1"),
		createStringPDU(descrTable+".2", "Power Supply 2"),
	}
	metric := func(value int64, descr string) ddsnmp.Metric {
		m := ddsnmp.Metric{
			Name:       "power",
			Value:      value,
			Tags:       nil,
			MetricType: "gauge",
			IsTable:    true,
			Table:      "fruTable",
		}
		if descr != "" {
			m.Tags = map[string]string{"ent_descr": descr}
		}
		return m
	}

	tests := map[string]struct {
		walks func(m *snmpmock.MockSession)
		want  [2][]ddsnmp.Metric
	}{
		"primary table": {
			walks: func(m *snmpmock.MockSession) {
				// Expectations of one table match in order.
				m.EXPECT().BulkWalkAll(gomock.Any(), table, gomock.Any()).Return(values[:1], gosnmp.WalkEnd{}, status)
				m.EXPECT().BulkWalkAll(gomock.Any(), table, gomock.Any()).Return(values, gosnmp.WalkEnd{}, nil)
				m.EXPECT().BulkWalkAll(gomock.Any(), descrTable, gomock.Any()).Return(descrs, gosnmp.WalkEnd{}, nil).Times(2)
			},
			want: [2][]ddsnmp.Metric{
				{metric(100, "Power Supply 1")},
				{metric(100, "Power Supply 1"), metric(150, "Power Supply 2")},
			},
		},
		"dependency table": {
			walks: func(m *snmpmock.MockSession) {
				m.EXPECT().BulkWalkAll(gomock.Any(), table, gomock.Any()).Return(values, gosnmp.WalkEnd{}, nil).Times(2)
				m.EXPECT().BulkWalkAll(gomock.Any(), descrTable, gomock.Any()).Return(descrs[:1], gosnmp.WalkEnd{}, status)
				m.EXPECT().BulkWalkAll(gomock.Any(), descrTable, gomock.Any()).Return(descrs, gosnmp.WalkEnd{}, nil)
			},
			want: [2][]ddsnmp.Metric{
				{metric(100, "Power Supply 1"), metric(150, "")},
				{metric(100, "Power Supply 1"), metric(150, "Power Supply 2")},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl, session := setupMockSession(t)
			defer ctrl.Finish()
			tc.walks(session)
			profile := &ddsnmp.Profile{
				SourceFile: "partial.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{{
						Table:   ddprofiledefinition.SymbolConfig{OID: table, Name: "fruTable"},
						Symbols: []ddprofiledefinition.SymbolConfig{{OID: valueOID, Name: "power"}},
						MetricTags: []ddprofiledefinition.MetricTagConfig{{
							Tag:    "ent_descr",
							Symbol: ddprofiledefinition.SymbolConfigCompat{OID: descrTable, Name: "entPhysicalDescr"},
							Table:  "entPhysicalTable",
						}},
					}},
				},
			}
			ddsnmp.FinalizeProfiles([]*ddsnmp.Profile{profile})
			collector := New(Config{
				Version:    gosnmp.Version2c,
				MaxOIDs:    10,
				SnmpClient: session,
				Profiles:   []*ddsnmp.Profile{profile},
				Log:        logger.New(),
			})
			collector.tableCache.setTTL(30*time.Minute, 0)

			for i, want := range tc.want {
				result, err := collector.Collect(context.Background())
				require.NoError(t, err, "collection %d", i+1)
				require.Len(t, result, 1, "collection %d", i+1)
				for j := range result[0].Metrics {
					result[0].Metrics[j].Profile = nil
				}
				assert.ElementsMatch(t, want, result[0].Metrics, "collection %d", i+1)
				if i == 0 {
					assert.EqualValues(t, 1, result[0].Stats.Errors.SNMP, "the status is an SNMP error")
				}
			}
		})
	}
}

// A licensing walk an error-status ended keeps the rows before it, and those
// of a dependency feed the rows' tags; the status is a licensing failure.
func TestCollector_PartialLicensingWalk(t *testing.T) {
	const (
		table   = "1.3.6.1.4.1.99999.6"
		ifNames = "1.3.6.1.2.1.31.1.1.1.1"
	)
	status := &gosnmp.StatusError{Status: gosnmp.GenErr, Index: 1}
	states := []gosnmp.VarBind{createIntegerPDU(table+".1.1", 0), createIntegerPDU(table+".1.2", 1)}
	names := []gosnmp.VarBind{createStringPDU(ifNames+".1", "eth1"), createStringPDU(ifNames+".2", "eth2")}

	tests := map[string]struct {
		states, names []gosnmp.VarBind
		statesErr     error
		namesErr      error
		want          map[string]map[string]string
	}{
		"primary table": {
			states:    states[:1],
			statesErr: status,
			names:     names,
			want:      map[string]map[string]string{"1": {"if_name": "eth1"}},
		},
		"dependency table": {
			states:   states,
			names:    names[:1],
			namesErr: status,
			want:     map[string]map[string]string{"1": {"if_name": "eth1"}, "2": {}},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl, session := setupMockSession(t)
			defer ctrl.Finish()
			session.EXPECT().BulkWalkAll(gomock.Any(), table, gomock.Any()).Return(tc.states, gosnmp.WalkEnd{}, tc.statesErr)
			session.EXPECT().BulkWalkAll(gomock.Any(), ifNames, gomock.Any()).Return(tc.names, gosnmp.WalkEnd{}, tc.namesErr)
			profile := &ddsnmp.Profile{
				SourceFile: "vendor-device.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Licensing: []ddprofiledefinition.LicensingConfig{{
						OriginProfileID: "_vendor-licensing.yaml",
						Table:           ddprofiledefinition.SymbolConfig{OID: table, Name: "licenseIfTable"},
						Identity:        ddprofiledefinition.LicenseIdentityConfig{ID: ddprofiledefinition.LicenseValueConfig{Index: 1}},
						State: ddprofiledefinition.LicenseStateConfig{
							LicenseValueConfig: ddprofiledefinition.LicenseValueConfig{
								Symbol: ddprofiledefinition.SymbolConfig{OID: table + ".1", Name: "licenseState"},
							},
							Policy: ddprofiledefinition.LicenseStatePolicyDefault,
						},
						MetricTags: []ddprofiledefinition.MetricTagConfig{{
							Tag:    "if_name",
							Table:  "ifXTable",
							Symbol: ddprofiledefinition.SymbolConfigCompat{OID: ifNames, Name: "ifName"},
						}},
					}},
				},
			}
			collector := New(Config{
				Version:    gosnmp.Version2c,
				MaxOIDs:    10,
				SnmpClient: session,
				Profiles:   []*ddsnmp.Profile{profile},
				Log:        logger.New(),
			})

			results, err := collector.Collect(context.Background())
			require.NoError(t, err)
			require.Len(t, results, 1)
			got := make(map[string]map[string]string)
			for _, row := range results[0].LicenseRows {
				got[row.ID] = row.Tags
			}
			assert.Equal(t, tc.want, got)
			failures := collector.CollectionFailures()
			assert.EqualValues(t, 1, failures.Licensing.Count)
			assert.Equal(t, "packet_error", failures.Licensing.Last.Reason)
		})
	}
}
