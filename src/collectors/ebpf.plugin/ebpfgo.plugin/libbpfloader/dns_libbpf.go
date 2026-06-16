//go:build netdata_ebpf_libbpf

package libbpfloader

/*
#include <stdlib.h>
#include <stdint.h>

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

struct netdata_dns_runtime *netdata_dns_runtime_open_mode(const char *path, int use_core);
int netdata_dns_runtime_prepare(struct netdata_dns_runtime *rt);
int netdata_dns_runtime_load(struct netdata_dns_runtime *rt);
int netdata_dns_runtime_attach(struct netdata_dns_runtime *rt);
int netdata_dns_runtime_snapshot(struct netdata_dns_runtime *rt, struct netdata_dns_snapshot *out);
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

func NewDNSRuntime(path string, useCore bool) (*DNSRuntime, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	cUseCore := C.int(0)
	if useCore {
		cUseCore = 1
	}

	rt := C.netdata_dns_runtime_open_mode(cpath, cUseCore)
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

func (r *DNSRuntime) Close() {
	if r == nil || r.ptr == nil {
		return
	}

	C.netdata_dns_runtime_close(r.ptr)
	r.ptr = nil
}
