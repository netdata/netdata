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

		"group_by explicit labels (interface,ifType) merges duplicates": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name:    "ifTrafficPerInterface",
						GroupBy: []string{"interface", "ifType"},
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ifHCInOctets", Table: "ifXTable", As: "in"},
							{Metric: "ifHCOutOctets", Table: "ifXTable", As: "out"},
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				// two rows that share the same (interface,ifType) -> should be summed within the group
				{Name: "ifHCInOctets", Value: 0, IsTable: true, Table: "ifXTable",
					Tags: map[string]string{"interface": "bond0", "ifType": "ethernetCsmacd", "ifIndex": "10"}},
				{Name: "ifHCOutOctets", Value: 50, IsTable: true, Table: "ifXTable",
					Tags: map[string]string{"interface": "bond0", "ifType": "ethernetCsmacd", "ifIndex": "10"}},
				{Name: "ifHCInOctets", Value: 0, IsTable: true, Table: "ifXTable",
					Tags: map[string]string{"interface": "bond0", "ifType": "ethernetCsmacd", "ifIndex": "42"}},
				{Name: "ifHCOutOctets", Value: 20, IsTable: true, Table: "ifXTable",
					Tags: map[string]string{"interface": "bond0", "ifType": "ethernetCsmacd", "ifIndex": "42"}},
			},
			expected: []ddsnmp.Metric{
				{
					Name:       "ifTrafficPerInterface",
					IsTable:    true,
					Table:      "ifXTable",
					Tags:       map[string]string{"interface": "bond0", "ifType": "ethernetCsmacd"}, // only group_by labels
					MultiValue: map[string]int64{"in": 0, "out": 70},
				},
			},
		},

		"group_by single-source per interface (Value path)": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name:    "ifErrorsPerInterface",
						GroupBy: []string{"interface"},
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ifInErrors", Table: "ifTable"},
						},
						ChartMeta: ddprofiledefinition.ChartMeta{
							Description: "Per-interface inbound errors",
							Family:      "Network/Interface/Errors",
							Unit:        "{error}/s",
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				{Name: "ifInErrors", Value: 5, IsTable: true, Table: "ifTable",
					Tags: map[string]string{"interface": "eth0", "ifIndex": "1"}},
				{Name: "ifInErrors", Value: 7, IsTable: true, Table: "ifTable",
					Tags: map[string]string{"interface": "eth1", "ifIndex": "2"}},
				{Name: "ifInErrors", Value: 3, IsTable: true, Table: "ifTable",
					Tags: map[string]string{"interface": "eth0", "ifIndex": "1"}},
			},
			expected: []ddsnmp.Metric{
				{
					Name:        "ifErrorsPerInterface",
					IsTable:     true,
					Table:       "ifTable",
					Tags:        map[string]string{"interface": "eth0"},
					Value:       8, // 5 + 3
					Description: "Per-interface inbound errors",
					Family:      "Network/Interface/Errors",
					Unit:        "{error}/s",
				},
				{
					Name:        "ifErrorsPerInterface",
					IsTable:     true,
					Table:       "ifTable",
					Tags:        map[string]string{"interface": "eth1"},
					Value:       7,
					Description: "Per-interface inbound errors",
					Family:      "Network/Interface/Errors",
					Unit:        "{error}/s",
				},
			},
		},

		"group_by per-row with missing dim (partial)": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name:   "ifTrafficPerRow",
						PerRow: true,
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ifHCInOctets", Table: "ifXTable", As: "in"},
							{Metric: "ifHCOutOctets", Table: "ifXTable", As: "out"},
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				// eth0 has only IN
				{Name: "ifHCInOctets", Value: 111, IsTable: true, Table: "ifXTable",
					Tags: map[string]string{"interface": "eth0", "ifType": "ethernetCsmacd", "ifIndex": "1"}},
				// eth1 has only OUT
				{Name: "ifHCOutOctets", Value: 222, IsTable: true, Table: "ifXTable",
					Tags: map[string]string{"interface": "eth1", "ifType": "ethernetCsmacd", "ifIndex": "2"}},
			},
			expected: []ddsnmp.Metric{
				{
					Name:       "ifTrafficPerRow",
					IsTable:    true,
					Table:      "ifXTable",
					Tags:       map[string]string{"interface": "eth0", "ifType": "ethernetCsmacd", "ifIndex": "1"},
					MultiValue: map[string]int64{"in": 111}, // out omitted
				},
				{
					Name:       "ifTrafficPerRow",
					IsTable:    true,
					Table:      "ifXTable",
					Tags:       map[string]string{"interface": "eth1", "ifType": "ethernetCsmacd", "ifIndex": "2"},
					MultiValue: map[string]int64{"out": 222},
				},
			},
		},

		"group_by explicit labels but sources span tables (skipped VM)": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name:    "invalidGroupedVM",
						GroupBy: []string{"interface"},
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ifInOctets", Table: "ifTable", As: "in"},
							{Metric: "ifHCInOctets", Table: "ifXTable", As: "in2"},
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				{Name: "ifInOctets", Value: 100, IsTable: true, Table: "ifTable",
					Tags: map[string]string{"interface": "eth0", "ifIndex": "1"}},
				{Name: "ifHCInOctets", Value: 200, IsTable: true, Table: "ifXTable",
					Tags: map[string]string{"interface": "eth0", "ifIndex": "1"}},
			},
			expected: []ddsnmp.Metric{}, // VM skipped during build phase
		},

		"group_by per-row without index/ifIndex (fallback key)": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name:   "ifTrafficPerRow",
						PerRow: true,
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ifHCInOctets", Table: "ifXTable", As: "in"},
							{Metric: "ifHCOutOctets", Table: "ifXTable", As: "out"},
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				// tags without index/ifIndex; fallback should still build a stable key
				{Name: "ifHCInOctets", Value: 5, IsTable: true, Table: "ifXTable",
					Tags: map[string]string{"interface": "ethA", "ifType": "ethernetCsmacd"}},
				{Name: "ifHCOutOctets", Value: 7, IsTable: true, Table: "ifXTable",
					Tags: map[string]string{"interface": "ethA", "ifType": "ethernetCsmacd"}},

				{Name: "ifHCInOctets", Value: 1, IsTable: true, Table: "ifXTable",
					Tags: map[string]string{"interface": "ethB", "ifType": "ethernetCsmacd"}},
				{Name: "ifHCOutOctets", Value: 2, IsTable: true, Table: "ifXTable",
					Tags: map[string]string{"interface": "ethB", "ifType": "ethernetCsmacd"}},
			},
			expected: []ddsnmp.Metric{
				{
					Name:       "ifTrafficPerRow",
					IsTable:    true,
					Table:      "ifXTable",
					Tags:       map[string]string{"interface": "ethA", "ifType": "ethernetCsmacd"},
					MultiValue: map[string]int64{"in": 5, "out": 7},
				},
				{
					Name:       "ifTrafficPerRow",
					IsTable:    true,
					Table:      "ifXTable",
					Tags:       map[string]string{"interface": "ethB", "ifType": "ethernetCsmacd"},
					MultiValue: map[string]int64{"in": 1, "out": 2},
				},
			},
		},

		"per_row composite (in/out)": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name:   "ifTrafficPerRow",
						PerRow: true,
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ifHCInOctets", Table: "ifXTable", As: "in"},
							{Metric: "ifHCOutOctets", Table: "ifXTable", As: "out"},
						},
						ChartMeta: ddprofiledefinition.ChartMeta{
							Description: "Per-row traffic (in/out)",
							Family:      "Network/Interface/Traffic",
							Unit:        "bit/s",
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				// eth0
				{Name: "ifHCInOctets", Value: 1000, IsTable: true, Table: "ifXTable",
					Tags: map[string]string{"interface": "eth0", "ifType": "ethernetCsmacd", "ifIndex": "1"}},
				{Name: "ifHCOutOctets", Value: 2000, IsTable: true, Table: "ifXTable",
					Tags: map[string]string{"interface": "eth0", "ifType": "ethernetCsmacd", "ifIndex": "1"}},
				// lo
				{Name: "ifHCInOctets", Value: 10, IsTable: true, Table: "ifXTable",
					Tags: map[string]string{"interface": "lo", "ifType": "softwareLoopback", "ifIndex": "2"}},
				{Name: "ifHCOutOctets", Value: 15, IsTable: true, Table: "ifXTable",
					Tags: map[string]string{"interface": "lo", "ifType": "softwareLoopback", "ifIndex": "2"}},
			},
			expected: []ddsnmp.Metric{
				{
					Name:        "ifTrafficPerRow",
					IsTable:     true,
					Table:       "ifXTable",
					Tags:        map[string]string{"interface": "eth0", "ifType": "ethernetCsmacd", "ifIndex": "1"},
					MultiValue:  map[string]int64{"in": 1000, "out": 2000},
					Description: "Per-row traffic (in/out)",
					Family:      "Network/Interface/Traffic",
					Unit:        "bit/s",
				},
				{
					Name:        "ifTrafficPerRow",
					IsTable:     true,
					Table:       "ifXTable",
					Tags:        map[string]string{"interface": "lo", "ifType": "softwareLoopback", "ifIndex": "2"},
					MultiValue:  map[string]int64{"in": 10, "out": 15},
					Description: "Per-row traffic (in/out)",
					Family:      "Network/Interface/Traffic",
					Unit:        "bit/s",
				},
			},
		},

		"per_row with key hints (group_by used as row key)": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name:    "ifTrafficPerRowHint",
						PerRow:  true,
						GroupBy: []string{"interface"}, // used as row-key hint
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ifHCInOctets", Table: "ifXTable", As: "in"},
							{Metric: "ifHCOutOctets", Table: "ifXTable", As: "out"},
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				{Name: "ifHCInOctets", Value: 5, IsTable: true, Table: "ifXTable",
					Tags: map[string]string{"interface": "ethA", "ifType": "ethernetCsmacd", "ifIndex": "11"}},
				{Name: "ifHCOutOctets", Value: 7, IsTable: true, Table: "ifXTable",
					Tags: map[string]string{"interface": "ethA", "ifType": "ethernetCsmacd", "ifIndex": "11"}},
				{Name: "ifHCInOctets", Value: 1, IsTable: true, Table: "ifXTable",
					Tags: map[string]string{"interface": "ethB", "ifType": "ethernetCsmacd", "ifIndex": "12"}},
				{Name: "ifHCOutOctets", Value: 2, IsTable: true, Table: "ifXTable",
					Tags: map[string]string{"interface": "ethB", "ifType": "ethernetCsmacd", "ifIndex": "12"}},
			},
			expected: []ddsnmp.Metric{
				{
					Name:       "ifTrafficPerRowHint",
					IsTable:    true,
					Table:      "ifXTable",
					Tags:       map[string]string{"interface": "ethA", "ifType": "ethernetCsmacd", "ifIndex": "11"},
					MultiValue: map[string]int64{"in": 5, "out": 7},
				},
				{
					Name:       "ifTrafficPerRowHint",
					IsTable:    true,
					Table:      "ifXTable",
					Tags:       map[string]string{"interface": "ethB", "ifType": "ethernetCsmacd", "ifIndex": "12"},
					MultiValue: map[string]int64{"in": 1, "out": 2},
				},
			},
		},

		"per_row hint missing for a row (fallback to full-tag key)": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name:    "ifTrafficPerRowFallback",
						PerRow:  true,
						GroupBy: []string{"interface"}, // hint missing on one row
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ifHCInOctets", Table: "ifXTable", As: "in"},
							{Metric: "ifHCOutOctets", Table: "ifXTable", As: "out"},
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				// has 'interface'
				{Name: "ifHCInOctets", Value: 10, IsTable: true, Table: "ifXTable",
					Tags: map[string]string{"interface": "eth0", "ifIndex": "1"}},
				{Name: "ifHCOutOctets", Value: 20, IsTable: true, Table: "ifXTable",
					Tags: map[string]string{"interface": "eth0", "ifIndex": "1"}},
				// missing 'interface' -> falls back to full-tag key
				{Name: "ifHCInOctets", Value: 30, IsTable: true, Table: "ifXTable",
					Tags: map[string]string{"name": "weird0", "ifIndex": "9"}},
				{Name: "ifHCOutOctets", Value: 40, IsTable: true, Table: "ifXTable",
					Tags: map[string]string{"name": "weird0", "ifIndex": "9"}},
			},
			expected: []ddsnmp.Metric{
				{
					Name:       "ifTrafficPerRowFallback",
					IsTable:    true,
					Table:      "ifXTable",
					Tags:       map[string]string{"interface": "eth0", "ifIndex": "1"},
					MultiValue: map[string]int64{"in": 10, "out": 20},
				},
				{
					Name:       "ifTrafficPerRowFallback",
					IsTable:    true,
					Table:      "ifXTable",
					Tags:       map[string]string{"name": "weird0", "ifIndex": "9"},
					MultiValue: map[string]int64{"in": 30, "out": 40},
				},
			},
		},

		"per_row single-source (value path)": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name:   "ifInErrorsPerRow",
						PerRow: true,
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ifInErrors", Table: "ifTable"},
						},
						ChartMeta: ddprofiledefinition.ChartMeta{
							Description: "Per-row inbound errors",
							Family:      "Network/Interface/Errors",
							Unit:        "{error}/s",
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				{Name: "ifInErrors", Value: 5, IsTable: true, Table: "ifTable",
					Tags: map[string]string{"interface": "eth0", "ifIndex": "1"}},
				{Name: "ifInErrors", Value: 7, IsTable: true, Table: "ifTable",
					Tags: map[string]string{"interface": "eth1", "ifIndex": "2"}},
			},
			expected: []ddsnmp.Metric{
				{
					Name:        "ifInErrorsPerRow",
					IsTable:     true,
					Table:       "ifTable",
					Tags:        map[string]string{"interface": "eth0", "ifIndex": "1"},
					Value:       5,
					Description: "Per-row inbound errors",
					Family:      "Network/Interface/Errors",
					Unit:        "{error}/s",
				},
				{
					Name:        "ifInErrorsPerRow",
					IsTable:     true,
					Table:       "ifTable",
					Tags:        map[string]string{"interface": "eth1", "ifIndex": "2"},
					Value:       7,
					Description: "Per-row inbound errors",
					Family:      "Network/Interface/Errors",
					Unit:        "{error}/s",
				},
			},
		},

		"per_row sources in different tables (skipped VM)": {
			profileDef: &ddprofiledefinition.ProfileDefinition{
				VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
					{
						Name:   "invalidPerRow",
						PerRow: true,
						Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
							{Metric: "ifHCInOctets", Table: "ifXTable", As: "in"},
							{Metric: "ifInOctets", Table: "ifTable", As: "in2"},
						},
					},
				},
			},
			collectedMetrics: []ddsnmp.Metric{
				{Name: "ifHCInOctets", Value: 1, IsTable: true, Table: "ifXTable",
					Tags: map[string]string{"interface": "eth0", "ifIndex": "1"}},
				{Name: "ifInOctets", Value: 2, IsTable: true, Table: "ifTable",
					Tags: map[string]string{"interface": "eth0", "ifIndex": "1"}},
			},
			expected: []ddsnmp.Metric{}, // VM skipped in build phase
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
