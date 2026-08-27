// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"errors"
	"reflect"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/diagnostic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/reversedns"
)

const (
	topologyDiagnosticMaxDNSRecords = 4_096
	topologyDiagnosticMaxOUIRecords = 16_384
)

type topologyGraphDiagnosticCapture struct {
	transaction *diagnostic.CaptureTransaction
	generation  *topologyGeneration
	query       diagnostic.GraphQueryOptionsV1
	dns         []diagnostic.DNSRecordV1
	oui         []diagnostic.OUIRecordV1
	err         error
	finished    bool
}

func beginTopologyGraphDiagnosticCapture(
	recorder *diagnostic.Recorder,
	generation *topologyGeneration,
	options topologyoptions.QueryOptions,
) *topologyGraphDiagnosticCapture {
	if recorder == nil || generation == nil || generation.diagnosticMember.ID() == 0 {
		return nil
	}
	transaction, err := recorder.Begin(diagnostic.GraphCapabilityV1())
	if err != nil {
		return nil
	}
	capture := &topologyGraphDiagnosticCapture{
		transaction: transaction,
		generation:  generation,
		query:       diagnosticGraphQueryOptions(options),
	}
	if err := transaction.DefineSection(diagnostic.GraphSectionGeneration, diagnostic.StateSuccess, 1); err != nil {
		capture.abort()
		return nil
	}
	if err := transaction.AddReference(diagnostic.GraphSectionGeneration, generation.diagnosticMember); err != nil {
		capture.abort()
		return nil
	}
	if err := transaction.DefineSection(diagnostic.GraphSectionQuery, diagnostic.StateSuccess, 1); err != nil {
		capture.abort()
		return nil
	}
	queryBytes := diagnosticRetainedBytes(reflect.ValueOf(capture.query)) + 512
	_, err = transaction.AddDerivedOwned(
		diagnostic.GraphSectionQuery,
		diagnostic.MemberType{Kind: diagnostic.KindGraphQuery, Schema: diagnostic.SchemaV1},
		[]diagnostic.MemberHandle{generation.diagnosticMember},
		func(refs []diagnostic.ContentRef) (any, error) {
			if len(refs) != 1 {
				return nil, errors.New("graph query requires one generation")
			}
			return diagnostic.GraphQueryV1{
				CaptureID:          transaction.CaptureID(),
				GenerationSequence: generation.sequence,
				Generation:         refs[0],
				Options:            capture.query,
			}, nil
		},
		queryBytes,
	)
	if err != nil {
		capture.abort()
		return nil
	}
	return capture
}

func (c *topologyGraphDiagnosticCapture) observeDNS(addr string, result reversedns.Result) {
	if c == nil || c.finished || c.err != nil {
		return
	}
	if len(c.dns) >= topologyDiagnosticMaxDNSRecords {
		c.err = errors.New("DNS diagnostic trace limit exceeded")
		return
	}
	record := diagnostic.DNSRecordV1{Ordinal: uint64(len(c.dns)), IP: addr}
	switch result.State {
	case reversedns.StateMiss:
		record.State = diagnostic.DNSStateMiss
	case reversedns.StatePositive:
		record.State = diagnostic.DNSStatePositive
		record.Name = result.Name
	case reversedns.StateNegative:
		record.State = diagnostic.DNSStateCachedNegative
	default:
		c.err = errors.New("unsupported DNS diagnostic state")
		return
	}
	c.dns = append(c.dns, record)
}

func (c *topologyGraphDiagnosticCapture) observeOUI(mac, vendor, prefix string) {
	if c == nil || c.finished || c.err != nil {
		return
	}
	if len(c.oui) >= topologyDiagnosticMaxOUIRecords {
		c.err = errors.New("OUI diagnostic trace limit exceeded")
		return
	}
	c.oui = append(c.oui, diagnostic.OUIRecordV1{
		Ordinal: uint64(len(c.oui)),
		MAC:     mac,
		Vendor:  vendor,
		Prefix:  prefix,
	})
}

func (c *topologyGraphDiagnosticCapture) finish(ok bool, buildErr error) {
	if c == nil || c.finished {
		return
	}

	c.defineTraceSections()
	state := diagnostic.StateSuccess
	switch {
	case buildErr != nil || c.err != nil:
		state = diagnostic.StateIncomplete
		_ = c.transaction.MarkIncomplete(errors.New("graph diagnostic capture is incomplete"))
	case !ok:
		state = diagnostic.StateEmpty
	}
	if err := c.transaction.Commit(state); err == nil {
		c.finished = true
	}
}

func (c *topologyGraphDiagnosticCapture) defineTraceSections() {
	dnsState := diagnostic.StateEmpty
	if len(c.dns) > 0 {
		dnsState = diagnostic.StateSuccess
	}
	_ = c.transaction.DefineSection(diagnostic.GraphSectionDNSTrace, dnsState, uint64(len(c.dns)))
	if len(c.dns) > 0 {
		trace := diagnostic.DNSTraceV1{CaptureID: c.transaction.CaptureID(), Records: c.dns}
		if _, err := c.transaction.AddOwned(
			diagnostic.GraphSectionDNSTrace,
			diagnostic.MemberType{Kind: diagnostic.KindDNSTrace, Schema: diagnostic.SchemaV1},
			trace,
			diagnosticRetainedBytes(reflect.ValueOf(trace)),
		); err != nil {
			c.err = errors.Join(c.err, errors.New("DNS trace admission failed"))
		}
	}

	ouiState := diagnostic.StateEmpty
	if len(c.oui) > 0 {
		ouiState = diagnostic.StateSuccess
	}
	_ = c.transaction.DefineSection(diagnostic.GraphSectionOUITrace, ouiState, uint64(len(c.oui)))
	if len(c.oui) > 0 {
		trace := diagnostic.OUITraceV1{CaptureID: c.transaction.CaptureID(), Records: c.oui}
		if _, err := c.transaction.AddOwned(
			diagnostic.GraphSectionOUITrace,
			diagnostic.MemberType{Kind: diagnostic.KindOUITrace, Schema: diagnostic.SchemaV1},
			trace,
			diagnosticRetainedBytes(reflect.ValueOf(trace)),
		); err != nil {
			c.err = errors.Join(c.err, errors.New("OUI trace admission failed"))
		}
	}
}

func (c *topologyGraphDiagnosticCapture) abort() {
	if c == nil || c.finished {
		return
	}
	if err := c.transaction.Abort(errors.New("graph query did not reach a terminal result")); err == nil {
		c.finished = true
	}
}

func diagnosticGraphQueryOptions(options topologyoptions.QueryOptions) diagnostic.GraphQueryOptionsV1 {
	return diagnostic.GraphQueryOptionsV1{
		CollapseActorsByIP:     options.CollapseActorsByIP,
		EliminateNonIPInferred: options.EliminateNonIPInferred,
		MapType:                options.MapType,
		InferenceStrategy:      options.InferenceStrategy,
		ManagedDeviceFocus:     options.ManagedDeviceFocus,
		Depth:                  options.Depth,
	}
}

func defaultTopologyVendorLookup(mac string) (vendor string, prefix string) {
	return topologyengine.LookupVendorByMAC(mac)
}
