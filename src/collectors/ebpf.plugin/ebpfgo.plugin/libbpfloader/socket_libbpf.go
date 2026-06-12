//go:build netdata_ebpf_libbpf

package libbpfloader

/*
#include <stdlib.h>

struct netdata_ebpf_socket_runtime;

struct netdata_ebpf_socket_runtime *netdata_socket_runtime_open_mode(const char *path, int use_core);
int netdata_socket_runtime_prepare(struct netdata_ebpf_socket_runtime *rt, int maps_per_core);
int netdata_socket_runtime_load(struct netdata_ebpf_socket_runtime *rt);
int netdata_socket_runtime_attach(struct netdata_ebpf_socket_runtime *rt);
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

func (r *SocketRuntime) Close() {
	if r == nil || r.ptr == nil {
		return
	}

	C.netdata_socket_runtime_close(r.ptr)
	r.ptr = nil
}
