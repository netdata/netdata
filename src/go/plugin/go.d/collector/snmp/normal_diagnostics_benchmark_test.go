// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/netdata/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

type normalBenchmarkHandler struct {
	snmputils.Session
	rows int
	fail bool
}

func (h *normalBenchmarkHandler) Get(ctx context.Context, oids []string) (*gosnmp.Message, error) {
	if h.fail {
		return nil, errors.New("benchmark timeout")
	}
	packet := &gosnmp.Message{
		VarBinds: make([]gosnmp.VarBind, len(oids)),
	}
	for i, oid := range oids {
		switch oid {
		case snmputils.OidSysObject:
			packet.VarBinds[i] = gosnmp.VarBind{
				Name:  oid,
				Value: gosnmp.ObjectIdentifierValue("1.3.6.1.4.1.99999.1"),
			}
			continue
		case snmputils.OidSysName, snmputils.OidSysDescr, snmputils.OidSysContact, snmputils.OidSysLocation:
			value := ""
			if oid == snmputils.OidSysName {
				value = "benchmark"
			}
			packet.VarBinds[i] = gosnmp.VarBind{
				Name:  oid,
				Value: gosnmp.OctetStringValue([]byte(value)),
			}
			continue
		}
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
		packet.VarBinds[i] = gosnmp.VarBind{
			Name:  oid,
			Value: gosnmp.IntegerValue(int32(value)),
		}
	}
	return packet, nil
}
func (h *normalBenchmarkHandler) BulkWalkAll(ctx context.Context, root string, maxRepetitions uint32) ([]gosnmp.VarBind, gosnmp.WalkEnd, error) {
	values := make([]gosnmp.VarBind, h.rows)
	for i := range values {
		values[i] = gosnmp.VarBind{
			Name:  root + ".1." + strconv.Itoa(i+1),
			Value: gosnmp.Counter64Value(7),
		}
	}
	return values, gosnmp.WalkEnd{}, nil
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
				c := newNormalBenchmarkCollector(shape.tables, shape.rows, record)
				collect := func() {
					if record {
						c.beginNormalAttempt("collect")
					}
					mx := make(map[string]int64)
					err := c.collectSNMP(context.Background(), mx)
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

func newNormalBenchmarkCollector(tables, rows int, record bool) *Collector {
	profile := normalTestProfile()
	for i := range tables {
		root := fmt.Sprintf("1.3.6.1.4.1.99999.200.%d", i+1)
		profile.Definition.Metrics = append(profile.Definition.Metrics, ddprofiledefinition.MetricsConfig{
			Table: ddprofiledefinition.SymbolConfig{
				OID:  root,
				Name: fmt.Sprintf("table%d", i),
			},
			Symbols:    []ddprofiledefinition.SymbolConfig{{OID: root + ".1", Name: fmt.Sprintf("metric%d", i)}},
			MetricTags: []ddprofiledefinition.MetricTagConfig{{Tag: "index", Index: 1}},
		})
	}
	c := New(ddsnmp.NewDeviceStore())
	c.Config = prepareV2Config()
	c.sysInfo = &snmputils.SysInfo{
		SysObjectID: "1.3.6.1.4.1.99999.1",
		Name:        "benchmark",
	}
	session := &normalBenchmarkHandler{
		rows: rows,
	}
	c.snmpSession, c.snmpClient = session, session
	cfg := ddsnmpcollector.Config{
		SnmpClient: c.snmpClient,
		Version:    gosnmp.Version2c,
		MaxOIDs:    32,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        c.Logger,
	}
	if record {
		cfg.AcquisitionObserver = ddsnmpcollector.AcquisitionObserverFunc(c.observeNormalProfile)
	}
	c.ddSnmpColl = ddsnmpcollector.New(cfg)
	return c
}

// Include the production lifecycle and the not-yet-due metadata check. The
// initial poll settles discovery before measuring repeated warm collections.
func BenchmarkNormalRuntimeCollection(b *testing.B) {
	c := newNormalBenchmarkCollector(1, 32, true)
	c.initialized = true
	if len(c.Collect(context.Background())) == 0 {
		b.Fatal("missing initial samples")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if len(c.Collect(context.Background())) == 0 {
			b.Fatal("missing samples")
		}
	}
}

// Cold costs include constructing a device and its profile fixture in both modes.
func BenchmarkNormalColdCollection(b *testing.B) {
	for name, shape := range map[string]struct{ tables, rows int }{"small": {1, 32}, "many_rows": {16, 256}} {
		for mode, capture := range map[string]bool{"baseline": false, "capture": true} {
			b.Run(name+"/"+mode, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					c := newNormalBenchmarkCollector(shape.tables, shape.rows, capture)
					if capture {
						c.beginNormalAttempt("collect")
					}
					mx := make(map[string]int64)
					err := c.collectSNMP(context.Background(), mx)
					if err != nil {
						b.Fatal(err)
					}
					if capture {
						c.finishNormalAttempt(err, mx)
					}
				}
			})
		}
	}
}

// Uses a real populated cut after success, failure and recovery, including
// normalized rows, decisions, cache structure and shared historical operations.
// Encoding deliberately includes a fresh encoder, an allocation upper bound for
// the publisher's reused encoder. The separate publisher benchmark measures IO.
func BenchmarkNormalCaptureAndEncode(b *testing.B) {
	for name, shape := range map[string]struct{ tables, rows int }{"small": {1, 32}, "many_rows": {16, 256}} {
		c := newNormalBenchmarkCollector(shape.tables, shape.rows, true)
		handler := c.snmpClient.(*normalBenchmarkHandler)
		for _, failed := range []bool{false, false, true, false} {
			handler.fail = failed
			c.beginNormalAttempt("collect")
			mx := make(map[string]int64)
			err := c.collectSNMP(context.Background(), mx)
			if (err != nil) != failed {
				b.Fatalf("unexpected collection result: %v", err)
			}
			c.finishNormalAttempt(err, mx)
		}
		for mode, encode := range map[string]bool{"assemble": false, "assemble_and_encode": true} {
			b.Run(name+"/"+mode, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					device, err := c.normal.cut.CaptureNormal()
					if err != nil {
						b.Fatal(err)
					}
					device.RegistrationID, device.RuntimeID = 1, 1
					if err := device.Validate(); err != nil {
						b.Fatal(err)
					}
					if encode {
						if err := diagnostics.Write(io.Discard, diagnostics.Document{
							Format:  diagnostics.Format,
							Version: diagnostics.Version,
							Kind:    diagnostics.KindNormal,
							Normal:  device,
						}); err != nil {
							b.Fatal(err)
						}
					}
				}
			})
		}
	}
}
