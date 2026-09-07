// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

type normalBenchmarkHandler struct {
	gosnmp.Handler
	rows int
}

func (*normalBenchmarkHandler) Version() gosnmp.SnmpVersion { return gosnmp.Version2c }
func (*normalBenchmarkHandler) MaxOids() int                { return 32 }
func (*normalBenchmarkHandler) Get(oids []string) (*gosnmp.SnmpPacket, error) {
	packet := &gosnmp.SnmpPacket{Variables: make([]gosnmp.SnmpPDU, len(oids))}
	for i, oid := range oids {
		value := 7
		if oid == normalTestRoot+".3.0" {
			value = 0
		}
		if oid == normalTestRoot+".4.0" {
			value = 3600
		}
		if oid == normalTestRoot+".5.0" {
			value = 6
		}
		packet.Variables[i] = gosnmp.SnmpPDU{Name: oid, Type: gosnmp.Integer, Value: value}
	}
	return packet, nil
}
func (h *normalBenchmarkHandler) BulkWalkAll(root string) ([]gosnmp.SnmpPDU, error) {
	values := make([]gosnmp.SnmpPDU, h.rows)
	for i := range values {
		values[i] = gosnmp.SnmpPDU{Name: root + ".1." + strconv.Itoa(i+1), Type: gosnmp.Counter64, Value: uint64(7)}
	}
	return values, nil
}

// Both modes run the real acquisition, normalization, chart/sample and consumer
// cache pipeline. The baseline omits only passive diagnostic capture.
func BenchmarkNormalWarmCollection(b *testing.B) {
	for name, shape := range map[string]struct{ tables, rows int }{
		"small":     {1, 32},
		"many_rows": {16, 256},
	} {
		for mode, record := range map[string]bool{"baseline": false, "capture": true} {
			b.Run(name+"/"+mode, func(b *testing.B) {
				profile := normalTestProfile()
				for i := range shape.tables {
					root := fmt.Sprintf("1.3.6.1.4.1.99999.200.%d", i+1)
					profile.Definition.Metrics = append(profile.Definition.Metrics, ddprofiledefinition.MetricsConfig{
						Table:      ddprofiledefinition.SymbolConfig{OID: root, Name: fmt.Sprintf("table%d", i)},
						Symbols:    []ddprofiledefinition.SymbolConfig{{OID: root + ".1", Name: fmt.Sprintf("metric%d", i)}},
						MetricTags: []ddprofiledefinition.MetricTagConfig{{Tag: "index", Index: 1}},
					})
				}
				c := New(ddsnmp.NewDeviceStore())
				c.Config = prepareV2Config()
				c.sysInfo = &snmputils.SysInfo{Name: "benchmark"}
				c.snmpClient = &normalBenchmarkHandler{rows: shape.rows}
				cfg := ddsnmpcollector.Config{SnmpClient: c.snmpClient, Profiles: []*ddsnmp.Profile{profile}, Log: c.Logger}
				if record {
					cfg.AcquisitionObserver = ddsnmpcollector.AcquisitionObserverFunc(c.observeNormalProfile)
				}
				c.ddSnmpColl = ddsnmpcollector.New(cfg)
				collect := func() {
					if record {
						c.beginNormalAttempt("collect")
					}
					mx := make(map[string]int64)
					err := c.collectSNMP(mx)
					if record {
						c.finishNormalAttempt(err, mx)
					}
					if err != nil {
						b.Fatal(err)
					}
					if len(mx) < shape.tables*shape.rows {
						b.Fatalf("missing table samples: %d", len(mx))
					}
				}
				collect()
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					collect()
				}
				b.StopTimer()
				if record {
					d, err := c.normal.cut.CaptureNormal()
					if err != nil {
						b.Fatal(err)
					}
					var retainedPDUs int
					for _, source := range d.Sources {
						if strings.Contains(source.Method, "walk") {
							retainedPDUs += len(source.PDUs)
						}
					}
					b.ReportMetric(float64(retainedPDUs), "retained_walk_pdus")
				}
			})
		}
	}
}
