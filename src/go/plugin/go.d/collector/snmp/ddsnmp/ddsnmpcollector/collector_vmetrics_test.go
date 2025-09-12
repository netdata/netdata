// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestVirtualMetricsCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		profileDef       *ddprofiledefinition.ProfileDefinition
		collectedMetrics []ddsnmp.Metric
		expected         []ddsnmp.Metric
	}{
		"basic sum of single table metric": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				Metrics: []ddprofiledefinition.MetricsConfig{
					{
						Table: ddprofiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.31.1.1",
							Name: "ifXTable",
						},
						Symbols: []ddprofiledefinition.SymbolConfig{
							{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
						},
					},
				},
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name: "ifTotalInOctets",
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ifHCInOctets", Table: "ifXTable"},
						},
						ChartMeta: ddprofiledefinition.ChartMeta{
							Description: "Total inbound traffic",
							Family:      "Network/Total/Traffic/In",
							Unit:        "bit/s",
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				{Name: "ifHCInOctets", Value: 1000, IsTable: true, Table: "ifXTable", MetricType: ddprofiledefinition.ProfileMetricTypeGauge},
				{Name: "ifHCInOctets", Value: 2000, IsTable: true, Table: "ifXTable"},
				{Name: "ifHCInOctets", Value: 3000, IsTable: true, Table: "ifXTable"},
			},
			expected: []ddsnmp.Metric{
				{
					Name:        "ifTotalInOctets",
					Value:       6000,
					Description: "Total inbound traffic",
					Family:      "Network/Total/Traffic/In",
					Unit:        "bit/s",
					MetricType:  ddprofiledefinition.ProfileMetricTypeGauge,
				},
			},
		},

		"sum from multiple sources": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				Metrics: []ddprofiledefinition.MetricsConfig{
					{
						Table: ddprofiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.2.2",
							Name: "ifTable",
						},
						Symbols: []ddprofiledefinition.SymbolConfig{
							{OID: "1.3.6.1.2.1.2.2.1.14", Name: "ifInErrors"},
							{OID: "1.3.6.1.2.1.2.2.1.20", Name: "ifOutErrors"},
						},
					},
				},
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name: "ifTotalErrors",
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ifInErrors", Table: "ifTable"},
							{Metric: "ifOutErrors", Table: "ifTable"},
						},
						ChartMeta: ddprofiledefinition.ChartMeta{
							Description: "Total errors",
							Family:      "Network/Total/Errors",
							Unit:        "{error}/s",
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				// Interface 1
				{Name: "ifInErrors", Value: 10, IsTable: true, Table: "ifTable", MetricType: ddprofiledefinition.ProfileMetricTypeRate},
				{Name: "ifOutErrors", Value: 5, IsTable: true, Table: "ifTable"},
				// Interface 2
				{Name: "ifInErrors", Value: 20, IsTable: true, Table: "ifTable"},
				{Name: "ifOutErrors", Value: 15, IsTable: true, Table: "ifTable"},
			},
			expected: []ddsnmp.Metric{
				{
					Name:        "ifTotalErrors",
					Value:       50, // 10 + 5 + 20 + 15
					Description: "Total errors",
					Family:      "Network/Total/Errors",
					Unit:        "{error}/s",
					MetricType:  ddprofiledefinition.ProfileMetricTypeRate,
				},
			},
		},

		"multiple virtual metrics": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				Metrics: []ddprofiledefinition.MetricsConfig{
					{
						Table: ddprofiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.31.1.1",
							Name: "ifXTable",
						},
						Symbols: []ddprofiledefinition.SymbolConfig{
							{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
							{OID: "1.3.6.1.2.1.31.1.1.1.10", Name: "ifHCOutOctets"},
						},
					},
				},
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name: "ifTotalInOctets",
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ifHCInOctets", Table: "ifXTable"},
						},
						ChartMeta: ddprofiledefinition.ChartMeta{
							Description: "Total in",
							Family:      "Network/In",
							Unit:        "bit/s",
						},
					},
					{
						Name: "ifTotalOutOctets",
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ifHCOutOctets", Table: "ifXTable"},
						},
						ChartMeta: ddprofiledefinition.ChartMeta{
							Description: "Total out",
							Family:      "Network/Out",
							Unit:        "bit/s",
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				{Name: "ifHCInOctets", Value: 1000, IsTable: true, Table: "ifXTable"},
				{Name: "ifHCInOctets", Value: 2000, IsTable: true, Table: "ifXTable"},
				{Name: "ifHCOutOctets", Value: 500, IsTable: true, Table: "ifXTable"},
				{Name: "ifHCOutOctets", Value: 1500, IsTable: true, Table: "ifXTable"},
			},
			expected: []ddsnmp.Metric{
				{
					Name:        "ifTotalInOctets",
					Value:       3000,
					Description: "Total in",
					Family:      "Network/In",
					Unit:        "bit/s",
				},
				{
					Name:        "ifTotalOutOctets",
					Value:       2000,
					Description: "Total out",
					Family:      "Network/Out",
					Unit:        "bit/s",
				},
			},
		},

		"skip missing sources": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name: "totalMissing",
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "nonExistentMetric", Table: "someTable"},
						},
						ChartMeta: ddprofiledefinition.ChartMeta{
							Description: "Should be skipped",
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				{Name: "ifInOctets", Value: 100, IsTable: true, Table: "ifTable"},
			},
			expected: []ddsnmp.Metric{},
		},

		"naming conflict with existing metric": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				Metrics: []ddprofiledefinition.MetricsConfig{
					{
						Symbol: ddprofiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.2.2.1.10",
							Name: "ifInOctets",
						},
					},
				},
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name: "ifInOctets", // Conflicts with existing metric
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ifOutOctets", Table: "ifTable"},
						},
						ChartMeta: ddprofiledefinition.ChartMeta{
							Description: "Should be skipped due to conflict",
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				{Name: "ifInOctets", Value: 100, IsTable: true, Table: "ifTable"},
				{Name: "ifOutOctets", Value: 200, IsTable: true, Table: "ifTable"},
			},
			expected: []ddsnmp.Metric{},
		},

		"source without table name": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name: "invalidVirtual",
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "someMetric", Table: ""}, // Missing table
						},
						ChartMeta: ddprofiledefinition.ChartMeta{
							Description: "Should skip source without table",
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				{Name: "someMetric", Value: 100, IsTable: true, Table: "someTable"},
			},
			expected: []ddsnmp.Metric{},
		},

		"ignore scalar metrics": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name: "totalOctets",
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ifHCInOctets", Table: "ifXTable"},
						},
						ChartMeta: ddprofiledefinition.ChartMeta{
							Description: "Total octets",
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				{Name: "ifHCInOctets", Value: 1000, IsTable: true, Table: "ifXTable"},
				{Name: "ifHCInOctets", Value: 2000, IsTable: false, Table: ""}, // Scalar, should be ignored
				{Name: "ifHCInOctets", Value: 3000, IsTable: true, Table: "ifXTable"},
			},
			expected: []ddsnmp.Metric{
				{
					Name:        "totalOctets",
					Value:       4000, // Only 1000 + 3000
					Description: "Total octets",
				},
			},
		},

		"metrics from different tables": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				Metrics: []ddprofiledefinition.MetricsConfig{
					{
						Table: ddprofiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.2.2",
							Name: "ifTable",
						},
						Symbols: []ddprofiledefinition.SymbolConfig{
							{OID: "1.3.6.1.2.1.2.2.1.10", Name: "ifInOctets"},
						},
					},
					{
						Table: ddprofiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.31.1.1",
							Name: "ifXTable",
						},
						Symbols: []ddprofiledefinition.SymbolConfig{
							{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
						},
					},
				},
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name: "totalTraffic",
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ifInOctets", Table: "ifTable"},
							{Metric: "ifHCInOctets", Table: "ifXTable"},
						},
						ChartMeta: ddprofiledefinition.ChartMeta{
							Description: "Total traffic from both tables",
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				{Name: "ifInOctets", Value: 100, IsTable: true, Table: "ifTable"},
				{Name: "ifInOctets", Value: 200, IsTable: true, Table: "ifTable"},
				{Name: "ifHCInOctets", Value: 1000, IsTable: true, Table: "ifXTable"},
				{Name: "ifHCInOctets", Value: 2000, IsTable: true, Table: "ifXTable"},
			},
			expected: []ddsnmp.Metric{
				{
					Name:        "totalTraffic",
					Value:       3300, // 100 + 200 + 1000 + 2000
					Description: "Total traffic from both tables",
				},
			},
		},

		"empty config": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{},
			},
			collectedMetrics: []ddsnmp.Metric{
				{Name: "ifInOctets", Value: 100, IsTable: true, Table: "ifTable"},
			},
			expected: nil,
		},

		"no collected metrics": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name: "totalOctets",
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ifInOctets", Table: "ifTable"},
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{},
			expected:         []ddsnmp.Metric{},
		},

		"large scale test": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name: "totalInterfaceTraffic",
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ifHCInOctets", Table: "ifXTable"},
						},
						ChartMeta: ddprofiledefinition.ChartMeta{
							Description: "Total traffic across many interfaces",
						},
					},
				},
			},
			collectedMetrics: func() []ddsnmp.Metric {
				// Simulate 1000 interfaces
				metrics := make([]ddsnmp.Metric, 0, 1000)
				for i := 0; i < 1000; i++ {
					metrics = append(metrics, ddsnmp.Metric{
						Name:    "ifHCInOctets",
						Value:   int64(i * 100),
						IsTable: true,
						Table:   "ifXTable",
					})
				}
				return metrics
			}(),
			expected: []ddsnmp.Metric{
				{
					Name:        "totalInterfaceTraffic",
					Value:       49950000, // Sum of 0*100 + 1*100 + ... + 999*100
					Description: "Total traffic across many interfaces",
				},
			},
		},

		"zero values included": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name: "totalWithZeros",
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "someMetric", Table: "someTable"},
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				{Name: "someMetric", Value: 0, IsTable: true, Table: "someTable"},
				{Name: "someMetric", Value: 100, IsTable: true, Table: "someTable"},
				{Name: "someMetric", Value: 0, IsTable: true, Table: "someTable"},
			},
			expected: []ddsnmp.Metric{
				{
					Name:  "totalWithZeros",
					Value: 100,
				},
			},
		},

		"negative values": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name: "totalWithNegatives",
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "someMetric", Table: "someTable"},
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				{Name: "someMetric", Value: 100, IsTable: true, Table: "someTable"},
				{Name: "someMetric", Value: -50, IsTable: true, Table: "someTable"},
				{Name: "someMetric", Value: 200, IsTable: true, Table: "someTable"},
			},
			expected: []ddsnmp.Metric{
				{
					Name:  "totalWithNegatives",
					Value: 250, // 100 - 50 + 200
				},
			},
		},

		"aggregate MultiValue metrics": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				Metrics: []ddprofiledefinition.MetricsConfig{
					{
						Table: ddprofiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.2.2",
							Name: "ifTable",
						},
						Symbols: []ddprofiledefinition.SymbolConfig{
							{
								OID:  "1.3.6.1.2.1.2.2.1.8",
								Name: "ifOperStatus",
								Mapping: map[string]string{
									"1": "up",
									"2": "down",
									"3": "testing",
									"4": "unknown",
									"5": "dormant",
								},
							},
						},
					},
				},
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name: "ifTotalOperStatus",
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ifOperStatus", Table: "ifTable"},
						},
						ChartMeta: ddprofiledefinition.ChartMeta{
							Description: "Total interface operational status",
							Family:      "Network/Total/Interface/Status",
							Unit:        "{status}",
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				// Interface 1 - up
				{
					Name:    "ifOperStatus",
					Value:   1,
					IsTable: true,
					Table:   "ifTable",
					MultiValue: map[string]int64{
						"up":      1,
						"down":    0,
						"testing": 0,
						"unknown": 0,
						"dormant": 0,
					},
					MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
				},
				// Interface 2 - down
				{
					Name:    "ifOperStatus",
					Value:   2,
					IsTable: true,
					Table:   "ifTable",
					MultiValue: map[string]int64{
						"up":      0,
						"down":    1,
						"testing": 0,
						"unknown": 0,
						"dormant": 0,
					},
				},
				// Interface 3 - up
				{
					Name:    "ifOperStatus",
					Value:   1,
					IsTable: true,
					Table:   "ifTable",
					MultiValue: map[string]int64{
						"up":      1,
						"down":    0,
						"testing": 0,
						"unknown": 0,
						"dormant": 0,
					},
				},
				// Interface 4 - testing
				{
					Name:    "ifOperStatus",
					Value:   3,
					IsTable: true,
					Table:   "ifTable",
					MultiValue: map[string]int64{
						"up":      0,
						"down":    0,
						"testing": 1,
						"unknown": 0,
						"dormant": 0,
					},
				},
			},
			expected: []ddsnmp.Metric{
				{
					Name:  "ifTotalOperStatus",
					Value: 0, // Not used for MultiValue
					MultiValue: map[string]int64{
						"up":      2, // 2 interfaces up
						"down":    1, // 1 interface down
						"testing": 1, // 1 interface testing
						"unknown": 0,
						"dormant": 0,
					},
					Description: "Total interface operational status",
					Family:      "Network/Total/Interface/Status",
					Unit:        "{status}",
					MetricType:  ddprofiledefinition.ProfileMetricTypeGauge,
				},
			},
		},

		"mixed MultiValue and regular metrics": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				Metrics: []ddprofiledefinition.MetricsConfig{
					{
						Table: ddprofiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.2.2",
							Name: "ifTable",
						},
						Symbols: []ddprofiledefinition.SymbolConfig{
							{
								OID:  "1.3.6.1.2.1.2.2.1.10",
								Name: "ifInOctets",
							},
							{
								OID:  "1.3.6.1.2.1.2.2.1.8",
								Name: "ifOperStatus",
								Mapping: map[string]string{
									"1": "up",
									"2": "down",
								},
							},
						},
					},
				},
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name: "ifTotalInOctets",
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ifInOctets", Table: "ifTable"},
						},
						ChartMeta: ddprofiledefinition.ChartMeta{
							Description: "Total inbound traffic",
							Family:      "Network/Total/Traffic",
							Unit:        "bit/s",
						},
					},
					{
						Name: "ifTotalOperStatus",
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ifOperStatus", Table: "ifTable"},
						},
						ChartMeta: ddprofiledefinition.ChartMeta{
							Description: "Total interface status",
							Family:      "Network/Total/Status",
							Unit:        "{status}",
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				// Regular metric
				{Name: "ifInOctets", Value: 1000, IsTable: true, Table: "ifTable"},
				{Name: "ifInOctets", Value: 2000, IsTable: true, Table: "ifTable"},
				// MultiValue metric
				{
					Name:       "ifOperStatus",
					Value:      1,
					IsTable:    true,
					Table:      "ifTable",
					MultiValue: map[string]int64{"up": 1, "down": 0},
				},
				{
					Name:       "ifOperStatus",
					Value:      2,
					IsTable:    true,
					Table:      "ifTable",
					MultiValue: map[string]int64{"up": 0, "down": 1},
				},
			},
			expected: []ddsnmp.Metric{
				{
					Name:        "ifTotalInOctets",
					Value:       3000,
					Description: "Total inbound traffic",
					Family:      "Network/Total/Traffic",
					Unit:        "bit/s",
				},
				{
					Name:        "ifTotalOperStatus",
					Value:       0,
					MultiValue:  map[string]int64{"up": 1, "down": 1},
					Description: "Total interface status",
					Family:      "Network/Total/Status",
					Unit:        "{status}",
				},
			},
		},

		"composite with as (two table sources)": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				Metrics: []ddprofiledefinition.MetricsConfig{
					{
						Table: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.31.1.1", Name: "ifXTable"},
						Symbols: []ddprofiledefinition.SymbolConfig{
							{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
							{OID: "1.3.6.1.2.1.31.1.1.1.10", Name: "ifHCOutOctets"},
						},
					},
				},
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name: "ifTotalTraffic",
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ifHCInOctets", Table: "ifXTable", As: "in"},
							{Metric: "ifHCOutOctets", Table: "ifXTable", As: "out"},
						},
						ChartMeta: ddprofiledefinition.ChartMeta{
							Description: "Total traffic by direction",
							Family:      "Network/Total/Traffic",
							Unit:        "bit/s",
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				{Name: "ifHCInOctets", Value: 1000, IsTable: true, Table: "ifXTable"},
				{Name: "ifHCInOctets", Value: 2000, IsTable: true, Table: "ifXTable"},
				{Name: "ifHCOutOctets", Value: 500, IsTable: true, Table: "ifXTable"},
				{Name: "ifHCOutOctets", Value: 1500, IsTable: true, Table: "ifXTable"},
			},
			expected: []ddsnmp.Metric{
				{
					Name:        "ifTotalTraffic",
					MultiValue:  map[string]int64{"in": 3000, "out": 2000},
					Description: "Total traffic by direction",
					Family:      "Network/Total/Traffic",
					Unit:        "bit/s",
				},
			},
		},

		"composite with as (missing one source)": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name: "ifTotalTraffic",
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ifHCInOctets", Table: "ifXTable", As: "in"},
							{Metric: "ifHCOutOctets", Table: "ifXTable", As: "out"},
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				// Only IN is present
				{Name: "ifHCInOctets", Value: 1000, IsTable: true, Table: "ifXTable"},
				{Name: "ifHCInOctets", Value: 2000, IsTable: true, Table: "ifXTable"},
			},
			expected: []ddsnmp.Metric{
				{
					Name:       "ifTotalTraffic",
					MultiValue: map[string]int64{"in": 3000}, // "out" omitted
				},
			},
		},

		"composite with duplicate 'as' merges values": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				Metrics: []ddprofiledefinition.MetricsConfig{
					{
						Table: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.2.2", Name: "ifTable"},
						Symbols: []ddprofiledefinition.SymbolConfig{
							{OID: "1.3.6.1.2.1.2.2.1.10", Name: "ifInOctets"},
						},
					},
					{
						Table: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.31.1.1", Name: "ifXTable"},
						Symbols: []ddprofiledefinition.SymbolConfig{
							{OID: "1.3.6.1.2.1.31.1.1.1.6", Name: "ifHCInOctets"},
						},
					},
				},
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name: "ifTotalInbound",
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ifInOctets", Table: "ifTable", As: "in"},
							{Metric: "ifHCInOctets", Table: "ifXTable", As: "in"}, // same 'as'
						},
						ChartMeta: ddprofiledefinition.ChartMeta{
							Description: "Inbound traffic from two tables merged",
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				{Name: "ifInOctets", Value: 100, IsTable: true, Table: "ifTable"},
				{Name: "ifInOctets", Value: 200, IsTable: true, Table: "ifTable"},
				{Name: "ifHCInOctets", Value: 1000, IsTable: true, Table: "ifXTable"},
				{Name: "ifHCInOctets", Value: 2000, IsTable: true, Table: "ifXTable"},
			},
			expected: []ddsnmp.Metric{
				{
					Name:        "ifTotalInbound",
					MultiValue:  map[string]int64{"in": 3300}, // 100+200+1000+2000
					Description: "Inbound traffic from two tables merged",
				},
			},
		},

		"composite with scalar sources (CPU)": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name: "cpu_usage",
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ucd.ssCpuUser", Table: "", As: "user"},
							{Metric: "ucd.ssCpuSystem", Table: "", As: "system"},
						},
						ChartMeta: ddprofiledefinition.ChartMeta{
							Description: "CPU usage breakdown",
							Family:      "System/CPU/Usage",
							Unit:        "%",
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				{Name: "ucd.ssCpuUser", Value: 12, IsTable: false, Table: ""},
				{Name: "ucd.ssCpuSystem", Value: 5, IsTable: false, Table: ""},
			},
			expected: []ddsnmp.Metric{
				{
					Name:        "cpu_usage",
					MultiValue:  map[string]int64{"user": 12, "system": 5},
					Description: "CPU usage breakdown",
					Family:      "System/CPU/Usage",
					Unit:        "%",
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			vmc := newVirtualMetricsCollector(logger.New())
			result := vmc.Collect(tc.profileDef, tc.collectedMetrics)

			// Sort both slices for consistent comparison
			assert.ElementsMatch(t, tc.expected, result)
		})
	}
}
