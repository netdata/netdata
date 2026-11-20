// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"context"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/gosnmp"
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
	snmputils.Client
	recorder *SourceRecorder
}

func (r *SourceRecorder) Wrap(client snmputils.Client) snmputils.Client {
	return &sourceClient{Client: client, recorder: r}
}

func (c *sourceClient) SourceRecorder() *SourceRecorder { return c.recorder }

func sourceRecorder(client any) *SourceRecorder {
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

func (c *sourceClient) Get(ctx context.Context, oids []string) (*gosnmp.Message, error) {
	start := time.Now()
	packet, err := c.Client.Get(ctx, oids)
	elapsed := time.Since(start)
	var pdus []gosnmp.VarBind
	if packet != nil {
		pdus = packet.VarBinds
	}
	c.recorder.record("get", oids, start, elapsed, packet != nil, pdus, snmputils.ClassifyGetFailure(packet, err))
	return packet, err
}

func (c *sourceClient) WalkAll(ctx context.Context, oid string) ([]gosnmp.VarBind, gosnmp.WalkEnd, error) {
	start := time.Now()
	values, end, err := c.Client.WalkAll(ctx, oid)
	c.recorder.recordWalk("walk", oid, start, time.Since(start), values, err)
	return values, end, err
}

func (c *sourceClient) BulkWalkAll(ctx context.Context, oid string, maxRepetitions uint32) ([]gosnmp.VarBind, gosnmp.WalkEnd, error) {
	start := time.Now()
	values, end, err := c.Client.BulkWalkAll(ctx, oid, maxRepetitions)
	c.recorder.recordWalk("bulk_walk", oid, start, time.Since(start), values, err)
	return values, end, err
}

func (r *SourceRecorder) recordWalk(method, oid string, started time.Time, elapsed time.Duration, values []gosnmp.VarBind, err error) {
	failure := snmputils.ClassifyFailure(err)
	if err != nil {
		failure.Operation = "walk"
	}
	r.record(method, []string{oid}, started, elapsed, values != nil, values, failure)
}

func (r *SourceRecorder) record(method string, oids []string, started time.Time, elapsed time.Duration, present bool, pdus []gosnmp.VarBind, failure snmputils.Failure) {
	if r == nil || r.finished {
		return
	}
	operation := &ddsnmp.SourceOperation{ContextID: r.ContextID, Ordinal: uint64(len(r.operations)) + 1, StartedAt: started.UTC(), Method: method, ElapsedNanos: int64(elapsed), Failure: failure, ResultPresent: present,
		RequestedOIDs: make([]string, len(oids)), PDUs: make([]ddsnmp.SourcePDU, len(pdus))}
	for i, oid := range oids {
		operation.RequestedOIDs[i] = strings.Clone(oid)
	}
	for i, pdu := range pdus {
		operation.PDUs[i] = ddsnmp.SourcePDU{OID: strings.Clone(pdu.Name), Type: uint8(pdu.Value.Type()), Value: sourceValue(pdu.Value)}
	}
	r.operations = append(r.operations, operation)
}

// sourceValue records a value by its SMI type; the evidence kinds are the ones
// the untyped values had: an IpAddress as its text, an empty one as null.
func sourceValue(value gosnmp.Value) ddsnmp.SourceValue {
	v := ddsnmp.SourceValue{Kind: "null"}
	switch value.Type() {
	case gosnmp.OctetString, gosnmp.Opaque:
		b, _ := value.Bytes()
		v.Kind = "bytes"
		if b == nil {
			v.Kind = "bytes_nil"
		}
		v.Bytes = slices.Clone(b)
	case gosnmp.ObjectIdentifier:
		oid, _ := value.OID()
		v.Kind, v.Text = "string", strings.Clone(oid)
	case gosnmp.IPAddress:
		if ip, _ := value.IP(); ip.IsValid() {
			v.Kind, v.Text = "string", ip.String()
		}
	case gosnmp.Integer:
		n, _ := value.Int32()
		v.Kind, v.Text = "signed", strconv.FormatInt(int64(n), 10)
	case gosnmp.Counter32, gosnmp.Gauge32, gosnmp.TimeTicks, gosnmp.Uinteger32, gosnmp.Counter64:
		n, _ := value.Uint64()
		v.Kind, v.Text = "unsigned", strconv.FormatUint(n, 10)
	case gosnmp.OpaqueFloat:
		f, _ := value.Float32()
		v.Kind, v.Text = "float32", strconv.FormatUint(uint64(math.Float32bits(f)), 16)
	case gosnmp.OpaqueDouble:
		f, _ := value.Float64()
		v.Kind, v.Text = "float64", strconv.FormatUint(math.Float64bits(f), 16)
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
