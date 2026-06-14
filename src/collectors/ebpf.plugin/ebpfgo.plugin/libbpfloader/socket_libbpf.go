//go:build netdata_ebpf_libbpf

package libbpfloader

/*
#include <stdlib.h>
#include <stdint.h>

struct netdata_ebpf_socket_runtime;

struct netdata_ebpf_socket_snapshot {
    uint64_t calls_tcp_sendmsg;
    uint64_t error_tcp_sendmsg;
    uint64_t bytes_tcp_sendmsg;
    uint64_t calls_tcp_cleanup_rbuf;
    uint64_t error_tcp_cleanup_rbuf;
    uint64_t bytes_tcp_cleanup_rbuf;
    uint64_t calls_tcp_close;
    uint64_t calls_udp_recvmsg;
    uint64_t error_udp_recvmsg;
    uint64_t bytes_udp_recvmsg;
    uint64_t calls_udp_sendmsg;
    uint64_t error_udp_sendmsg;
    uint64_t bytes_udp_sendmsg;
    uint64_t tcp_retransmit;
    uint64_t calls_tcp_connect_ipv4;
    uint64_t error_tcp_connect_ipv4;
    uint64_t calls_tcp_connect_ipv6;
    uint64_t error_tcp_connect_ipv6;
    uint64_t inbound_conn_tcp;
    uint64_t inbound_conn_udp;
};

struct netdata_socket_per_pid_entry {
    uint32_t pid;
    uint64_t bytes_sent;
    uint64_t bytes_received;
    uint64_t call_tcp_sent;
    uint64_t call_tcp_received;
    uint64_t retransmit;
    uint64_t call_udp_sent;
    uint64_t call_udp_received;
    uint64_t call_close;
    uint64_t call_tcp_v4_connection;
    uint64_t call_tcp_v6_connection;
};

struct netdata_ebpf_socket_runtime *netdata_socket_runtime_open_mode(const char *path, int use_core);
int netdata_socket_runtime_prepare(struct netdata_ebpf_socket_runtime *rt, int maps_per_core);
int netdata_socket_runtime_load(struct netdata_ebpf_socket_runtime *rt);
int netdata_socket_runtime_attach(struct netdata_ebpf_socket_runtime *rt);
int netdata_socket_runtime_snapshot(struct netdata_ebpf_socket_runtime *rt, int maps_per_core, struct netdata_ebpf_socket_snapshot *out);
struct netdata_socket_per_pid_entry *netdata_socket_per_pid_snapshot(struct netdata_ebpf_socket_runtime *rt, int *out_count);
void netdata_socket_runtime_close(struct netdata_ebpf_socket_runtime *rt);
*/
import "C"

import (
	"fmt"
	"unsafe"
)

type SocketRuntime struct {
	ptr *C.struct_netdata_ebpf_socket_runtime
}

func NewSocketRuntime(path string, useCore bool) (*SocketRuntime, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	cUseCore := C.int(0)
	if useCore {
		cUseCore = 1
	}

	rt := C.netdata_socket_runtime_open_mode(cpath, cUseCore)
	if rt == nil {
		return nil, fmt.Errorf("open socket object %q failed", path)
	}

	return &SocketRuntime{ptr: rt}, nil
}

func (r *SocketRuntime) Prepare(mapsPerCore bool) error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	cMapsPerCore := C.int(0)
	if mapsPerCore {
		cMapsPerCore = 1
	}

	if ret := C.netdata_socket_runtime_prepare(r.ptr, cMapsPerCore); ret != 0 {
		return fmt.Errorf("prepare socket runtime failed: %d", int(ret))
	}

	return nil
}

func (r *SocketRuntime) Load() error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	if ret := C.netdata_socket_runtime_load(r.ptr); ret != 0 {
		return fmt.Errorf("load socket runtime failed: %d", int(ret))
	}

	return nil
}

func (r *SocketRuntime) Attach() error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	if ret := C.netdata_socket_runtime_attach(r.ptr); ret != 0 {
		return fmt.Errorf("attach socket runtime failed: %d", int(ret))
	}

	return nil
}

func (r *SocketRuntime) Snapshot(mapsPerCore bool) (SocketSnapshot, error) {
	if r == nil || r.ptr == nil {
		return SocketSnapshot{}, ErrDisabled
	}

	cMapsPerCore := C.int(0)
	if mapsPerCore {
		cMapsPerCore = 1
	}

	var csnap C.struct_netdata_ebpf_socket_snapshot
	if ret := C.netdata_socket_runtime_snapshot(r.ptr, cMapsPerCore, &csnap); ret != 0 {
		return SocketSnapshot{}, fmt.Errorf("socket snapshot failed: %d", int(ret))
	}

	return SocketSnapshot{
		CallsTCPSendmsg:     uint64(csnap.calls_tcp_sendmsg),
		ErrorTCPSendmsg:     uint64(csnap.error_tcp_sendmsg),
		BytesTCPSendmsg:     uint64(csnap.bytes_tcp_sendmsg),
		CallsTCPCleanupRbuf: uint64(csnap.calls_tcp_cleanup_rbuf),
		ErrorTCPCleanupRbuf: uint64(csnap.error_tcp_cleanup_rbuf),
		BytesTCPCleanupRbuf: uint64(csnap.bytes_tcp_cleanup_rbuf),
		CallsTCPClose:       uint64(csnap.calls_tcp_close),
		CallsUDPRecvmsg:     uint64(csnap.calls_udp_recvmsg),
		ErrorUDPRecvmsg:     uint64(csnap.error_udp_recvmsg),
		BytesUDPRecvmsg:     uint64(csnap.bytes_udp_recvmsg),
		CallsUDPSendmsg:     uint64(csnap.calls_udp_sendmsg),
		ErrorUDPSendmsg:     uint64(csnap.error_udp_sendmsg),
		BytesUDPSendmsg:     uint64(csnap.bytes_udp_sendmsg),
		TCPRetransmit:       uint64(csnap.tcp_retransmit),
		CallsTCPConnectIPv4: uint64(csnap.calls_tcp_connect_ipv4),
		ErrorTCPConnectIPv4: uint64(csnap.error_tcp_connect_ipv4),
		CallsTCPConnectIPv6: uint64(csnap.calls_tcp_connect_ipv6),
		ErrorTCPConnectIPv6: uint64(csnap.error_tcp_connect_ipv6),
		InboundConnTCP:      uint64(csnap.inbound_conn_tcp),
		InboundConnUDP:      uint64(csnap.inbound_conn_udp),
	}, nil
}

// SnapshotPerPID reads tbl_nd_socket, aggregates per-PID totals across all
// per-CPU values, and returns a sorted slice (ascending PID).  The returned
// slice is owned by the caller; the underlying C buffer remains valid only
// until the next call to SnapshotPerPID on this runtime.
func (r *SocketRuntime) SnapshotPerPID() ([]SocketPIDEntry, error) {
	if r == nil || r.ptr == nil {
		return nil, ErrDisabled
	}

	var cCount C.int
	cEntries := C.netdata_socket_per_pid_snapshot(r.ptr, &cCount)
	if cEntries == nil || cCount <= 0 {
		return nil, nil
	}

	n := int(cCount)
	raw := unsafe.Slice(cEntries, n)
	out := make([]SocketPIDEntry, n)
	for i, e := range raw {
		out[i] = SocketPIDEntry{
			PID:                 uint32(e.pid),
			BytesSent:           uint64(e.bytes_sent),
			BytesReceived:       uint64(e.bytes_received),
			CallTCPSent:         uint64(e.call_tcp_sent),
			CallTCPReceived:     uint64(e.call_tcp_received),
			Retransmit:          uint64(e.retransmit),
			CallUDPSent:         uint64(e.call_udp_sent),
			CallUDPReceived:     uint64(e.call_udp_received),
			CallClose:           uint64(e.call_close),
			CallTCPV4Connection: uint64(e.call_tcp_v4_connection),
			CallTCPV6Connection: uint64(e.call_tcp_v6_connection),
		}
	}
	return out, nil
}

func (r *SocketRuntime) Close() {
	if r == nil || r.ptr == nil {
		return
	}

	C.netdata_socket_runtime_close(r.ptr)
	r.ptr = nil
}
