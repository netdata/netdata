//go:build linux && cgo

package main

/*
#include <stddef.h>
#include <stdlib.h>
#include <string.h>

#include "shared_dns_memory.h"

// Inline offset getters for per-field ABI verification in assertSharedDnsMemoryLayout.
// Declared as static inline to avoid external-linkage requirements across CGo
// translation units; CGo inlines the call and needs no exported symbol.
static inline size_t dns_flow_off_timestamp_us_fn(void) { return offsetof(struct ebpfgo_dns_flow_record, timestamp_us); }
static inline size_t dns_flow_off_domain_fn(void)       { return offsetof(struct ebpfgo_dns_flow_record, domain); }
static inline size_t dns_flow_off_client_port_fn(void)  { return offsetof(struct ebpfgo_dns_flow_record, client_port); }
static inline size_t dns_agg_off_queries_udp4_fn(void)   { return offsetof(struct ebpfgo_dns_aggregate, queries_udp4); }
static inline size_t dns_agg_off_responses_udp4_fn(void) { return offsetof(struct ebpfgo_dns_aggregate, responses_udp4); }
*/
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// ebpfgoDnsAggregate mirrors struct ebpfgo_dns_aggregate for CGo ABI verification.
// Field order and types must match the C struct exactly.
type ebpfgoDnsAggregate struct {
	QueriesUDP4   uint64
	QueriesUDP6   uint64
	QueriesTCP4   uint64
	QueriesTCP6   uint64
	ResponsesUDP4 uint64
	ResponsesUDP6 uint64
	ResponsesTCP4 uint64
	ResponsesTCP6 uint64
}

// ebpfgoDnsFlowRecord mirrors struct ebpfgo_dns_flow_record (320 bytes).
// Must stay in sync with apps_ebpf_shared_dns_row.h — verified by
// assertSharedDnsMemoryLayout at startup.
type ebpfgoDnsFlowRecord struct {
	TimestampUs uint64
	LatencyUs   uint64
	ServerIP    [4]uint32
	ClientIP    [4]uint32
	Domain      [256]byte
	ClientPort  uint16
	QueryType   uint16
	RCode       uint16
	Protocol    uint8
	IPVersion   uint8
	TimedOut    uint8
	Pad         [7]uint8
}

func copyDNSDomain(dst *[256]byte, domain string) {
	n := copy(dst[:len(dst)-1], domain)
	dst[n] = 0
}

// assertSharedDnsMemoryLayout panics if the Go DNS mirror structs drift from
// their C counterparts.  Checks total size and key field offsets so a field
// reorder that preserves struct size is also caught.
func assertSharedDnsMemoryLayout() {
	if got, want := unsafe.Sizeof(ebpfgoDnsAggregate{}),
		uintptr(C.sizeof_struct_ebpfgo_dns_aggregate); got != want {
		panic(fmt.Sprintf("ebpfgo_dns_aggregate ABI mismatch: Go=%d C=%d", got, want))
	}
	if got, want := unsafe.Offsetof(ebpfgoDnsAggregate{}.QueriesUDP4),
		uintptr(C.dns_agg_off_queries_udp4_fn()); got != want {
		panic(fmt.Sprintf("ebpfgo_dns_aggregate.QueriesUDP4 offset mismatch: Go=%d C=%d", got, want))
	}
	// ResponsesUDP4 is field 5 of 8 (offset 32). Checking a non-first same-typed
	// field catches silent reorders that preserve total size.
	if got, want := unsafe.Offsetof(ebpfgoDnsAggregate{}.ResponsesUDP4),
		uintptr(C.dns_agg_off_responses_udp4_fn()); got != want {
		panic(fmt.Sprintf("ebpfgo_dns_aggregate.ResponsesUDP4 offset mismatch: Go=%d C=%d", got, want))
	}
	if got, want := unsafe.Sizeof(ebpfgoDnsFlowRecord{}),
		uintptr(C.sizeof_struct_ebpfgo_dns_flow_record); got != want {
		panic(fmt.Sprintf("ebpfgo_dns_flow_record ABI mismatch: Go=%d C=%d", got, want))
	}
	if got, want := unsafe.Offsetof(ebpfgoDnsFlowRecord{}.TimestampUs),
		uintptr(C.dns_flow_off_timestamp_us_fn()); got != want {
		panic(fmt.Sprintf("ebpfgo_dns_flow_record.TimestampUs offset mismatch: Go=%d C=%d", got, want))
	}
	if got, want := unsafe.Offsetof(ebpfgoDnsFlowRecord{}.Domain),
		uintptr(C.dns_flow_off_domain_fn()); got != want {
		panic(fmt.Sprintf("ebpfgo_dns_flow_record.Domain offset mismatch: Go=%d C=%d", got, want))
	}
	if got, want := unsafe.Offsetof(ebpfgoDnsFlowRecord{}.ClientPort),
		uintptr(C.dns_flow_off_client_port_fn()); got != want {
		panic(fmt.Sprintf("ebpfgo_dns_flow_record.ClientPort offset mismatch: Go=%d C=%d", got, want))
	}
}

type SharedDnsMemoryPublisher struct {
	ptr *C.struct_shared_dns_memory
}

func NewSharedDnsMemoryPublisher(updateEverySec uint32) (*SharedDnsMemoryPublisher, error) {
	assertSharedDnsMemoryLayout()

	ctx := C.shared_dns_memory_open(C.uint32_t(updateEverySec))
	if ctx == nil {
		return nil, fmt.Errorf("open shared dns memory failed")
	}

	return &SharedDnsMemoryPublisher{ptr: ctx}, nil
}

func (p *SharedDnsMemoryPublisher) Publish(snap libbpfloader.DNSSnapshot, flows []libbpfloader.DNSFlowRecord) {
	if p == nil || p.ptr == nil {
		return
	}

	agg := ebpfgoDnsAggregate{
		QueriesUDP4:   snap.QueriesUDPv4,
		QueriesUDP6:   snap.QueriesUDPv6,
		QueriesTCP4:   snap.QueriesTCPv4,
		QueriesTCP6:   snap.QueriesTCPv6,
		ResponsesUDP4: snap.ResponsesUDPv4,
		ResponsesUDP6: snap.ResponsesUDPv6,
		ResponsesTCP4: snap.ResponsesTCPv4,
		ResponsesTCP6: snap.ResponsesTCPv6,
	}

	cagg := (*C.struct_ebpfgo_dns_aggregate)(unsafe.Pointer(&agg))

	if len(flows) > 0 {
		// Build a C-layout array using the Go mirror struct. The buf slice is kept
		// in scope across the C call so the GC cannot collect it before the call
		// completes. CGo permits passing Go memory (value types only, no Go pointers)
		// to C for the duration of a single C call.
		buf := make([]ebpfgoDnsFlowRecord, len(flows))
		for i, f := range flows {
			r := &buf[i]
			r.TimestampUs = f.TimestampUs
			r.LatencyUs = f.LatencyUs
			r.ClientPort = f.ClientPort
			r.QueryType = f.QueryType
			r.RCode = f.RCode
			r.Protocol = f.Protocol
			r.IPVersion = f.IPVersion
			if f.TimedOut {
				r.TimedOut = 1
			}
			copyDNSDomain(&r.Domain, f.Domain)
			r.ServerIP = f.ServerIP
			r.ClientIP = f.ClientIP
		}
		C.shared_dns_memory_publish(
			p.ptr, cagg,
			(*C.struct_ebpfgo_dns_flow_record)(unsafe.Pointer(&buf[0])),
			C.uint32_t(len(buf)))
	} else {
		C.shared_dns_memory_publish(p.ptr, cagg, nil, 0)
	}
}

func (p *SharedDnsMemoryPublisher) Close() {
	if p == nil || p.ptr == nil {
		return
	}

	C.shared_dns_memory_close(p.ptr)
	p.ptr = nil
}
