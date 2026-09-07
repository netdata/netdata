// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// processingObserver records only attempted non-success outcomes; it never changes
// production error handling, counters or whether a row is accepted.
type processingObserver struct {
	route *AcquisitionRouteReport
	row   string
}

func processingFor(route *AcquisitionRouteReport, row string) *processingObserver {
	if route == nil {
		return nil
	}
	return &processingObserver{route: route, row: row}
}

func (o *processingObserver) record(field, oid, reason string) {
	o.recordStage(field, oid, reason, "")
}

func (o *processingObserver) recordStage(field, oid, reason, stage string) {
	if o == nil || o.route == nil {
		return
	}
	if field == "" {
		field = "value"
	}
	o.route.Processing = append(o.route.Processing, ddsnmp.ProcessingEvent{
		RowIndex: strings.Clone(o.row),
		Field:    strings.Clone(field),
		OID:      strings.Clone(oid),
		Reason:   reason,
		Stage:    stage,
	})
}

func (o *acquisitionTableObservation) processing(row string) *processingObserver {
	if o == nil {
		return nil
	}
	return processingFor(o.collection.route(int(o.routeOrdinal)), row)
}

func (o *acquisitionScalarObserver) processing(index int) *processingObserver {
	return processingFor(o.route(index), "")
}

func (o *acquisitionMetadataObserver) processing(name string) *processingObserver {
	return processingFor(o.route(name), "")
}

func (o *acquisitionGlobalTagObserver) processing(index int) *processingObserver {
	return processingFor(o.route(index), "")
}

// Field components are static profile paths. Join only when an event is emitted.
func (ctx bgpValueContext) forField(field string) bgpValueContext {
	ctx.field, ctx.fieldLeaf = field, ""
	return ctx
}

func (ctx bgpValueContext) forLeaf(leaf string) bgpValueContext {
	ctx.fieldLeaf = leaf
	return ctx
}

func (ctx bgpValueContext) record(sym ddprofiledefinition.SymbolConfig, oid, reason string) {
	if ctx.processing == nil {
		return
	}
	field := ctx.field
	if ctx.fieldLeaf != "" {
		field += "." + ctx.fieldLeaf
	}
	if field == "" {
		field = sym.Name
	}
	ctx.processing.recordStage(field, oid, reason, ctx.processingStage)
}
