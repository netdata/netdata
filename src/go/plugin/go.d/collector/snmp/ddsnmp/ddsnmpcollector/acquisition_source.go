// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

// SourceRecorder belongs to one synchronous collection context. Finish transfers
// its immutable evidence to the attempt; it retains neither clients nor profiles.
// Each operation has independent ownership so a cache reference cannot pin the
// other operations and payloads from its original collection.
type SourceRecorder struct {
	ContextID  uint64
	operations []*ddsnmp.SourceOperation
	finished   bool
}

type sourceClient struct {
	gosnmp.Handler
	recorder *SourceRecorder
}

func (r *SourceRecorder) Wrap(client gosnmp.Handler) gosnmp.Handler {
	return &sourceClient{Handler: client, recorder: r}
}

func (c *sourceClient) SourceRecorder() *SourceRecorder { return c.recorder }

func sourceRecorder(client gosnmp.Handler) *SourceRecorder {
	if c, ok := client.(interface{ SourceRecorder() *SourceRecorder }); ok {
		return c.SourceRecorder()
	}
	return nil
}

func (r *SourceRecorder) Cursor() uint64 {
	if r == nil {
		return 0
	}
	return uint64(len(r.operations))
}

// Operation returns an immutable independently owned source, not a pointer into
// the recorder's operation array. Cache generations can retain it directly.
func (r *SourceRecorder) Operation(ordinal uint64) *ddsnmp.SourceOperation {
	if r == nil || ordinal == 0 || ordinal > uint64(len(r.operations)) {
		return nil
	}
	return r.operations[ordinal-1]
}

func (r *SourceRecorder) operationsSince(cursor uint64) []*ddsnmp.SourceOperation {
	if r == nil || cursor >= uint64(len(r.operations)) {
		return nil
	}
	return slices.Clone(r.operations[cursor:])
}

func (r *SourceRecorder) Finish() []*ddsnmp.SourceOperation {
	if r == nil {
		return nil
	}
	r.finished = true
	operations := r.operations
	r.operations = nil
	return operations
}

func (r *SourceRecorder) requestsSince(cursor uint64) map[string][]uint64 {
	if r == nil || cursor >= uint64(len(r.operations)) {
		return nil
	}
	result := make(map[string][]uint64)
	for i := cursor; i < uint64(len(r.operations)); i++ {
		for _, oid := range r.operations[i].RequestedOIDs {
			oid = trimOID(oid)
			result[oid] = append(result[oid], i+1)
		}
	}
	return result
}

func (c *sourceClient) Get(oids []string) (*gosnmp.SnmpPacket, error) {
	start := time.Now()
	packet, err := c.Handler.Get(oids)
	elapsed := time.Since(start)
	var pdus []gosnmp.SnmpPDU
	if packet != nil {
		pdus = packet.Variables
	}
	c.recorder.record("get", oids, start, elapsed, packet != nil, pdus, snmputils.ClassifyGetFailure(packet, err))
	return packet, err
}

func (c *sourceClient) WalkAll(oid string) ([]gosnmp.SnmpPDU, error) {
	start := time.Now()
	values, err := c.Handler.WalkAll(oid)
	c.recorder.recordWalk("walk", oid, start, time.Since(start), values, err)
	return values, err
}

func (c *sourceClient) BulkWalkAll(oid string) ([]gosnmp.SnmpPDU, error) {
	start := time.Now()
	values, err := c.Handler.BulkWalkAll(oid)
	c.recorder.recordWalk("bulk_walk", oid, start, time.Since(start), values, err)
	return values, err
}

func (r *SourceRecorder) recordWalk(method, oid string, started time.Time, elapsed time.Duration, values []gosnmp.SnmpPDU, err error) {
	failure := snmputils.ClassifyFailure(err)
	if err != nil {
		failure.Operation = "walk"
	}
	r.record(method, []string{oid}, started, elapsed, values != nil, values, failure)
}

func (r *SourceRecorder) record(method string, oids []string, started time.Time, elapsed time.Duration, present bool, pdus []gosnmp.SnmpPDU, failure snmputils.Failure) {
	if r == nil || r.finished {
		return
	}
	operation := &ddsnmp.SourceOperation{ContextID: r.ContextID, Ordinal: uint64(len(r.operations)) + 1, StartedAt: started.UTC(), Method: method, ElapsedNanos: int64(elapsed), Failure: failure, ResultPresent: present,
		RequestedOIDs: make([]string, len(oids)), PDUs: make([]ddsnmp.SourcePDU, len(pdus))}
	for i, oid := range oids {
		operation.RequestedOIDs[i] = strings.Clone(oid)
	}
	for i, pdu := range pdus {
		operation.PDUs[i] = ddsnmp.SourcePDU{OID: strings.Clone(pdu.Name), Type: uint8(pdu.Type), Value: sourceValue(pdu.Value)}
	}
	r.operations = append(r.operations, operation)
}

func sourceValue(value any) ddsnmp.SourceValue {
	v := ddsnmp.SourceValue{}
	switch value := value.(type) {
	case nil:
		v.Kind = "null"
	case []byte:
		v.Kind = "bytes"
		if value == nil {
			v.Kind = "bytes_nil"
		}
		v.Bytes = slices.Clone(value)
	case string:
		if utf8.ValidString(value) {
			v.Kind = "string"
			v.Text = strings.Clone(value)
		} else {
			v.Kind = "string_bytes"
			v.Bytes = []byte(value)
		}
	case int:
		v.Kind = "signed"
		v.Text = strconv.FormatInt(int64(value), 10)
	case int32:
		v.Kind = "signed"
		v.Text = strconv.FormatInt(int64(value), 10)
	case int64:
		v.Kind = "signed"
		v.Text = strconv.FormatInt(value, 10)
	case uint:
		v.Kind = "unsigned"
		v.Text = strconv.FormatUint(uint64(value), 10)
	case uint32:
		v.Kind = "unsigned"
		v.Text = strconv.FormatUint(uint64(value), 10)
	case uint64:
		v.Kind = "unsigned"
		v.Text = strconv.FormatUint(value, 10)
	case float32:
		v.Kind = "float32"
		v.Text = strconv.FormatUint(uint64(math.Float32bits(value)), 16)
	case float64:
		v.Kind = "float64"
		v.Text = strconv.FormatUint(math.Float64bits(value), 16)
	default:
		v.Kind = "unavailable"
	}
	return v
}

func bindSourceGETs(route *AcquisitionRouteReport, requests map[string][]uint64, oid, role string) {
	if route == nil {
		return
	}
	oid = trimOID(oid)
	for _, operation := range requests[oid] {
		route.Sources = append(route.Sources, ddsnmp.SourceBinding{Operation: operation, OID: oid, Role: role})
	}
}

func bindSourceWalk(route *AcquisitionRouteReport, operation uint64, oid, role string) {
	if route == nil || operation == 0 {
		return
	}
	route.Sources = append(route.Sources, ddsnmp.SourceBinding{Operation: operation, OID: trimOID(oid), Role: role})
}

func (o *acquisitionScalarObserver) bindSource(requests map[string][]uint64, configs []ddprofiledefinition.MetricsConfig) {
	if o == nil {
		return
	}
	for i, cfg := range configs {
		route := o.route(i)
		bindSourceGETs(route, requests, cfg.Symbol.OID, "primary")
		for _, tag := range cfg.MetricTags {
			bindSourceGETs(route, requests, tag.Symbol.OID, "dependency")
		}
	}
}

func (o *acquisitionGlobalTagObserver) bindSource(requests map[string][]uint64) {
	if o == nil {
		return
	}
	for i, cfg := range o.configs {
		bindSourceGETs(o.route(i), requests, cfg.Symbol.OID, "primary")
	}
}

func (o *acquisitionMetadataObserver) bindSource(requests map[string][]uint64) {
	if o == nil {
		return
	}
	for name, binding := range o.routes {
		route := o.route(name)
		bindSourceGETs(route, requests, binding.field.Symbol.OID, "primary")
		for _, symbol := range binding.field.Symbols {
			bindSourceGETs(route, requests, symbol.OID, "primary")
		}
	}
}
