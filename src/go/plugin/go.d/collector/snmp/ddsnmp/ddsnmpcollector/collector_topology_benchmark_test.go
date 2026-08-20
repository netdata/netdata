// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func BenchmarkTableCollector_OrganizeRowsByCacheEligibility(b *testing.B) {
	const (
		tableOID  = "1.3.6.1.4.1.99999.90"
		columnOID = tableOID + ".1"
	)
	config := ddprofiledefinition.MetricsConfig{
		Table:   ddprofiledefinition.SymbolConfig{OID: tableOID, Name: "allocationTable"},
		Symbols: []ddprofiledefinition.SymbolConfig{{OID: columnOID, Name: "value"}},
	}
	collector := newTableCollector(nil, make(map[string]bool), nil, logger.New(), false)

	for _, rowCount := range []int{256, 4096} {
		pdus := make(map[string]gosnmp.SnmpPDU, rowCount)
		for i := range rowCount {
			rowOID := fmt.Sprintf("%s.%d", columnOID, i+1)
			pdus[rowOID] = createGauge32PDU(rowOID, uint(i+1))
		}
		for _, cacheStructure := range []bool{false, true} {
			name := "ineligible"
			if cacheStructure {
				name = "ordinary-value"
			}
			b.Run(fmt.Sprintf("%s/rows=%d", name, rowCount), func(b *testing.B) {
				ctx := &tableProcessingContext{
					config:         config,
					pdus:           pdus,
					columnOIDs:     buildColumnOIDs(config),
					orderedTags:    buildOrderedTags(config),
					cacheStructure: cacheStructure,
				}
				b.ReportAllocs()
				b.ReportMetric(float64(rowCount), "rows/op")
				b.ResetTimer()
				for range b.N {
					rows, oidCache, tagCache := collector.organizePDUsByRow(ctx)
					if len(rows) != rowCount {
						b.Fatalf("expected %d rows, got %d", rowCount, len(rows))
					}
					runtime.KeepAlive(oidCache)
					runtime.KeepAlive(tagCache)
				}
			})
		}
	}
}

func BenchmarkTableRowProcessor_IPTopologyPresence(b *testing.B) {
	const (
		ifIndexOID = "1.3.6.1.2.1.4.20.1.2"
		netmaskOID = "1.3.6.1.2.1.4.20.1.3"
	)

	for _, rowCount := range []int{1, 256, 4096} {
		b.Run(fmt.Sprintf("rows=%d", rowCount), func(b *testing.B) {
			processor := newTableRowProcessor(logger.New())
			rows := make([]tableRowData, rowCount)
			for i := range rows {
				index := fmt.Sprintf("198.18.%d.%d", i/254, i%254+1)
				rows[i] = tableRowData{
					index: index,
					pdus: map[string]gosnmp.SnmpPDU{
						ifIndexOID: createIntegerPDU(ifIndexOID+"."+index, i+1),
						netmaskOID: createPDU(netmaskOID+"."+index, gosnmp.IPAddress, "255.255.255.0"),
					},
					staticTags: map[string]string{},
					tableName:  "ipAddrTable",
				}
			}

			ctx := &tableRowProcessingContext{
				columnOIDs: map[string][]ddprofiledefinition.SymbolConfig{
					ifIndexOID: {{OID: ifIndexOID, Name: "ip_if_index"}},
				},
				orderedTags: []orderedTagConfig{
					{
						tagType: tagTypeIndex,
						config: ddprofiledefinition.MetricTagConfig{
							Tag:            "topo_ip_addr",
							Symbol:         ddprofiledefinition.SymbolConfigCompat{Name: "ipAdEntAddr", Format: "ip_address"},
							IndexTransform: []ddprofiledefinition.MetricIndexTransform{{Start: 0, End: 3}},
						},
					},
					{
						tagType: tagTypeSameTable,
						config: ddprofiledefinition.MetricTagConfig{
							Tag:    "topo_if_index",
							Symbol: ddprofiledefinition.SymbolConfigCompat{OID: ifIndexOID, Name: "ipAdEntIfIndex"},
						},
					},
					{
						tagType: tagTypeSameTable,
						config: ddprofiledefinition.MetricTagConfig{
							Tag:    "topo_ip_netmask",
							Symbol: ddprofiledefinition.SymbolConfigCompat{OID: netmaskOID, Name: "ipAdEntNetMask"},
						},
					},
				},
				symbolMode: tableSymbolModePresence,
			}

			b.ReportAllocs()
			b.ReportMetric(float64(rowCount), "rows/op")
			b.ResetTimer()
			for range b.N {
				for i := range rows {
					rows[i].tags = make(map[string]string, 3)
					metrics, err := processor.processRow(&rows[i], ctx)
					if err != nil || len(metrics) != 1 {
						b.Fatalf("processed row %d: metrics=%d err=%v", i, len(metrics), err)
					}
					runtime.KeepAlive(metrics)
				}
			}
		})
	}
}
