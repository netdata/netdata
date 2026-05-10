//go:build netdata_ebpf_libbpf

package libbpfloader

/*
#include <stdlib.h>

struct netdata_ebpf_cachestat_runtime;

struct netdata_ebpf_cachestat_runtime *netdata_cachestat_runtime_open(const char *path);
int netdata_cachestat_runtime_prepare(struct netdata_ebpf_cachestat_runtime *rt, unsigned int pid_table_size, int maps_per_core);
int netdata_cachestat_runtime_load(struct netdata_ebpf_cachestat_runtime *rt);
int netdata_cachestat_runtime_attach(struct netdata_ebpf_cachestat_runtime *rt, const char *account_function);
void netdata_cachestat_runtime_close(struct netdata_ebpf_cachestat_runtime *rt);
*/
import "C"

import (
	"fmt"
	"unsafe"
)

type CachestatRuntime struct {
	ptr *C.struct_netdata_ebpf_cachestat_runtime
}

type CachestatRuntimeConfig struct {
	PidTableSize    uint32
	MapsPerCore     bool
	AccountFunction string
}

func NewCachestatRuntime(path string) (*CachestatRuntime, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	rt := C.netdata_cachestat_runtime_open(cpath)
	if rt == nil {
		return nil, fmt.Errorf("open cachestat object %q failed", path)
	}

	return &CachestatRuntime{ptr: rt}, nil
}

func (r *CachestatRuntime) Prepare(pidTableSize uint32, mapsPerCore bool) error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	cMapsPerCore := C.int(0)
	if mapsPerCore {
		cMapsPerCore = 1
	}

	if ret := C.netdata_cachestat_runtime_prepare(r.ptr, C.uint(pidTableSize), cMapsPerCore); ret != 0 {
		return fmt.Errorf("prepare cachestat runtime failed: %d", int(ret))
	}

	return nil
}

func (r *CachestatRuntime) Load() error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	if ret := C.netdata_cachestat_runtime_load(r.ptr); ret != 0 {
		return fmt.Errorf("load cachestat runtime failed: %d", int(ret))
	}

	return nil
}

func (r *CachestatRuntime) Attach(accountFunction string) error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	cfn := C.CString(accountFunction)
	defer C.free(unsafe.Pointer(cfn))

	if ret := C.netdata_cachestat_runtime_attach(r.ptr, cfn); ret != 0 {
		return fmt.Errorf("attach cachestat runtime failed: %d", int(ret))
	}

	return nil
}

func (r *CachestatRuntime) Close() {
	if r == nil || r.ptr == nil {
		return
	}

	C.netdata_cachestat_runtime_close(r.ptr)
	r.ptr = nil
}
