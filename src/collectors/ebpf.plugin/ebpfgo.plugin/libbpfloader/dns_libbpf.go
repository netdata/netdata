//go:build netdata_ebpf_libbpf

package libbpfloader

/*
#include <stdlib.h>
#include <stdint.h>
#include <string.h>

#define DNS_DOMAIN_MAX 256
#define DNS_FLOW_RING_CAP 1000

struct netdata_dns_runtime;

struct netdata_dns_snapshot {
    uint64_t queries_udp_ipv4;
    uint64_t queries_udp_ipv6;
    uint64_t queries_tcp_ipv4;
    uint64_t queries_tcp_ipv6;
    uint64_t responses_udp_ipv4;
    uint64_t responses_udp_ipv6;
    uint64_t responses_tcp_ipv4;
    uint64_t responses_tcp_ipv6;
};

struct netdata_dns_flow_record {
    uint64_t timestamp_us;
    uint64_t latency_us;
    uint32_t server_ip[4];
    uint32_t client_ip[4];
    char     domain[DNS_DOMAIN_MAX];
    uint16_t client_port;
    uint16_t query_type;
    uint16_t rcode;
    uint8_t  protocol;
    uint8_t  ip_version;
    uint8_t  timed_out;
    uint8_t  _pad[7];
};

struct netdata_dns_runtime *netdata_dns_runtime_open_mode(const char *path, int use_core, int per_query);
int netdata_dns_runtime_prepare(struct netdata_dns_runtime *rt);
int netdata_dns_runtime_load(struct netdata_dns_runtime *rt);
int netdata_dns_runtime_attach(struct netdata_dns_runtime *rt);
int netdata_dns_runtime_snapshot(struct netdata_dns_runtime *rt, struct netdata_dns_snapshot *out);
int netdata_dns_runtime_flow_snapshot(struct netdata_dns_runtime *rt,
                                      struct netdata_dns_flow_record *out, int max_records);
void netdata_dns_runtime_close(struct netdata_dns_runtime *rt);
*/
import "C"

import (
	"fmt"
	"unsafe"
)

type DNSRuntime struct {
	ptr *C.struct_netdata_dns_runtime
}

func NewDNSRuntime(path string, useCore bool, perQuery bool) (*DNSRuntime, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	cUseCore := C.int(0)
	if useCore {
		cUseCore = 1
	}

	cPerQuery := C.int(0)
	if perQuery {
		cPerQuery = 1
	}

	rt := C.netdata_dns_runtime_open_mode(cpath, cUseCore, cPerQuery)
	if rt == nil {
		return nil, fmt.Errorf("open DNS object %q failed", path)
	}

	return &DNSRuntime{ptr: rt}, nil
}

func (r *DNSRuntime) Prepare() error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	if ret := C.netdata_dns_runtime_prepare(r.ptr); ret != 0 {
		return fmt.Errorf("prepare DNS runtime failed: %d", int(ret))
	}

	return nil
}

func (r *DNSRuntime) Load() error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	if ret := C.netdata_dns_runtime_load(r.ptr); ret != 0 {
		return fmt.Errorf("load DNS runtime failed: %d", int(ret))
	}

	return nil
}

func (r *DNSRuntime) Attach() error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	if ret := C.netdata_dns_runtime_attach(r.ptr); ret != 0 {
		return fmt.Errorf("attach DNS runtime failed: %d", int(ret))
	}

	return nil
}

func (r *DNSRuntime) Snapshot() (DNSSnapshot, error) {
	if r == nil || r.ptr == nil {
		return DNSSnapshot{}, ErrDisabled
	}

	var csnap C.struct_netdata_dns_snapshot
	if ret := C.netdata_dns_runtime_snapshot(r.ptr, &csnap); ret != 0 {
		return DNSSnapshot{}, fmt.Errorf("DNS snapshot failed: %d", int(ret))
	}

	return DNSSnapshot{
		QueriesUDPv4:   uint64(csnap.queries_udp_ipv4),
		QueriesUDPv6:   uint64(csnap.queries_udp_ipv6),
		QueriesTCPv4:   uint64(csnap.queries_tcp_ipv4),
		QueriesTCPv6:   uint64(csnap.queries_tcp_ipv6),
		ResponsesUDPv4: uint64(csnap.responses_udp_ipv4),
		ResponsesUDPv6: uint64(csnap.responses_udp_ipv6),
		ResponsesTCPv4: uint64(csnap.responses_tcp_ipv4),
		ResponsesTCPv6: uint64(csnap.responses_tcp_ipv6),
	}, nil
}

// FlowSnapshot returns per-query DNS flow records from the 20-second ring.
// Only records within the live window are returned.
func (r *DNSRuntime) FlowSnapshot() ([]DNSFlowRecord, error) {
	if r == nil || r.ptr == nil {
		return nil, ErrDisabled
	}

	cbuf := (*C.struct_netdata_dns_flow_record)(
		C.calloc(C.DNS_FLOW_RING_CAP, C.sizeof_struct_netdata_dns_flow_record))
	if cbuf == nil {
		return nil, fmt.Errorf("DNS FlowSnapshot: calloc failed")
	}
	defer C.free(unsafe.Pointer(cbuf))

	count := C.netdata_dns_runtime_flow_snapshot(r.ptr, cbuf, C.DNS_FLOW_RING_CAP)
	if count < 0 {
		return nil, fmt.Errorf("DNS FlowSnapshot failed: %d", int(count))
	}

	n := int(count)
	out := make([]DNSFlowRecord, 0, n)

	cslice := (*[1 << 20]C.struct_netdata_dns_flow_record)(unsafe.Pointer(cbuf))[:n:n]
	for i := 0; i < n; i++ {
		cr := &cslice[i]
		rec := DNSFlowRecord{
			TimestampUs: uint64(cr.timestamp_us),
			LatencyUs:   uint64(cr.latency_us),
			ClientPort:  uint16(cr.client_port),
			QueryType:   uint16(cr.query_type),
			RCode:       uint16(cr.rcode),
			Protocol:    uint8(cr.protocol),
			IPVersion:   uint8(cr.ip_version),
			TimedOut:    cr.timed_out != 0,
			Domain:      C.GoString(&cr.domain[0]),
		}
		for j := 0; j < 4; j++ {
			rec.ServerIP[j] = uint32(cr.server_ip[j])
			rec.ClientIP[j] = uint32(cr.client_ip[j])
		}
		out = append(out, rec)
	}

	return out, nil
}

func (r *DNSRuntime) Close() {
	if r == nil || r.ptr == nil {
		return
	}

	C.netdata_dns_runtime_close(r.ptr)
	r.ptr = nil
}
