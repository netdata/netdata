// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"
	"maps"
	"runtime"
	"testing"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type benchmarkTopologySNMPHandler struct {
	gosnmp.Handler
	pdus []gosnmp.SnmpPDU
}

type benchmarkAcquisitionProjection struct {
	report AcquisitionProfileReport
	values []benchmarkAcquisitionTopologyValue
}

type benchmarkAcquisitionTopologyValue struct {
	kind ddsnmp.TopologyKind
	tags map[string]string
}

func (p *benchmarkAcquisitionProjection) ObserveProfile(
	report AcquisitionProfileReport,
	metrics *ddsnmp.ProfileMetrics,
) {
	p.report = report
	p.values = nil
	if metrics == nil {
		return
	}
	p.values = make([]benchmarkAcquisitionTopologyValue, 0, len(metrics.TopologyMetrics))
	for _, metric := range metrics.TopologyMetrics {
		p.values = append(p.values, benchmarkAcquisitionTopologyValue{
			kind: metric.TopologyKind,
			tags: maps.Clone(metric.Tags),
		})
	}
}

func benchmarkAcquisitionObserverModes() []struct {
	name string
	new  func() AcquisitionObserver
} {
	return []struct {
		name string
		new  func() AcquisitionObserver
	}{
		{name: "nil", new: func() AcquisitionObserver { return nil }},
		{name: "noop", new: func() AcquisitionObserver {
			return AcquisitionObserverFunc(func(AcquisitionProfileReport, *ddsnmp.ProfileMetrics) {})
		}},
		{name: "projection", new: func() AcquisitionObserver { return &benchmarkAcquisitionProjection{} }},
	}
}

func (h *benchmarkTopologySNMPHandler) Version() gosnmp.SnmpVersion {
	return gosnmp.Version2c
}

func (h *benchmarkTopologySNMPHandler) BulkWalkAll(string) ([]gosnmp.SnmpPDU, error) {
	return h.pdus, nil
}

func (h *benchmarkTopologySNMPHandler) MaxOids() int {
	return 4096
}

func benchmarkTopologyProfile(ordinal int) *ddsnmp.Profile {
	tableOID := fmt.Sprintf("1.3.6.1.4.1.99999.%d", ordinal+1)
	return &ddsnmp.Profile{
		SourceFile: fmt.Sprintf("benchmark-topology-%d.yaml", ordinal+1),
		Definition: &ddprofiledefinition.ProfileDefinition{
			Topology: []ddprofiledefinition.TopologyConfig{
				{
					Kind: ddsnmp.KindIfName,
					MetricsConfig: ddprofiledefinition.MetricsConfig{
						Table: ddprofiledefinition.SymbolConfig{
							OID:  tableOID,
							Name: fmt.Sprintf("benchmarkTable%d", ordinal+1),
						},
						Symbols: []ddprofiledefinition.SymbolConfig{
							{
								OID:  tableOID + ".1",
								Name: fmt.Sprintf("benchmark_value_%d", ordinal+1),
							},
						},
					},
				},
			},
		},
	}
}

func BenchmarkCollector_NewTopologyProfiles(b *testing.B) {
	for _, profileCount := range []int{1, 4} {
		profiles := make([]*ddsnmp.Profile, 0, profileCount)
		for i := range profileCount {
			profiles = append(profiles, benchmarkTopologyProfile(i))
		}
		for _, mode := range benchmarkAcquisitionObserverModes()[:2] {
			b.Run(fmt.Sprintf("profiles=%d/observer=%s", profileCount, mode.name), func(b *testing.B) {
				cfg := Config{
					SnmpClient:                 &benchmarkTopologySNMPHandler{},
					Profiles:                   profiles,
					Log:                        logger.New(),
					InitialAcquisitionObserver: mode.new(),
				}
				b.ReportAllocs()
				for b.Loop() {
					collector := New(cfg)
					runtime.KeepAlive(collector)
				}
			})
		}
	}
}

func BenchmarkCollector_InitialCollectTopologyRows(b *testing.B) {
	const columnOID = "1.3.6.1.4.1.99999.1.1"

	for _, rowCount := range []int{0, 256, 4096} {
		pdus := make([]gosnmp.SnmpPDU, 0, rowCount)
		for i := range rowCount {
			oid := fmt.Sprintf("%s.%d", columnOID, i+1)
			pdus = append(pdus, createGauge32PDU(oid, uint(i+1)))
		}
		profile := benchmarkTopologyProfile(0)
		for _, mode := range benchmarkAcquisitionObserverModes() {
			b.Run(fmt.Sprintf("rows=%d/observer=%s", rowCount, mode.name), func(b *testing.B) {
				log := logger.New()
				b.ReportAllocs()
				b.ReportMetric(float64(rowCount), "rows/op")
				for b.Loop() {
					observer := mode.new()
					collector := New(Config{
						SnmpClient:                 &benchmarkTopologySNMPHandler{pdus: pdus},
						Profiles:                   []*ddsnmp.Profile{profile},
						Log:                        log,
						InitialAcquisitionObserver: observer,
					})
					results, err := collector.Collect()
					topologyCount := -1
					if len(results) == 1 {
						topologyCount = len(results[0].TopologyMetrics)
					}
					if err != nil || topologyCount != rowCount {
						b.Fatalf("collection: profiles=%d topology_metrics=%d err=%v", len(results), topologyCount, err)
					}
					runtime.KeepAlive(results)
					runtime.KeepAlive(observer)
				}
			})
		}
	}
}

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
